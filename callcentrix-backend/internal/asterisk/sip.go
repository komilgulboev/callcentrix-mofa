package asterisk

import (
	"database/sql"
	"fmt"
	"log"
)

// CreateSIPAccount creates PJSIP realtime records for WebRTC softphone.
// transport is the PJSIP transport name (e.g. "transport-ws", "transport-wss").
func CreateSIPAccount(db *sql.DB, username, password, context, transport string) error {
	if transport == "" {
		transport = "transport-wss"
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// ── ast_ps_auths ──────────────────────────────────────────
	_, _ = tx.Exec(`DELETE FROM ast_ps_auths WHERE id = $1`, username)
	_, err = tx.Exec(`
		INSERT INTO ast_ps_auths (id, auth_type, password, username)
		VALUES ($1, 'userpass', $2, $1)`,
		username, password,
	)
	if err != nil {
		return fmt.Errorf("ast_ps_auths: %w", err)
	}

	// ── ast_ps_aors ───────────────────────────────────────────
	_, _ = tx.Exec(`DELETE FROM ast_ps_aors WHERE id = $1`, username)
	_, err = tx.Exec(`
		INSERT INTO ast_ps_aors (id, max_contacts, remove_existing, qualify_frequency)
		VALUES ($1, 1, 'yes', 30)`,
		username,
	)
	if err != nil {
		return fmt.Errorf("ast_ps_aors: %w", err)
	}

	// ── ast_ps_endpoints ─────────────────────────────────────
	_, _ = tx.Exec(`DELETE FROM ast_ps_endpoints WHERE id = $1`, username)
	_, err = tx.Exec(`
		INSERT INTO ast_ps_endpoints (
			id, transport, aors, auth, context,
			disallow, allow,
			direct_media,
			webrtc, dtls_auto_generate_cert,
			force_rport, rtp_symmetric
		) VALUES (
			$1, $3, $1, $1, $2,
			'all', 'ulaw,alaw',
			'no',
			'yes', 'yes',
			'yes', 'yes'
		)`,
		username, context, transport,
	)
	if err != nil {
		return fmt.Errorf("ast_ps_endpoints: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	log.Printf("[Asterisk] SIP account created: %s (context: %s)", username, context)
	return nil
}

// DeleteSIPAccount removes PJSIP records for a user.
// Called when a user is deactivated or deleted.
func DeleteSIPAccount(db *sql.DB, username string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	for _, table := range []string{"ast_ps_endpoints", "ast_ps_aors", "ast_ps_auths"} {
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE id = $1`, username); err != nil {
			return fmt.Errorf("delete %s: %w", table, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	log.Printf("[Asterisk] SIP account deleted: %s", username)
	return nil
}

// UpdateSIPContext updates the dialplan context for an endpoint.
// Called when a user is assigned to a tenant.
func UpdateSIPContext(db *sql.DB, username, context string) error {
	_, err := db.Exec(
		`UPDATE ast_ps_endpoints SET context = $1 WHERE id = $2`,
		context, username,
	)
	if err != nil {
		return fmt.Errorf("update context: %w", err)
	}
	log.Printf("[Asterisk] SIP context updated: %s → %s", username, context)
	return nil
}

// CreateTenantContext creates dialplan routing rules for a tenant's internal
// calls, plus its outbound-trunk rule if one is assigned. Writes to the
// Asterisk realtime extensions table.
//
// Context "tenant-{N}" routes:
//   _X.  → Dial(PJSIP/${EXTEN},30,rU)     — internal: agent ↔ agent by username
//   i    → Hangup                           — invalid extension
//   h    → Hangup                           — hangup handler
//   _9X. → Set(CALLERID)+Dial(...@provider) — outbound: dial 9 for an outside
//          line, only written when tenants.outbound_provider_id is set
func CreateTenantContext(db *sql.DB, tenantID int) error {
	ctx := TenantContext(tenantID)

	var outboundProviderID sql.NullInt64
	var outboundCallerID string
	if err := db.QueryRow(
		`SELECT outbound_provider_id, outbound_caller_id FROM tenants WHERE id=$1`, tenantID,
	).Scan(&outboundProviderID, &outboundCallerID); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("load tenant outbound settings: %w", err)
	}

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

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	for _, e := range entries {
		// Delete first to avoid unique constraint issues on ast_extensions
		_, _ = tx.Exec(`DELETE FROM ast_extensions WHERE context=$1 AND exten=$2 AND priority=$3`,
			ctx, e.exten, e.priority)
		_, err := tx.Exec(`
			INSERT INTO ast_extensions (context, exten, priority, app, appdata)
			VALUES ($1,$2,$3,$4,$5)`,
			ctx, e.exten, e.priority, e.app, e.appdata)
		if err != nil {
			return fmt.Errorf("insert exten %s: %w", e.exten, err)
		}
	}

	// Outbound "dial 9 for an outside line" rule. Cleared first regardless,
	// so unassigning a tenant's outbound trunk cleanly removes the ability
	// to dial out; rewritten below only if a trunk is currently assigned.
	if _, err := tx.Exec(`DELETE FROM ast_extensions WHERE context=$1 AND exten='_9X.'`, ctx); err != nil {
		return fmt.Errorf("clear outbound rule: %w", err)
	}
	if outboundProviderID.Valid {
		callerIDStep := row{"_9X.", 1, "NoOp", ""}
		if outboundCallerID != "" {
			callerIDStep = row{"_9X.", 1, "Set", "CALLERID(num)=" + outboundCallerID}
		}
		outboundEntries := []row{
			callerIDStep,
			{"_9X.", 2, "Dial", fmt.Sprintf("PJSIP/${EXTEN:1}@%s,60,r", ProviderEndpointID(int(outboundProviderID.Int64)))},
			{"_9X.", 3, "Hangup", ""},
		}
		for _, e := range outboundEntries {
			if _, err := tx.Exec(`
				INSERT INTO ast_extensions (context, exten, priority, app, appdata)
				VALUES ($1,$2,$3,$4,$5)`,
				ctx, e.exten, e.priority, e.app, e.appdata); err != nil {
				return fmt.Errorf("insert outbound exten: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	log.Printf("[Asterisk] Dialplan context created: %s", ctx)
	return nil
}

// DeleteTenantContext removes all dialplan rules for a tenant's context.
// Called when a tenant is deleted.
func DeleteTenantContext(db *sql.DB, tenantID int) error {
	ctx := TenantContext(tenantID)
	_, err := db.Exec(`DELETE FROM ast_extensions WHERE context = $1`, ctx)
	if err != nil {
		return fmt.Errorf("delete context %s: %w", ctx, err)
	}
	log.Printf("[Asterisk] Dialplan context deleted: %s", ctx)
	return nil
}

// AddToQueue adds an operator to an Asterisk queue.
func AddToQueue(db *sql.DB, queueName, username string) error {
	iface := "PJSIP/" + username
	// Delete first to avoid unique constraint issues, then insert fresh
	_, _ = db.Exec(`DELETE FROM ast_queue_members WHERE queue_name=$1 AND interface=$2`, queueName, iface)
	_, err := db.Exec(`
		INSERT INTO ast_queue_members
			(queue_name, interface, membername, penalty, paused, wrapuptime)
		VALUES ($1, $2, $3, 0, 0, 0)`,
		queueName, iface, username,
	)
	return err
}

// RemoveFromQueue removes an operator from a queue.
func RemoveFromQueue(db *sql.DB, queueName, username string) error {
	iface := "PJSIP/" + username
	_, err := db.Exec(
		`DELETE FROM ast_queue_members WHERE queue_name=$1 AND interface=$2`,
		queueName, iface,
	)
	return err
}

// TenantContext returns the dialplan context name for a tenant.
// Convention: "tenant-{tenantId}"
func TenantContext(tenantID int) string {
	return fmt.Sprintf("tenant-%d", tenantID)
}

// IVRDialplanOption represents a single DTMF option for the IVR dialplan.
type IVRDialplanOption struct {
	Digit   string
	App     string
	AppData string
}
