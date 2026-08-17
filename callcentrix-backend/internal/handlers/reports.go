package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/lib/pq"
	mw "callcentrix/internal/middleware"
)

// ReportsHandler serves cross-cutting reports. DB is the main application
// database (tickets/users/topics); CDRDB is where ast_cdr lives — often the
// same *sql.DB as DB, but may be a separate physical database (see
// config.CDRDSN), so CDR lookups always go through their own query rather
// than a SQL join against DB.
type ReportsHandler struct {
	DB    *sql.DB
	CDRDB *sql.DB
}

// TicketReportRow is one line of the tickets report: the ticket itself plus
// who created/handled it, who it's assigned to, and (best-effort) the call
// recording it came from.
type TicketReportRow struct {
	ID         int        `json:"id"`
	Subject    string     `json:"subject"`
	TopicID    *int       `json:"topicId"`
	Topic      *TopicInfo `json:"topic,omitempty"`
	CallerNo   string     `json:"callerNo"`
	HandledBy  string     `json:"handledBy"`
	AssignedTo string     `json:"assignedTo"`
	Status     string     `json:"status"`
	CreatedAt  string     `json:"createdAt"`
	// CdrID, if set, is a best-effort match to the ast_cdr row this ticket's
	// call likely came from (see attachRecordings) — the frontend plays it
	// back via the existing /api/cdr/{id}/audio endpoint, same as the CDR page.
	CdrID *int `json:"cdrId"`
}

// Tickets returns tickets in a date range, optionally filtered by topic/
// status, for the "Отчёт по обращениям" report. Gated to Supervisor+ (see
// main.go) — same access level as CDR detail/monitor.
func (h *ReportsHandler) Tickets(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	q := r.URL.Query()
	dateFrom := q.Get("date_from")
	dateTo := q.Get("date_to")
	status := q.Get("status")
	topicIDStr := q.Get("topicId")

	query := `SELECT t.id, t.subject, t.topic_id, tc.names, t.caller_no,
	                 COALESCE(NULLIF(TRIM(CONCAT(cu.first_name,' ',cu.last_name)), ''), cu.username, ''),
	                 COALESCE(NULLIF(TRIM(CONCAT(au.first_name,' ',au.last_name)), ''), au.username, ''),
	                 t.status, t.created_at
	          FROM tickets t
	          LEFT JOIN topic_catalog tc ON tc.id = t.topic_id
	          LEFT JOIN users cu ON cu.id = t.user_id
	          LEFT JOIN users au ON au.id = t.assigned_user_id
	          WHERE 1=1`
	args := []any{}
	n := 1

	if c.UserType != 0 {
		if c.TenantID == nil {
			writeJSON(w, http.StatusOK, map[string]any{"tickets": []TicketReportRow{}})
			return
		}
		query += ` AND t.tenant_id = $` + strconv.Itoa(n)
		args = append(args, *c.TenantID)
		n++
	} else if tidStr := q.Get("tenantId"); tidStr != "" {
		if tid, err := strconv.Atoi(tidStr); err == nil && tid > 0 {
			query += ` AND t.tenant_id = $` + strconv.Itoa(n)
			args = append(args, tid)
			n++
		}
	}
	if dateFrom != "" {
		query += ` AND t.created_at >= $` + strconv.Itoa(n)
		args = append(args, dateFrom)
		n++
	}
	if dateTo != "" {
		query += ` AND t.created_at < ($` + strconv.Itoa(n) + `::date + interval '1 day')`
		args = append(args, dateTo)
		n++
	}
	if status != "" {
		query += ` AND t.status = $` + strconv.Itoa(n)
		args = append(args, status)
		n++
	}
	if topicIDStr != "" {
		if tid, err := strconv.Atoi(topicIDStr); err == nil {
			query += ` AND t.topic_id = $` + strconv.Itoa(n)
			args = append(args, tid)
			n++
		}
	}
	query += ` ORDER BY t.created_at DESC LIMIT 1000`

	rows, err := h.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := []TicketReportRow{}
	callerNumbers := map[string]bool{}
	for rows.Next() {
		var row TicketReportRow
		var namesJSON []byte
		if err := rows.Scan(&row.ID, &row.Subject, &row.TopicID, &namesJSON, &row.CallerNo,
			&row.HandledBy, &row.AssignedTo, &row.Status, &row.CreatedAt); err != nil {
			continue
		}
		if row.TopicID != nil && namesJSON != nil {
			names := map[string]string{}
			if err := json.Unmarshal(namesJSON, &names); err == nil {
				row.Topic = &TopicInfo{ID: *row.TopicID, Names: names}
			}
		}
		if np := normalizePhone(row.CallerNo); np != "" {
			callerNumbers[np] = true
		}
		result = append(result, row)
	}
	rows.Close()

	if len(callerNumbers) > 0 && h.CDRDB != nil {
		h.attachRecordings(r.Context(), result, callerNumbers)
	}

	writeJSON(w, http.StatusOK, map[string]any{"tickets": result})
}

type cdrCandidate struct {
	id       int
	callDate time.Time
}

// attachRecordings best-effort-matches each report row to the ast_cdr call
// it most likely came from, and sets its CdrID. Tickets have no direct call
// reference (they're filled in by hand — caller_no is free text), so this
// matches on normalized phone digits (see normalizePhone in blacklist.go,
// reused here for the same +992/00/leading-zero drift) against whichever of
// that call's src/dst carried the caller's number, picking whichever
// candidate call is closest in time to the ticket's created_at. Mutates rows
// in place (safe: slice elements, not a copy).
func (h *ReportsHandler) attachRecordings(ctx context.Context, rows []TicketReportRow, callerNumbers map[string]bool) {
	numbers := make([]string, 0, len(callerNumbers))
	for num := range callerNumbers {
		numbers = append(numbers, num)
	}

	cdrRows, err := h.CDRDB.QueryContext(ctx,
		`SELECT id, src, dst, calldate FROM ast_cdr
		 WHERE userfield <> '' AND (
		   regexp_replace(src,'\D','','g') = ANY($1) OR regexp_replace(dst,'\D','','g') = ANY($1)
		 )`, pq.Array(numbers))
	if err != nil {
		return
	}
	defer cdrRows.Close()

	byNumber := map[string][]cdrCandidate{}
	for cdrRows.Next() {
		var id int
		var src, dst string
		var callDate time.Time
		if err := cdrRows.Scan(&id, &src, &dst, &callDate); err != nil {
			continue
		}
		for _, num := range []string{normalizePhone(src), normalizePhone(dst)} {
			if num == "" || !callerNumbers[num] {
				continue
			}
			byNumber[num] = append(byNumber[num], cdrCandidate{id: id, callDate: callDate})
		}
	}

	for i := range rows {
		candidates := byNumber[normalizePhone(rows[i].CallerNo)]
		if len(candidates) == 0 {
			continue
		}
		createdAt, err := time.Parse(time.RFC3339Nano, rows[i].CreatedAt)
		if err != nil {
			continue
		}
		best := candidates[0]
		bestDiff := math.Abs(createdAt.Sub(best.callDate).Seconds())
		for _, cand := range candidates[1:] {
			diff := math.Abs(createdAt.Sub(cand.callDate).Seconds())
			if diff < bestDiff {
				best, bestDiff = cand, diff
			}
		}
		id := best.id
		rows[i].CdrID = &id
	}
}
