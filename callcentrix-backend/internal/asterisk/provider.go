package asterisk

import (
	"database/sql"
	"fmt"
	"log"
	"regexp"
)

// Provider is a telecom carrier's SIP trunk. SuperAdmin-managed; KC numbers
// are created against a provider so inbound routing knows which trunk a DID
// belongs to. One provider maps to one PJSIP endpoint (+aor+auth+identify,
// optionally +registration) in the realtime tables — see writeProviderPJSIP.
type Provider struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Transport string `json:"transport"`
	Codecs    string `json:"codecs"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	Register  bool   `json:"register"`
	CreatedAt string `json:"createdAt"`
}

// ProviderEndpointID returns the PJSIP endpoint id for a provider's trunk.
func ProviderEndpointID(providerID int) string {
	return fmt.Sprintf("provider-%d", providerID)
}

func providerAorID(providerID int) string      { return fmt.Sprintf("aor-provider-%d", providerID) }
func providerAuthID(providerID int) string     { return fmt.Sprintf("auth-provider-%d", providerID) }
func providerIdentifyID(providerID int) string { return fmt.Sprintf("identify-provider-%d", providerID) }
// ProviderRegistrationID returns the PJSIP outbound-registration id for a
// provider's trunk (only meaningful when the provider has Register enabled).
func ProviderRegistrationID(providerID int) string { return fmt.Sprintf("reg-provider-%d", providerID) }

// providerNamePattern restricts provider names to what's safe to use
// verbatim as an Asterisk dialplan context name (see writeProviderPJSIP) —
// English letters/digits/underscore/hyphen only, starting with a letter.
// Cyrillic or other non-ASCII names would silently fail to resolve as a
// context: realtime dialplan only matches the exact context name declared in
// extensions.conf, and that file can only hold what you type into it there.
var providerNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

func validateProviderName(name string) error {
	if !providerNamePattern.MatchString(name) {
		return fmt.Errorf("provider name must start with an English letter and contain only English letters, digits, '_' or '-' (it's used directly as the Asterisk dialplan context name)")
	}
	return nil
}

// providerNameTaken reports whether another provider already uses this name
// (case-insensitive) — the name doubles as the dialplan context name, so two
// providers sharing one would collide in ast_extensions (in particular their
// "s" anonymous-fallback rows, see syncProviderAnonymousFallback). excludeID
// excludes the provider being updated from the check; pass 0 when creating.
func providerNameTaken(tx *sql.Tx, name string, excludeID int) (bool, error) {
	var cnt int
	err := tx.QueryRow(`SELECT COUNT(*) FROM providers WHERE lower(name) = lower($1) AND id != $2`, name, excludeID).Scan(&cnt)
	return cnt > 0, err
}

// providerNameByID looks up a provider's current name, e.g. to know which
// dialplan context a KC number's rows belong in (see writeKCDialplan).
func providerNameByID(tx *sql.Tx, providerID int) (string, error) {
	var name string
	err := tx.QueryRow(`SELECT name FROM providers WHERE id = $1`, providerID).Scan(&name)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("provider not found")
	}
	return name, err
}

// writeProviderPJSIP (re)writes the PJSIP realtime rows for a provider's
// trunk: endpoint (context=<provider name>, so inbound calls land directly
// in that provider's own dialplan — see writeKCDialplan and
// syncProviderAnonymousFallback) + aor (points at the carrier host) +
// identify (matches inbound INVITEs from the carrier's IP to this endpoint)
// + auth, and — if register is set — an outbound registration so Asterisk
// logs in to the carrier. Mirrors CreateSIPAccount's realtime-write pattern
// (sip.go). The provider's name must already have been statically declared
// as a context in extensions.conf (switch => Realtime/@extensions) — that's
// a one-time manual step per provider, not something this backend can do,
// since it doesn't have filesystem access to the Asterisk config.
func writeProviderPJSIP(tx *sql.Tx, p Provider) error {
	endpointID := ProviderEndpointID(p.ID)
	aorID := providerAorID(p.ID)
	authID := providerAuthID(p.ID)
	identifyID := providerIdentifyID(p.ID)
	regID := ProviderRegistrationID(p.ID)
	hostURI := fmt.Sprintf("sip:%s:%d", p.Host, p.Port)

	if _, err := tx.Exec(`DELETE FROM ast_ps_auths WHERE id = $1`, authID); err != nil {
		return fmt.Errorf("clear ast_ps_auths: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO ast_ps_auths (id, auth_type, username, password)
		VALUES ($1, 'userpass', $2, $3)`,
		authID, p.Username, p.Password); err != nil {
		return fmt.Errorf("ast_ps_auths: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM ast_ps_aors WHERE id = $1`, aorID); err != nil {
		return fmt.Errorf("clear ast_ps_aors: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO ast_ps_aors (id, contact)
		VALUES ($1, $2)`,
		aorID, hostURI); err != nil {
		return fmt.Errorf("ast_ps_aors: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM ast_ps_endpoints WHERE id = $1`, endpointID); err != nil {
		return fmt.Errorf("clear ast_ps_endpoints: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO ast_ps_endpoints (
			id, transport, aors, auth, context,
			disallow, allow,
			direct_media, force_rport, rtp_symmetric, rewrite_contact,
			from_user, from_domain
		) VALUES (
			$1, $2, $3, $4, $5,
			'all', $6,
			'no', 'yes', 'yes', 'yes',
			$7, $8
		)`,
		endpointID, p.Transport, aorID, authID, p.Name, p.Codecs, p.Username, p.Host); err != nil {
		return fmt.Errorf("ast_ps_endpoints: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM ast_ps_identifies WHERE id = $1`, identifyID); err != nil {
		return fmt.Errorf("clear ast_ps_identifies: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO ast_ps_identifies (id, endpoint, match)
		VALUES ($1, $2, $3)`,
		identifyID, endpointID, p.Host); err != nil {
		return fmt.Errorf("ast_ps_identifies: %w", err)
	}

	// Outbound registration is only meaningful when the carrier requires it;
	// IP-authenticated trunks skip it entirely (identify above is enough).
	if _, err := tx.Exec(`DELETE FROM ast_ps_registrations WHERE id = $1`, regID); err != nil {
		return fmt.Errorf("clear ast_ps_registrations: %w", err)
	}
	if p.Register {
		clientURI := fmt.Sprintf("sip:%s@%s", p.Username, p.Host)
		serverURI := fmt.Sprintf("sip:%s", p.Host)
		if _, err := tx.Exec(`
			INSERT INTO ast_ps_registrations
				(id, transport, outbound_auth, server_uri, client_uri, retry_interval)
			VALUES ($1, $2, $3, $4, $5, 60)`,
			regID, p.Transport, authID, serverURI, clientURI); err != nil {
			return fmt.Errorf("ast_ps_registrations: %w", err)
		}
	}

	return nil
}

// deleteProviderPJSIP removes every realtime row a provider's trunk owns.
func deleteProviderPJSIP(tx *sql.Tx, providerID int) error {
	rows := []struct{ table, id string }{
		{"ast_ps_registrations", ProviderRegistrationID(providerID)},
		{"ast_ps_identifies", providerIdentifyID(providerID)},
		{"ast_ps_endpoints", ProviderEndpointID(providerID)},
		{"ast_ps_aors", providerAorID(providerID)},
		{"ast_ps_auths", providerAuthID(providerID)},
	}
	for _, r := range rows {
		if _, err := tx.Exec(`DELETE FROM `+r.table+` WHERE id = $1`, r.id); err != nil {
			return fmt.Errorf("delete %s: %w", r.table, err)
		}
	}
	return nil
}

// CreateProvider adds a new carrier trunk: the providers row plus its PJSIP
// realtime rows (endpoint/aor/auth/identify/registration).
func CreateProvider(db *sql.DB, p Provider) (int, error) {
	if p.Name == "" || p.Host == "" {
		return 0, fmt.Errorf("name and host required")
	}
	if err := validateProviderName(p.Name); err != nil {
		return 0, err
	}
	if p.Port == 0 {
		p.Port = 5060
	}
	if p.Transport == "" {
		p.Transport = "transport-udp"
	}
	if p.Codecs == "" {
		p.Codecs = "ulaw,alaw"
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if taken, err := providerNameTaken(tx, p.Name, 0); err != nil {
		return 0, fmt.Errorf("check provider name: %w", err)
	} else if taken {
		return 0, fmt.Errorf("provider name %q is already in use", p.Name)
	}

	var id int
	err = tx.QueryRow(`
		INSERT INTO providers (name, host, port, transport, codecs, username, password, register)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		p.Name, p.Host, p.Port, p.Transport, p.Codecs, p.Username, p.Password, p.Register,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("providers insert: %w", err)
	}
	p.ID = id

	if err := writeProviderPJSIP(tx, p); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	log.Printf("[Asterisk] Provider created: %s (id=%d, host=%s:%d)", p.Name, id, p.Host, p.Port)
	return id, nil
}

// UpdateProvider changes a carrier trunk's settings and rewrites its PJSIP
// realtime rows to match.
func UpdateProvider(db *sql.DB, p Provider) error {
	if p.Name == "" || p.Host == "" {
		return fmt.Errorf("name and host required")
	}
	if err := validateProviderName(p.Name); err != nil {
		return err
	}
	if p.Port == 0 {
		p.Port = 5060
	}
	if p.Transport == "" {
		p.Transport = "transport-udp"
	}
	if p.Codecs == "" {
		p.Codecs = "ulaw,alaw"
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if taken, err := providerNameTaken(tx, p.Name, p.ID); err != nil {
		return fmt.Errorf("check provider name: %w", err)
	} else if taken {
		return fmt.Errorf("provider name %q is already in use", p.Name)
	}

	res, err := tx.Exec(`
		UPDATE providers SET name=$1, host=$2, port=$3, transport=$4, codecs=$5,
		       username=$6, password=$7, register=$8
		WHERE id=$9`,
		p.Name, p.Host, p.Port, p.Transport, p.Codecs, p.Username, p.Password, p.Register, p.ID)
	if err != nil {
		return fmt.Errorf("providers update: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("provider not found")
	}

	if err := writeProviderPJSIP(tx, p); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	log.Printf("[Asterisk] Provider updated: %s (id=%d)", p.Name, p.ID)
	return nil
}

// DeleteProvider removes a carrier trunk and its PJSIP realtime rows. Refuses
// to delete while KC numbers or a tenant's outbound trunk still reference
// it — those must be reassigned or removed first so a tenant's inbound
// routing or outbound dialing never silently loses its trunk.
func DeleteProvider(db *sql.DB, providerID int) error {
	var inUse int
	if err := db.QueryRow(`SELECT COUNT(*) FROM kc_numbers WHERE provider_id=$1`, providerID).Scan(&inUse); err != nil {
		return fmt.Errorf("check kc_numbers: %w", err)
	}
	if inUse > 0 {
		return fmt.Errorf("provider still has %d KC number(s) attached", inUse)
	}

	var outboundUse int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tenants WHERE outbound_provider_id=$1`, providerID).Scan(&outboundUse); err != nil {
		return fmt.Errorf("check tenants: %w", err)
	}
	if outboundUse > 0 {
		return fmt.Errorf("provider is still the outbound trunk for %d tenant(s)", outboundUse)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := deleteProviderPJSIP(tx, providerID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM providers WHERE id=$1`, providerID); err != nil {
		return fmt.Errorf("delete provider: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	log.Printf("[Asterisk] Provider deleted: id=%d", providerID)
	return nil
}

// ListProviders returns every carrier trunk.
func ListProviders(db *sql.DB) ([]Provider, error) {
	rows, err := db.Query(`
		SELECT id, name, host, port, transport, codecs, username, password, register, created_at
		FROM providers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []Provider{}
	for rows.Next() {
		var p Provider
		if err := rows.Scan(&p.ID, &p.Name, &p.Host, &p.Port, &p.Transport, &p.Codecs,
			&p.Username, &p.Password, &p.Register, &p.CreatedAt); err != nil {
			continue
		}
		result = append(result, p)
	}
	return result, nil
}
