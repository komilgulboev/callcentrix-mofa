package asterisk

import (
	"database/sql"
	"fmt"
	"log"
)

// AsteriskServer is one physical Asterisk box in a multi-server deployment
// that all share the same Postgres realtime schema. Users are assigned to a
// server (see PickLeastLoadedServer) so the app knows which box's AMI to use
// for that user's actions and which WS URI to proxy their softphone to.
// SIP/dialplan config itself needs no per-server changes — see
// ASTERISK_CLUSTER_SETUP.md for the one-time Asterisk-side realtime-contact
// setup that lets any box reach a user registered on any other box directly.
type AsteriskServer struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	AMIHost   string `json:"amiHost"`
	AMIUser   string `json:"amiUser"`
	AMIPass   string `json:"amiPass"`
	WSUri     string `json:"wsUri"`
	SIPHost   string `json:"sipHost"`
	SIPPort   int    `json:"sipPort"`
	Active    bool   `json:"active"`
	CreatedAt string `json:"createdAt"`
}

func serverTrustEndpointID(a, b int) string { return fmt.Sprintf("trust-asterisk-%d-%d", a, b) }
func serverTrustIdentifyID(a, b int) string { return fmt.Sprintf("identify-asterisk-%d-%d", a, b) }

// interServerRelayContext is a single tenant-agnostic dialplan context shared
// by every inter-server trust endpoint: an inbound INVITE from a peer box
// carries the target username in ${EXTEN} (e.g. sip:bob@this-box, because the
// originating box dialed bob's realtime contact directly), and PJSIP endpoint
// IDs are globally unique usernames regardless of tenant — so a single
// tenant-agnostic Dial(PJSIP/${EXTEN}) rule is enough to hand the call to its
// real destination. Mirrors CreateTenantContext's "_X." rule exactly.
const interServerRelayContext = "asterisk-cluster-relay"

// writeInterServerRelayContext (re)writes the shared relay context's
// dialplan rows. Idempotent — same delete-then-insert pattern as
// CreateTenantContext.
func writeInterServerRelayContext(tx *sql.Tx) error {
	type row struct {
		exten    string
		priority int
		app      string
		appdata  string
	}
	entries := []row{
		{"_X.", 1, "Dial", "PJSIP/${EXTEN},30,rU"},
		{"i", 1, "Hangup", ""},
		{"h", 1, "Hangup", ""},
	}
	for _, e := range entries {
		if _, err := tx.Exec(`DELETE FROM ast_extensions WHERE context=$1 AND exten=$2 AND priority=$3`,
			interServerRelayContext, e.exten, e.priority); err != nil {
			return fmt.Errorf("clear relay context: %w", err)
		}
		if _, err := tx.Exec(`
			INSERT INTO ast_extensions (context, exten, priority, app, appdata)
			VALUES ($1,$2,$3,$4,$5)`,
			interServerRelayContext, e.exten, e.priority, e.app, e.appdata); err != nil {
			return fmt.Errorf("insert relay exten %s: %w", e.exten, err)
		}
	}
	return nil
}

// writeInterServerTrust re-derives the full-mesh set of trust objects that
// let every active Asterisk box accept SIP signaling arriving from every
// other one — required once live registrations (ast_ps_contacts) are shared
// via realtime, so a box can send an INVITE straight to a peer box hosting
// the callee. One identify (by source IP) + a minimal pass-through endpoint
// per ordered pair, same shape as writeProviderPJSIP's identify pattern in
// provider.go; both route into interServerRelayContext.
func writeInterServerTrust(tx *sql.Tx, servers []AsteriskServer) error {
	if err := writeInterServerRelayContext(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM ast_ps_identifies WHERE id LIKE 'identify-asterisk-%'`); err != nil {
		return fmt.Errorf("clear ast_ps_identifies: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM ast_ps_endpoints WHERE id LIKE 'trust-asterisk-%'`); err != nil {
		return fmt.Errorf("clear ast_ps_endpoints: %w", err)
	}

	for _, a := range servers {
		if !a.Active {
			continue
		}
		for _, b := range servers {
			if !b.Active || a.ID == b.ID {
				continue
			}
			// Endpoint on `a` that inbound traffic from `b` is identified
			// against, so the call is accepted and handed to interServerRelayContext.
			endpointID := serverTrustEndpointID(a.ID, b.ID)
			identifyID := serverTrustIdentifyID(a.ID, b.ID)
			if _, err := tx.Exec(`
				INSERT INTO ast_ps_endpoints (id, transport, context, disallow, allow, direct_media)
				VALUES ($1, 'transport-udp', $2, 'all', 'ulaw,alaw', 'no')`,
				endpointID, interServerRelayContext); err != nil {
				return fmt.Errorf("ast_ps_endpoints (trust): %w", err)
			}
			if _, err := tx.Exec(`
				INSERT INTO ast_ps_identifies (id, endpoint, match)
				VALUES ($1, $2, $3)`,
				identifyID, endpointID, b.SIPHost); err != nil {
				return fmt.Errorf("ast_ps_identifies (trust): %w", err)
			}
		}
	}
	return nil
}

// CreateServer adds a new Asterisk server and re-derives the inter-server
// trust mesh for every active server.
func CreateServer(db *sql.DB, s AsteriskServer) (int, error) {
	if s.Name == "" || s.AMIHost == "" || s.WSUri == "" || s.SIPHost == "" {
		return 0, fmt.Errorf("name, amiHost, wsUri and sipHost are required")
	}
	if s.SIPPort == 0 {
		s.SIPPort = 5060
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var id int
	err = tx.QueryRow(`
		INSERT INTO asterisk_servers (name, ami_host, ami_user, ami_pass, ws_uri, sip_host, sip_port, active)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		s.Name, s.AMIHost, s.AMIUser, s.AMIPass, s.WSUri, s.SIPHost, s.SIPPort, s.Active,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("asterisk_servers insert: %w", err)
	}
	s.ID = id

	servers, err := listServersTx(tx)
	if err != nil {
		return 0, err
	}
	if err := writeInterServerTrust(tx, servers); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	log.Printf("[Asterisk] server created: %s (id=%d, ami=%s)", s.Name, id, s.AMIHost)
	return id, nil
}

// UpdateServer changes a server's settings and re-derives the trust mesh.
func UpdateServer(db *sql.DB, s AsteriskServer) error {
	if s.Name == "" || s.AMIHost == "" || s.WSUri == "" || s.SIPHost == "" {
		return fmt.Errorf("name, amiHost, wsUri and sipHost are required")
	}
	if s.SIPPort == 0 {
		s.SIPPort = 5060
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.Exec(`
		UPDATE asterisk_servers SET name=$1, ami_host=$2, ami_user=$3, ami_pass=$4,
		       ws_uri=$5, sip_host=$6, sip_port=$7, active=$8
		WHERE id=$9`,
		s.Name, s.AMIHost, s.AMIUser, s.AMIPass, s.WSUri, s.SIPHost, s.SIPPort, s.Active, s.ID)
	if err != nil {
		return fmt.Errorf("asterisk_servers update: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("server not found")
	}

	servers, err := listServersTx(tx)
	if err != nil {
		return err
	}
	if err := writeInterServerTrust(tx, servers); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	log.Printf("[Asterisk] server updated: %s (id=%d)", s.Name, s.ID)
	return nil
}

// DeleteServer removes a server and re-derives the trust mesh for the rest.
// Refuses while users are still assigned to it — those must be reassigned
// first so no user is silently left pointing at a server that no longer
// exists in our registry.
func DeleteServer(db *sql.DB, id int) error {
	var inUse int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE server_id=$1`, id).Scan(&inUse); err != nil {
		return fmt.Errorf("check users: %w", err)
	}
	if inUse > 0 {
		return fmt.Errorf("server still has %d user(s) assigned", inUse)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM asterisk_servers WHERE id=$1`, id); err != nil {
		return fmt.Errorf("delete server: %w", err)
	}

	servers, err := listServersTx(tx)
	if err != nil {
		return err
	}
	if err := writeInterServerTrust(tx, servers); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	log.Printf("[Asterisk] server deleted: id=%d", id)
	return nil
}

func listServersTx(tx *sql.Tx) ([]AsteriskServer, error) {
	rows, err := tx.Query(`
		SELECT id, name, ami_host, ami_user, ami_pass, ws_uri, sip_host, sip_port, active, created_at
		FROM asterisk_servers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []AsteriskServer{}
	for rows.Next() {
		var s AsteriskServer
		if err := rows.Scan(&s.ID, &s.Name, &s.AMIHost, &s.AMIUser, &s.AMIPass,
			&s.WSUri, &s.SIPHost, &s.SIPPort, &s.Active, &s.CreatedAt); err != nil {
			continue
		}
		result = append(result, s)
	}
	return result, nil
}

// ListServers returns every configured Asterisk server.
func ListServers(db *sql.DB) ([]AsteriskServer, error) {
	rows, err := db.Query(`
		SELECT id, name, ami_host, ami_user, ami_pass, ws_uri, sip_host, sip_port, active, created_at
		FROM asterisk_servers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []AsteriskServer{}
	for rows.Next() {
		var s AsteriskServer
		if err := rows.Scan(&s.ID, &s.Name, &s.AMIHost, &s.AMIUser, &s.AMIPass,
			&s.WSUri, &s.SIPHost, &s.SIPPort, &s.Active, &s.CreatedAt); err != nil {
			continue
		}
		result = append(result, s)
	}
	return result, nil
}

// PickLeastLoadedServer returns the active server with the fewest users
// currently assigned to it (ties broken by lowest id), for auto-assigning a
// newly created user. Returns (0, nil) if no active server is configured —
// callers treat that as "leave server_id NULL", falling back to the single
// default AMI/WS config (single-box deployments keep working unchanged).
func PickLeastLoadedServer(db *sql.DB) (int, error) {
	var id int
	err := db.QueryRow(`
		SELECT s.id
		FROM asterisk_servers s
		LEFT JOIN users u ON u.server_id = s.id
		WHERE s.active = TRUE
		GROUP BY s.id
		ORDER BY COUNT(u.id) ASC, s.id ASC
		LIMIT 1`).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("pick least loaded server: %w", err)
	}
	return id, nil
}
