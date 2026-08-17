package asterisk

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
)

// KCQueueName returns the Asterisk queue name backing a KC number.
// The id here is the kc_numbers.id (not the tenant id) so it stays globally
// unique even when a tenant owns several KC numbers.
func KCQueueName(kcNumberID int) string {
	return fmt.Sprintf("queue_tenant_%d", kcNumberID)
}

// publicBase/asteriskKey are set once at startup via Configure() so dialplan
// generation can build a CURL() URL back into this same backend (used by the
// shared whitelist-check subroutine). See cmd/server/main.go.
var publicBase, asteriskKey string

// Configure records the values dialplan generation needs to call back into
// this backend (HTTP_PUBLIC_BASE / ASTERISK_KEY). Call once at startup.
func Configure(publicBaseURL, asteriskKeyVal string) {
	publicBase = publicBaseURL
	asteriskKey = asteriskKeyVal
}

// EnsureWhitelistCheckSubroutine writes the shared [whitelist-check] Gosub
// context: Gosub(whitelist-check,s,1(${CALLERID(num)},<tenantId>)) from any
// KC number's dialplan returns "1"/"0" in GOSUB_RETVAL via our existing
// /internal/whitelist/check endpoint. Written once (not per tenant/number);
// safe to call on every startup — clears and rewrites the same context.
func EnsureWhitelistCheckSubroutine(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM ast_extensions WHERE context='whitelist-check'`); err != nil {
		return fmt.Errorf("clear whitelist-check context: %w", err)
	}

	url := fmt.Sprintf("%s/internal/whitelist/check?phone=${ARG1}&tenantId=${ARG2}&key=%s", publicBase, asteriskKey)
	entries := []struct{ p int; app, data string }{
		{1, "NoOp", "Whitelist check for ${ARG1} (tenant ${ARG2})"},
		{2, "Set", fmt.Sprintf("WLOK=${CURL(%s)}", url)},
		{3, "Return", "${WLOK}"},
	}
	for _, e := range entries {
		if _, err := tx.Exec(`INSERT INTO ast_extensions (context,exten,priority,app,appdata) VALUES ('whitelist-check','s',$1,$2,$3)`,
			e.p, e.app, e.data); err != nil {
			return fmt.Errorf("insert whitelist-check s,%d: %w", e.p, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	log.Printf("[Asterisk] whitelist-check subroutine synced")
	return nil
}

// EnsureBlockedContext writes the shared "blocked" context (Answer → privacy
// notice → Hangup), reached via Goto(blocked,s,1) from any provider's own
// dialplan when whitelist-check rejects a caller. Like whitelist-check, this
// name is one of the small, fixed set statically declared once in
// extensions.conf — written once (not per provider), safe to call on every
// startup.
func EnsureBlockedContext(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM ast_extensions WHERE context='blocked'`); err != nil {
		return fmt.Errorf("clear blocked: %w", err)
	}

	entries := []struct{ p int; app, data string }{
		{1, "Answer", ""},
		{2, "Playback", "privacy-sorry"},
		{3, "Hangup", ""},
	}
	for _, e := range entries {
		if _, err := tx.Exec(`INSERT INTO ast_extensions (context,exten,priority,app,appdata) VALUES ('blocked','s',$1,$2,$3)`,
			e.p, e.app, e.data); err != nil {
			return fmt.Errorf("insert blocked s,%d: %w", e.p, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	log.Printf("[Asterisk] blocked context synced")
	return nil
}

// syncProviderAnonymousFallback (re)writes a provider's "s" extension —
// reached when the carrier's INVITE carries no DID digits at all (some
// single-DID trunks never populate the Request-URI's user part) — inside
// that provider's own dialplan context (its name; see writeProviderPJSIP).
// Only written when the provider owns exactly one KC number: with no digits
// to go on, a call to "s" can't be told apart between several numbers on the
// same trunk, so multi-number (and number-less) providers get no "s" handler
// at all and such calls are simply rejected, same as an unmatched DID.
// Call whenever a KC number is added to or removed from a provider, since
// that changes whether this applies.
func syncProviderAnonymousFallback(tx *sql.Tx, providerID int) error {
	providerName, err := providerNameByID(tx, providerID)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`DELETE FROM ast_extensions WHERE context=$1 AND exten='s'`, providerName); err != nil {
		return fmt.Errorf("clear %s s: %w", providerName, err)
	}

	rows, err := tx.Query(`SELECT id, tenant_id FROM kc_numbers WHERE provider_id = $1`, providerID)
	if err != nil {
		return fmt.Errorf("lookup provider kc_numbers: %w", err)
	}
	type owned struct{ id, tenantID int }
	var nums []owned
	for rows.Next() {
		var n owned
		if err := rows.Scan(&n.id, &n.tenantID); err != nil {
			rows.Close()
			return err
		}
		nums = append(nums, n)
	}
	rows.Close()
	if len(nums) != 1 {
		return nil
	}
	kcID, tenantID := nums[0].id, nums[0].tenantID

	entries := []struct{ p int; app, data string }{
		{1, "NoOp", fmt.Sprintf("Anonymous inbound, tenant %d", tenantID)},
		{2, "Answer", ""},
		{3, "Gosub", fmt.Sprintf("whitelist-check,s,1(${CALLERID(num)},%d)", tenantID)},
		{4, "GotoIf", `$["${GOSUB_RETVAL}" = "0"]?blocked,s,1`},
		// 'c': continue in the dialplan (not hang up) if the connected
		// agent's leg disappears mid-call — the Wait() below is what
		// actually holds the caller for a possible reconnect; see
		// writeKCDialplan for the matching, more-detailed comment.
		{5, "Queue", fmt.Sprintf("%s,rHhc,,300", KCQueueName(kcID))},
		{6, "Wait", "5"},
		{7, "Hangup", ""},
	}
	for _, e := range entries {
		if _, err := tx.Exec(`INSERT INTO ast_extensions (context,exten,priority,app,appdata) VALUES ($1,'s',$2,$3,$4)`,
			providerName, e.p, e.app, e.data); err != nil {
			return fmt.Errorf("insert %s s,%d: %w", providerName, e.p, err)
		}
	}
	return nil
}

// kcNumberOwner returns the tenant_id that owns a kc_number, or 0 if not found.
func kcNumberOwner(db *sql.DB, kcNumberID int) (int, error) {
	var tenantID int
	err := db.QueryRow(`SELECT tenant_id FROM kc_numbers WHERE id = $1`, kcNumberID).Scan(&tenantID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return tenantID, nil
}

// KCNumber is a tenant's call-center DID plus a summary of what's configured
// for it, for the admin's overview table (Приветствие/Меню/Очередь/Операторы).
type KCNumber struct {
	ID           int    `json:"id"`
	TenantID     int    `json:"tenantId"`
	ProviderID   int    `json:"providerId"`
	ProviderName string `json:"providerName"`
	Number       string `json:"number"`
	HasGreeting  bool   `json:"hasGreeting"`
	OptionsCount int    `json:"optionsCount"`
	QueueMembers int    `json:"queueMembers"`
	CreatedAt    string `json:"createdAt"`
}

// WorkHours describes an optional daily schedule gating a KC number's queue.
// Outside the window, the caller hits "closed" instead of "open" (see
// writeKCDialplan) — plays ClosedGreeting then hangs up.
type WorkHours struct {
	Enabled        bool
	Start, End     string // "HH:MM"
	Days           string // Asterisk weekday list/range, e.g. "mon-fri" or "mon,tue,wed"
	ClosedGreeting string
}

// dialplanBuilder assembles a linear sequence of realtime dialplan priorities
// for one context+exten, resolving forward Goto/GotoIf targets to concrete
// priority numbers once the whole sequence is known. Realtime dialplan only
// resolves context names statically declared once in extensions.conf — exten
// values and priority counts are fully dynamic — so a KC number's whole flow
// lives as priorities under its own exten (the DID) in its provider's own
// context, not a uniquely-named context per number (which would need its own
// extensions.conf stanza and reload every time a number is created).
type dialplanBuilder struct {
	exten string
	rows  []struct{ app, tmpl string }
	marks map[string]int
}

func newDialplanBuilder(exten string) *dialplanBuilder {
	return &dialplanBuilder{exten: exten, marks: map[string]int{}}
}

// mark names the priority that will be assigned to the next row added, so
// earlier rows can jump to it by writing {{name}} in their appdata template.
func (b *dialplanBuilder) mark(name string) {
	b.marks[name] = len(b.rows) + 1
}

func (b *dialplanBuilder) add(app, tmpl string) {
	b.rows = append(b.rows, struct{ app, tmpl string }{app, tmpl})
}

// writeTo resolves every {{mark}} placeholder to its priority number and
// inserts the rows into context, one priority per row in order.
func (b *dialplanBuilder) writeTo(tx *sql.Tx, context string) error {
	for i, r := range b.rows {
		data := r.tmpl
		for name, p := range b.marks {
			data = strings.ReplaceAll(data, "{{"+name+"}}", strconv.Itoa(p))
		}
		if _, err := tx.Exec(`INSERT INTO ast_extensions (context,exten,priority,app,appdata) VALUES ($1,$2,$3,$4,$5)`,
			context, b.exten, i+1, r.app, data); err != nil {
			return fmt.Errorf("insert %s,%d: %w", b.exten, i+1, err)
		}
	}
	return nil
}

// writeKCDialplan (re)writes a KC number's inbound flow directly under
// <providerContext>,<number>: whitelist check (Gosub into the shared
// subroutine, see EnsureWhitelistCheckSubroutine) → shared blocked context if
// rejected (see EnsureBlockedContext) → optional work-hours gate → Answer →
// greeting → optional digit menu → default queue. The digit menu uses
// Read()+GotoIf rather than Background+WaitExten: WaitExten's automatic
// "t"/"i" timeout/invalid-digit fallback resolves relative to the *context*,
// not the exten, and every number on the same provider shares one context —
// Read() keeps everything on explicit priorities within this number's own
// exten instead, so different numbers' menus can never collide.
// Shared by CreateKCNumber (initial, no menu/schedule yet) and
// SyncKCNumberDialplan (after greeting/menu/schedule edits). Runs inside tx
// so it's atomic with whatever else the caller is doing in the same transaction.
func writeKCDialplan(tx *sql.Tx, context string, tenantID int, number, queueName, greeting string, waitTimeout, queueTimeout int, options []IVRDialplanOption, wh WorkHours) error {
	if _, err := tx.Exec(`DELETE FROM ast_extensions WHERE context=$1 AND exten=$2`, context, number); err != nil {
		return fmt.Errorf("clear number's dialplan: %w", err)
	}
	if greeting == "" {
		greeting = "beep"
	}
	closedGreeting := wh.ClosedGreeting
	if closedGreeting == "" {
		closedGreeting = "beep"
	}
	// 'c': continue in the dialplan (instead of hanging up the caller
	// outright) when the connected agent's leg disappears mid-call — see the
	// Wait() right after Queue() below, which is what actually uses that.
	queueAppData := fmt.Sprintf("%s,rHhc,,%d", queueName, queueTimeout)

	b := newDialplanBuilder(number)
	b.add("NoOp", fmt.Sprintf("KC number %s (tenant %d)", number, tenantID))
	b.add("Gosub", fmt.Sprintf("whitelist-check,s,1(${CALLERID(num)},%d)", tenantID))
	b.add("GotoIf", `$["${GOSUB_RETVAL}" = "0"]?blocked,s,1`)

	if wh.Enabled {
		b.add("GotoIfTime", fmt.Sprintf("%s-%s,%s,*,*?{{open}}", wh.Start, wh.End, wh.Days))
		b.add("Answer", "")
		b.add("Background", closedGreeting)
		b.add("Goto", "{{hangup}}")
	}

	b.mark("open")
	b.add("Answer", "")

	if len(options) > 0 {
		b.mark("replay")
		b.add("Read", fmt.Sprintf("DIGIT,%s,1,,,%d", greeting, waitTimeout))
		for _, opt := range options {
			b.add("GotoIf", fmt.Sprintf(`$["${DIGIT}" = "%s"]?{{opt-%s}}`, opt.Digit, opt.Digit))
		}
		b.add("GotoIf", `$["${DIGIT}" = ""]?{{queue}}`)
		b.add("Goto", "{{replay}}") // invalid digit → replay greeting
		for _, opt := range options {
			b.mark("opt-" + opt.Digit)
			b.add(opt.App, opt.AppData)
			if opt.App == "Queue" {
				// Same reconnect-hold pattern as the default queue below —
				// only meaningful here if opt.AppData also carries Queue()'s
				// 'c' option (see IVRHandler.Sync, which builds it).
				b.add("Wait", "5")
			}
			b.add("Goto", "{{hangup}}")
		}
	} else {
		b.add("Background", greeting)
	}

	b.mark("queue")
	b.add("Queue", queueAppData)
	// Reached only if the agent's leg vanished mid-call (see the 'c' option
	// above) — hold the caller briefly in case that agent's browser
	// reconnects (see ami.Monitor.pendingReconnect / PhoneHandler.ResumeCall,
	// which redirects this channel straight back into a fresh Dial() at the
	// agent's extension if they do). Duration must match pendingReconnectTTL.
	b.add("Wait", "5")
	b.mark("hangup")
	b.add("Hangup", "")

	return b.writeTo(tx, context)
}

// CreateKCNumber provisions a new call-center number for a tenant: the
// kc_numbers row, its own IVR config + queue, and its inbound dialplan flow
// (written directly under <providerName>,<number> — see writeKCDialplan).
// providerID identifies which carrier trunk this DID arrives on (see
// internal/asterisk/provider.go) — required, since the provider's name is
// what the dialplan context is named after; there's no trunk-less context
// left to fall back to.
func CreateKCNumber(db *sql.DB, tenantID, providerID int, number string) (int, error) {
	if number == "" {
		return 0, fmt.Errorf("number required")
	}
	if providerID <= 0 {
		return 0, fmt.Errorf("provider required")
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	providerName, err := providerNameByID(tx, providerID)
	if err != nil {
		return 0, err
	}

	var id int
	err = tx.QueryRow(`INSERT INTO kc_numbers (tenant_id, provider_id, number) VALUES ($1,$2,$3) RETURNING id`,
		tenantID, providerID, number).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("kc_numbers insert: %w", err)
	}

	if _, err := tx.Exec(`INSERT INTO ivr_configs (tenant_id, kc_number_id, did_number) VALUES ($1,$2,$3)`,
		tenantID, id, number); err != nil {
		return 0, fmt.Errorf("ivr_configs insert: %w", err)
	}

	queueName := KCQueueName(id)
	if _, err := tx.Exec(`
		INSERT INTO ast_queues
			(name, strategy, timeout, maxlen, ringinuse, joinempty, leavewhenempty, musiconhold, retry, wrapuptime)
		VALUES ($1,'ringall',15,0,'no','yes','no','default',5,0)`, queueName); err != nil {
		return 0, fmt.Errorf("ast_queues insert: %w", err)
	}

	if err := writeKCDialplan(tx, providerName, tenantID, number, queueName, "beep", 5, 300, nil, WorkHours{}); err != nil {
		return 0, err
	}

	if err := syncProviderAnonymousFallback(tx, providerID); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	log.Printf("[Asterisk] KC number created: %s (id=%d, tenant=%d)", number, id, tenantID)
	return id, nil
}

// DeleteKCNumber tears down everything owned by a KC number: inbound route,
// dialplan context, queue (+members), IVR options/config, and the number itself.
func DeleteKCNumber(db *sql.DB, tenantID, kcNumberID int) error {
	owner, err := kcNumberOwner(db, kcNumberID)
	if err != nil {
		return fmt.Errorf("lookup kc number: %w", err)
	}
	if owner == 0 {
		return fmt.Errorf("kc number not found")
	}
	if owner != tenantID {
		return fmt.Errorf("kc number does not belong to this tenant")
	}

	var number string
	var providerID int
	_ = db.QueryRow(`SELECT number, COALESCE(provider_id,0) FROM kc_numbers WHERE id=$1`, kcNumberID).Scan(&number, &providerID)

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	queueName := KCQueueName(kcNumberID)

	if number != "" && providerID > 0 {
		providerName, err := providerNameByID(tx, providerID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM ast_extensions WHERE context=$1 AND exten=$2`, providerName, number); err != nil {
			return fmt.Errorf("delete dialplan: %w", err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM ast_queue_members WHERE queue_name=$1`, queueName); err != nil {
		return fmt.Errorf("delete queue members: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM ast_queues WHERE name=$1`, queueName); err != nil {
		return fmt.Errorf("delete queue: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM ivr_options WHERE kc_number_id=$1`, kcNumberID); err != nil {
		return fmt.Errorf("delete ivr options: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM ivr_configs WHERE kc_number_id=$1`, kcNumberID); err != nil {
		return fmt.Errorf("delete ivr config: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM kc_numbers WHERE id=$1`, kcNumberID); err != nil {
		return fmt.Errorf("delete kc number: %w", err)
	}

	if providerID > 0 {
		if err := syncProviderAnonymousFallback(tx, providerID); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	log.Printf("[Asterisk] KC number deleted: %s (id=%d, tenant=%d)", number, kcNumberID, tenantID)
	return nil
}

// ListKCNumbers returns every number owned by a tenant with a config-status summary.
func ListKCNumbers(db *sql.DB, tenantID int) ([]KCNumber, error) {
	rows, err := db.Query(`
		SELECT kn.id, kn.tenant_id, COALESCE(kn.provider_id, 0), COALESCE(p.name, ''),
		       kn.number, kn.created_at,
		       COALESCE(ic.greeting_file, ''),
		       (SELECT COUNT(*) FROM ivr_options io WHERE io.kc_number_id = kn.id)
		FROM kc_numbers kn
		LEFT JOIN ivr_configs ic ON ic.kc_number_id = kn.id
		LEFT JOIN providers p ON p.id = kn.provider_id
		WHERE kn.tenant_id = $1
		ORDER BY kn.number`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []KCNumber{}
	for rows.Next() {
		var n KCNumber
		var greeting string
		if err := rows.Scan(&n.ID, &n.TenantID, &n.ProviderID, &n.ProviderName, &n.Number, &n.CreatedAt, &greeting, &n.OptionsCount); err != nil {
			continue
		}
		n.HasGreeting = greeting != ""
		_ = db.QueryRow(`SELECT COUNT(*) FROM ast_queue_members WHERE queue_name=$1`, KCQueueName(n.ID)).Scan(&n.QueueMembers)
		result = append(result, n)
	}
	return result, nil
}

// UpsertKCQueue creates or updates the Asterisk queue backing a KC number.
func UpsertKCQueue(db *sql.DB, kcNumberID int, strategy string, timeout, maxLen int) error {
	name := KCQueueName(kcNumberID)
	if strategy == "" {
		strategy = "ringall"
	}
	if timeout == 0 {
		timeout = 15
	}
	res, err := db.Exec(`
		UPDATE ast_queues SET strategy=$1, timeout=$2, maxlen=$3
		WHERE name=$4`, strategy, timeout, maxLen, name)
	if err != nil {
		return fmt.Errorf("update queue %s: %w", name, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		_, err = db.Exec(`
			INSERT INTO ast_queues
				(name, strategy, timeout, maxlen, ringinuse, joinempty, leavewhenempty, musiconhold, retry, wrapuptime)
			VALUES ($1,$2,$3,$4,'no','yes','no','default',5,0)`,
			name, strategy, timeout, maxLen)
		if err != nil {
			return fmt.Errorf("insert queue %s: %w", name, err)
		}
	}
	log.Printf("[Asterisk] KC queue upserted: %s (strategy=%s)", name, strategy)
	return nil
}

// SyncKCNumberDialplan rebuilds the IVR dialplan context for a single KC number.
// Called after IVR config/menu changes. options is a list of {digit, app, appdata}.
func SyncKCNumberDialplan(db *sql.DB, kcNumberID int, greetingFile string, waitTimeout, queueTimeout int, options []IVRDialplanOption, wh WorkHours) error {
	tenantID, err := kcNumberOwner(db, kcNumberID)
	if err != nil {
		return fmt.Errorf("lookup kc number: %w", err)
	}
	if tenantID == 0 {
		return fmt.Errorf("kc number not found")
	}
	var number string
	var providerID int
	_ = db.QueryRow(`SELECT number, COALESCE(provider_id,0) FROM kc_numbers WHERE id=$1`, kcNumberID).Scan(&number, &providerID)
	if providerID == 0 {
		return fmt.Errorf("kc number has no provider assigned")
	}

	queueName := KCQueueName(kcNumberID)

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	providerName, err := providerNameByID(tx, providerID)
	if err != nil {
		return err
	}

	if err := writeKCDialplan(tx, providerName, tenantID, number, queueName, greetingFile, waitTimeout, queueTimeout, options, wh); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	log.Printf("[Asterisk] KC IVR dialplan synced: %s (%d options)", number, len(options))
	return nil
}

// AddMemberToKCQueue adds a tenant user to a KC number's queue.
func AddMemberToKCQueue(db *sql.DB, tenantID, kcNumberID int, username string) error {
	owner, err := kcNumberOwner(db, kcNumberID)
	if err != nil {
		return fmt.Errorf("lookup kc number: %w", err)
	}
	if owner == 0 {
		return fmt.Errorf("kc number not found")
	}
	if owner != tenantID {
		return fmt.Errorf("kc number does not belong to this tenant")
	}

	var cnt int
	_ = db.QueryRow(`SELECT COUNT(*) FROM users WHERE username=$1 AND tenant_id=$2`, username, tenantID).Scan(&cnt)
	if cnt == 0 {
		return fmt.Errorf("user not found in this tenant")
	}

	queueName := KCQueueName(kcNumberID)
	iface := "PJSIP/" + username
	_, _ = db.Exec(`DELETE FROM ast_queue_members WHERE queue_name=$1 AND interface=$2`, queueName, iface)
	_, err = db.Exec(`
		INSERT INTO ast_queue_members
			(queue_name, interface, membername, penalty, paused, wrapuptime, tenant_id)
		VALUES ($1,$2,$3,0,0,0,$4)`,
		queueName, iface, username, tenantID,
	)
	if err != nil {
		return fmt.Errorf("add member %s to queue %s: %w", username, queueName, err)
	}
	return nil
}

// RemoveMemberFromKCQueue removes a user from a KC number's queue.
func RemoveMemberFromKCQueue(db *sql.DB, tenantID, kcNumberID int, username string) error {
	owner, err := kcNumberOwner(db, kcNumberID)
	if err != nil {
		return fmt.Errorf("lookup kc number: %w", err)
	}
	if owner == 0 {
		return fmt.Errorf("kc number not found")
	}
	if owner != tenantID {
		return fmt.Errorf("kc number does not belong to this tenant")
	}
	return RemoveFromQueue(db, KCQueueName(kcNumberID), username)
}

// QueueMember is an operator currently in a KC number's queue.
type QueueMember struct {
	Username  string `json:"username"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Paused    bool   `json:"paused"`
}

// ListKCQueueMembers returns the operators currently in a KC number's queue.
func ListKCQueueMembers(db *sql.DB, tenantID, kcNumberID int) ([]QueueMember, error) {
	owner, err := kcNumberOwner(db, kcNumberID)
	if err != nil {
		return nil, fmt.Errorf("lookup kc number: %w", err)
	}
	if owner == 0 {
		return nil, fmt.Errorf("kc number not found")
	}
	if owner != tenantID {
		return nil, fmt.Errorf("kc number does not belong to this tenant")
	}

	rows, err := db.Query(`
		SELECT qm.membername, qm.paused, COALESCE(u.first_name,''), COALESCE(u.last_name,'')
		FROM ast_queue_members qm
		LEFT JOIN users u ON u.username = qm.membername
		WHERE qm.queue_name = $1
		ORDER BY qm.membername`, KCQueueName(kcNumberID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []QueueMember{}
	for rows.Next() {
		var m QueueMember
		var paused int
		if err := rows.Scan(&m.Username, &paused, &m.FirstName, &m.LastName); err != nil {
			continue
		}
		m.Paused = paused != 0
		result = append(result, m)
	}
	return result, nil
}

// AvailableUser is a tenant user that a KC number's queue could accept as a member.
type AvailableUser struct {
	Username  string `json:"username"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Active    bool   `json:"active"`
}

// ListKCAvailableUsers returns tenant users not already in a KC number's queue.
func ListKCAvailableUsers(db *sql.DB, tenantID, kcNumberID int) ([]AvailableUser, error) {
	rows, err := db.Query(`
		SELECT u.username, COALESCE(u.first_name,''), COALESCE(u.last_name,''), u.active
		FROM users u
		WHERE u.tenant_id = $1
		  AND u.username NOT IN (
		    SELECT membername FROM ast_queue_members WHERE queue_name = $2
		  )
		ORDER BY u.username`, tenantID, KCQueueName(kcNumberID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []AvailableUser{}
	for rows.Next() {
		var u AvailableUser
		if err := rows.Scan(&u.Username, &u.FirstName, &u.LastName, &u.Active); err != nil {
			continue
		}
		result = append(result, u)
	}
	return result, nil
}
