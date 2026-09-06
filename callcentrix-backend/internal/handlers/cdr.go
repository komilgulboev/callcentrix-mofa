package handlers

import (
	"database/sql"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/minio/minio-go/v7"
	"callcentrix/internal/jwt"
	mw "callcentrix/internal/middleware"
)

type CDRHandler struct {
	DB     *sql.DB
	Minio  *minio.Client // nil if MinIO isn't configured — Audio then 503s
	Bucket string
}

// EnsureLinkedIDColumn adds ast_cdr.linkedid if the table predates it — some
// classic cdr_pgsql schema examples still copy-pasted from old Asterisk setup
// guides omit it, but List/Get need it to JOIN call_outcomes (see
// ami.EnsureCallOutcomesTable). Must run against whichever database ast_cdr
// itself lives in (see cmd/server/main.go's cdrDB) — not the main
// migration.sql, which only ever runs against the main app database.
// Asterisk's cdr_pgsql module detects available columns at load time, so a
// running Asterisk may need `module reload cdr_pgsql` (or a restart) before
// it actually starts writing to the new column — existing rows and any CDRs
// logged before that reload will just show an empty linkedid, which
// call_outcomes's LEFT JOIN already tolerates (no agent-connected match).
func EnsureLinkedIDColumn(db *sql.DB) error {
	_, err := db.Exec(`ALTER TABLE ast_cdr ADD COLUMN IF NOT EXISTS linkedid VARCHAR(150) DEFAULT ''`)
	return err
}

type CDRRecord struct {
	ID          int    `json:"id"`
	CallDate    string `json:"callDate"`
	Clid        string `json:"clid"`
	Src         string `json:"src"`
	Dst         string `json:"dst"`
	Dcontext    string `json:"dcontext"`
	Channel     string `json:"channel"`
	DstChannel  string `json:"dstChannel"`
	Duration    int    `json:"duration"`
	Billsec     int    `json:"billsec"`
	Disposition string `json:"disposition"`
	AccountCode string `json:"accountCode"`
	UniqueID    string `json:"uniqueId"`
	UserField   string `json:"userField"`
	Recording   bool   `json:"recording"`
	LinkedID    string `json:"linkedId"`
	// AgentConnected is true iff AMI ever saw a human agent's channel join a
	// bridge for this call (see ami.Monitor's BridgeEnter handling /
	// recordAgentConnected) — unlike Disposition, this is accurate for
	// KC-routed inbound calls, where the caller's channel gets Answer()'d by
	// the dialplan itself (to play the greeting/IVR/hold music) before an
	// agent ever picks up, so Disposition alone reads "ANSWERED" even for
	// calls nobody actually took.
	AgentConnected bool `json:"agentConnected"`
}

// cdrScopeFilter returns a SQL "AND (...)" fragment (or "" for a SuperAdmin,
// who is unrestricted) that scopes a query against ast_cdr — optionally
// LEFT JOINed to call_outcomes, as every caller below does — to only the
// calls c is allowed to see, plus the bind args it references. argN is the
// next unused $N placeholder; nextArgN is what a caller should continue
// numbering from afterwards.
func cdrScopeFilter(c *jwt.Claims, argN int) (fragment string, args []any, nextArgN int) {
	switch {
	case c.UserType == 3:
		// Operator: only their own calls. call_outcomes.agent_username (set
		// from a real-time AMI BridgeEnter event — see
		// ami.recordAgentConnected) is the authoritative answer to "which
		// single agent actually took this call": a KC queue rings every
		// available member's channel at once (ringall), and ast_cdr's own
		// dst/channel/dstchannel — snapshotted from the dial attempt, not
		// the eventual bridge — can end up reflecting a member who was rung
		// but never answered, which let that operator see a colleague's
		// answered call. Trust call_outcomes whenever a row exists for this
		// call (call_outcomes.linkedid IS NOT NULL); fall back to the old
		// src/dst/channel heuristic only for calls AMI never tracked a
		// bridge for — direct extension-to-extension calls, outbound calls
		// (channel/dstchannel still carries the operator's own PJSIP
		// endpoint name regardless of any CALLERID override an outbound
		// tenant caller ID applies to src), or calls predating this tracking.
		fragment = ` AND (
			call_outcomes.agent_username = $` + strconv.Itoa(argN) + `
			OR (
				call_outcomes.linkedid IS NULL
				AND (src = $` + strconv.Itoa(argN+1) + ` OR dst = $` + strconv.Itoa(argN+1) +
			` OR channel LIKE $` + strconv.Itoa(argN+2) + ` OR dstchannel LIKE $` + strconv.Itoa(argN+2) + `)
			)
		)`
		args = []any{c.Username, c.Username, "PJSIP/" + c.Username + "-%"}
		nextArgN = argN + 3
	case c.UserType != 0 && c.TenantID != nil:
		// TenantAdmin / Supervisor: all calls of their tenant (see kc_numbers match below for missed-call coverage)
		fragment = ` AND (accountcode = $` + strconv.Itoa(argN) + ` OR src IN (
			SELECT sip_no FROM users WHERE tenant_id = $` + strconv.Itoa(argN) + ` AND sip_no != ''
		) OR dst IN (
			SELECT sip_no FROM users WHERE tenant_id = $` + strconv.Itoa(argN) + ` AND sip_no != ''
		) OR dst IN (
			SELECT number FROM kc_numbers WHERE tenant_id = $` + strconv.Itoa(argN) + `
		))`
		args = []any{strconv.Itoa(*c.TenantID)}
		nextArgN = argN + 1
	default:
		// SuperAdmin (UserType 0): unrestricted, sees everything.
		nextArgN = argN
	}
	return
}

func (h *CDRHandler) List(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	q := r.URL.Query()

	dateFrom   := q.Get("date_from")
	dateTo     := q.Get("date_to")
	disposition := q.Get("disposition")
	search     := q.Get("search")
	limitStr   := q.Get("limit")
	limit      := 500
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}

	query := `SELECT ast_cdr.id, calldate, clid, src, dst, dcontext, channel, dstchannel,
	                 duration, billsec, disposition, accountcode, uniqueid, userfield,
	                 ast_cdr.linkedid, COALESCE(call_outcomes.agent_connected, FALSE)
	          FROM ast_cdr
	          LEFT JOIN call_outcomes ON call_outcomes.linkedid = ast_cdr.linkedid
	          WHERE 1=1`
	args := []any{}
	n := 1

	scopeFrag, scopeArgs, nextN := cdrScopeFilter(c, n)
	query += scopeFrag
	args = append(args, scopeArgs...)
	n = nextN

	if dateFrom != "" {
		query += ` AND calldate >= $` + strconv.Itoa(n)
		args = append(args, dateFrom)
		n++
	}
	if dateTo != "" {
		query += ` AND calldate < ($` + strconv.Itoa(n) + `::date + interval '1 day')`
		args = append(args, dateTo)
		n++
	}
	if disposition != "" {
		query += ` AND disposition = $` + strconv.Itoa(n)
		args = append(args, disposition)
		n++
	}
	if search != "" {
		query += ` AND (src ILIKE $` + strconv.Itoa(n) + ` OR dst ILIKE $` + strconv.Itoa(n) + `)`
		args = append(args, "%"+search+"%")
		n++
	}
	query += ` ORDER BY calldate DESC LIMIT $` + strconv.Itoa(n)
	args = append(args, limit)

	rows, err := h.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	result := []CDRRecord{}
	for rows.Next() {
		var rec CDRRecord
		if err := rows.Scan(&rec.ID, &rec.CallDate, &rec.Clid, &rec.Src, &rec.Dst,
			&rec.Dcontext, &rec.Channel, &rec.DstChannel,
			&rec.Duration, &rec.Billsec, &rec.Disposition,
			&rec.AccountCode, &rec.UniqueID, &rec.UserField,
			&rec.LinkedID, &rec.AgentConnected); err != nil {
			continue
		}
		rec.Recording = rec.UserField != ""
		result = append(result, rec)
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": result, "total": len(result)})
}

func (h *CDRHandler) Get(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	query := `SELECT ast_cdr.id, calldate, clid, src, dst, dcontext, channel, dstchannel,
	                 duration, billsec, disposition, accountcode, uniqueid, userfield,
	                 ast_cdr.linkedid, COALESCE(call_outcomes.agent_connected, FALSE)
	          FROM ast_cdr
	          LEFT JOIN call_outcomes ON call_outcomes.linkedid = ast_cdr.linkedid
	          WHERE ast_cdr.id = $1`
	args := []any{id}
	// This handler is RequireRole(2)-gated (SuperAdmin/TenantAdmin/Supervisor
	// only — Operators never reach it), but without this it let a
	// Supervisor/TenantAdmin from ANY tenant pull ANY other tenant's call
	// metadata just by guessing the numeric id. Same scoping as List.
	scopeFrag, scopeArgs, _ := cdrScopeFilter(c, 2)
	query += scopeFrag
	args = append(args, scopeArgs...)

	var rec CDRRecord
	err := h.DB.QueryRowContext(r.Context(), query, args...).Scan(
		&rec.ID, &rec.CallDate, &rec.Clid, &rec.Src, &rec.Dst,
		&rec.Dcontext, &rec.Channel, &rec.DstChannel,
		&rec.Duration, &rec.Billsec, &rec.Disposition,
		&rec.AccountCode, &rec.UniqueID, &rec.UserField,
		&rec.LinkedID, &rec.AgentConnected)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rec.Recording = rec.UserField != ""
	writeJSON(w, http.StatusOK, rec)
}

// Audio streams a call recording from MinIO through this same
// JWT/role-gated endpoint, rather than redirecting the browser straight to
// MinIO. That keeps MinIO itself fully private (no anonymous bucket access,
// no address the browser ever needs to reach directly) and means a
// bookmarked/leaked URL is useless without a valid session, since every
// request re-checks auth here — including which specific call this id is
// allowed to see (cdrScopeFilter), not just that the caller holds a
// RequireRole(2)-eligible role in general: without that, any
// Supervisor/TenantAdmin, from any tenant, could listen to any other
// tenant's recording just by iterating the numeric id. http.ServeContent
// drives it so Range requests (seeking within the recording) work correctly
// — minio.Object implements io.ReadSeeker, translating each Seek into a
// ranged GetObject call under the hood.
func (h *CDRHandler) Audio(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	query := `SELECT ast_cdr.userfield
	          FROM ast_cdr
	          LEFT JOIN call_outcomes ON call_outcomes.linkedid = ast_cdr.linkedid
	          WHERE ast_cdr.id = $1`
	args := []any{id}
	scopeFrag, scopeArgs, _ := cdrScopeFilter(c, 2)
	query += scopeFrag
	args = append(args, scopeArgs...)

	var userField string
	err := h.DB.QueryRowContext(r.Context(), query, args...).Scan(&userField)
	if err != nil || userField == "" {
		writeError(w, http.StatusNotFound, "no recording")
		return
	}
	if h.Minio == nil {
		writeError(w, http.StatusServiceUnavailable, "recording storage not configured")
		return
	}

	obj, err := h.Minio.GetObject(r.Context(), h.Bucket, userField, minio.GetObjectOptions{})
	if err != nil {
		writeError(w, http.StatusNotFound, "recording not found")
		return
	}
	defer obj.Close()

	stat, err := obj.Stat()
	if err != nil {
		writeError(w, http.StatusNotFound, "recording not found")
		return
	}

	if ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(userField))); ct != "" {
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", "audio/wav")
	}
	http.ServeContent(w, r, userField, stat.LastModified, obj)
}
