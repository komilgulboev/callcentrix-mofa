package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"callcentrix/internal/email"
	"callcentrix/internal/jwt"
	mw "callcentrix/internal/middleware"
	"callcentrix/internal/telegram"
)

type TicketsHandler struct{ DB *sql.DB }

type Ticket struct {
	ID             int              `json:"id"`
	TenantID       *int             `json:"tenantId"`
	TopicID        *int             `json:"topicId"`
	Topic          *TopicInfo       `json:"topic,omitempty"`
	SiteID         *int             `json:"siteId"`
	Site           *SiteInfo        `json:"site,omitempty"`
	Subject        string           `json:"subject"`
	Body           string           `json:"body"`
	CallerNo       string           `json:"callerNo"`
	CallerName     string           `json:"callerName,omitempty"`
	CalleeNo       string           `json:"calleeNo"`
	UserID         *int             `json:"userId"`
	Assignees      []TicketAssignee `json:"assignees"`
	Status         string           `json:"status"`
	Priority       string           `json:"priority"`
	CreatedAt      string           `json:"createdAt"`
	UpdatedAt      string           `json:"updatedAt"`
	ResolvedBy     *int             `json:"resolvedBy"`
	ResolvedByName string           `json:"resolvedByName,omitempty"`
	ResolvedAt     *string          `json:"resolvedAt"`
}

// TicketAssignee is one row of ticket_assignees, with the user's display
// name joined in for the API response — same shape as TaskAssignee.
type TicketAssignee struct {
	UserID    int    `json:"userId"`
	Name      string `json:"name"`
	IsPrimary bool   `json:"isPrimary"`
}

// ticketAssigneesJSONExpr is the shared SELECT fragment that aggregates a
// ticket's assignees into a JSON array, used by both List (many tickets)
// and Get (one) — mirrors tasks.go's assigneesJSONExpr.
const ticketAssigneesJSONExpr = `COALESCE((
	SELECT json_agg(json_build_object(
		'userId', ta.user_id,
		'name', COALESCE(NULLIF(TRIM(CONCAT(au.first_name,' ',au.last_name)), ''), au.username),
		'isPrimary', ta.is_primary
	) ORDER BY ta.is_primary DESC, au.first_name)
	FROM ticket_assignees ta JOIN users au ON au.id = ta.user_id
	WHERE ta.ticket_id = t.id
), '[]')`

// callerNameJoin matches a ticket's caller_no against users.username/sip_no
// (a self-registered citizen's phone doubles as both, see RegistrationHandler)
// ignoring formatting (+/00/country-code) differences, same normalization
// rule as blacklist/whitelist matching (see normalizePhone in blacklist.go).
// Scoped to the ticket's own tenant so operators never see another tenant's
// caller names.
const callerNameJoin = `
	LEFT JOIN users cnu ON cnu.tenant_id = t.tenant_id AND t.caller_no <> ''
		AND (regexp_replace(cnu.username,'\D','','g') = regexp_replace(t.caller_no,'\D','','g')
		     OR regexp_replace(cnu.sip_no,'\D','','g') = regexp_replace(t.caller_no,'\D','','g'))`

const callerNameSelect = `COALESCE(NULLIF(TRIM(CONCAT(cnu.first_name,' ',cnu.last_name)), ''), '')`

type TicketComment struct {
	ID        int    `json:"id"`
	TicketID  int    `json:"ticketId"`
	UserID    *int   `json:"userId"`
	Username  string `json:"username"`
	Text      string `json:"text"`
	CreatedAt string `json:"createdAt"`
}

// TicketHistoryEvent is one entry on a ticket's combined history timeline —
// either a status change (Kind == "status": who opened it, when OldStatus
// is "", or who moved it between two statuses otherwise — see
// TicketsHandler.Create/Update) or a (re)assignment (Kind == "assignment":
// who assigned it and to whom — Assignees is a comma-joined snapshot of
// display names, "" meaning it was unassigned — see TicketsHandler.Assign).
// ListTicketHistory merges both source tables into this one shape, sorted
// together by time, since they're really one "what happened to this ticket"
// timeline from the reader's point of view.
type TicketHistoryEvent struct {
	ID        int    `json:"id"`
	Kind      string `json:"kind"`
	Username  string `json:"username"`
	OldStatus string `json:"oldStatus,omitempty"`
	NewStatus string `json:"newStatus,omitempty"`
	Assignees string `json:"assignees,omitempty"`
	CreatedAt string `json:"createdAt"`
}

func (h *TicketsHandler) List(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	q := r.URL.Query()
	status      := q.Get("status")
	search      := q.Get("search")
	callerNo    := q.Get("caller_no")
	topicIDStr  := q.Get("topicId")

	query := `SELECT t.id, t.tenant_id, t.topic_id, t.site_id, t.subject, t.body, t.caller_no, t.callee_no,
	           t.user_id, t.status, t.priority, t.created_at, t.updated_at,
	           tc.names, sc.names, ` + ticketAssigneesJSONExpr + `, ` + callerNameSelect + `,
	           t.resolved_by, COALESCE(NULLIF(TRIM(CONCAT(ru.first_name,' ',ru.last_name)), ''), ru.username), t.resolved_at
	          FROM tickets t
	          LEFT JOIN topic_catalog tc ON tc.id = t.topic_id
	          LEFT JOIN site_catalog sc ON sc.id = t.site_id
	          LEFT JOIN users ru ON ru.id = t.resolved_by` + callerNameJoin + `
	          WHERE 1=1`
	args := []any{}
	n := 1

	if c.UserType != 0 && c.TenantID != nil {
		query += ` AND t.tenant_id = $` + strconv.Itoa(n)
		args = append(args, *c.TenantID)
		n++
	}
	if c.UserType == 3 {
		// Operator: only tickets they created themselves or where they're one
		// of the assignees — not every ticket in the tenant (see
		// AssignableUsers/Assign).
		query += ` AND (t.user_id = $` + strconv.Itoa(n) + ` OR EXISTS (SELECT 1 FROM ticket_assignees ta WHERE ta.ticket_id = t.id AND ta.user_id = $` + strconv.Itoa(n) + `))`
		args = append(args, c.Sub)
		n++
	}
	if status != "" {
		query += ` AND t.status = $` + strconv.Itoa(n)
		args = append(args, status)
		n++
	}
	if callerNo != "" {
		query += ` AND t.caller_no = $` + strconv.Itoa(n)
		args = append(args, callerNo)
		n++
	}
	if topicIDStr != "" {
		if tid, err := strconv.Atoi(topicIDStr); err == nil {
			query += ` AND t.topic_id = $` + strconv.Itoa(n)
			args = append(args, tid)
			n++
		}
	}
	if search != "" {
		query += ` AND (t.subject ILIKE $` + strconv.Itoa(n) + ` OR t.caller_no ILIKE $` + strconv.Itoa(n) + `)`
		args = append(args, "%"+search+"%")
		n++
	}
	_ = n
	query += ` ORDER BY t.created_at DESC LIMIT 200`

	rows, err := h.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	result := []Ticket{}
	for rows.Next() {
		var t Ticket
		var topicNamesJSON, siteNamesJSON, assigneesJSON []byte
		var resolvedByName sql.NullString
		var resolvedAt sql.NullString
		if err := rows.Scan(
			&t.ID, &t.TenantID, &t.TopicID, &t.SiteID, &t.Subject, &t.Body,
			&t.CallerNo, &t.CalleeNo, &t.UserID, &t.Status, &t.Priority,
			&t.CreatedAt, &t.UpdatedAt, &topicNamesJSON, &siteNamesJSON, &assigneesJSON, &t.CallerName,
			&t.ResolvedBy, &resolvedByName, &resolvedAt,
		); err != nil {
			continue
		}
		t.ResolvedByName = resolvedByName.String
		if resolvedAt.Valid {
			t.ResolvedAt = &resolvedAt.String
		}
		if t.TopicID != nil && topicNamesJSON != nil {
			names := map[string]string{}
			if err := json.Unmarshal(topicNamesJSON, &names); err == nil {
				t.Topic = &TopicInfo{ID: *t.TopicID, Names: names}
			}
		}
		if t.SiteID != nil && siteNamesJSON != nil {
			names := map[string]string{}
			if err := json.Unmarshal(siteNamesJSON, &names); err == nil {
				t.Site = &SiteInfo{ID: *t.SiteID, Names: names}
			}
		}
		t.Assignees = []TicketAssignee{}
		_ = json.Unmarshal(assigneesJSON, &t.Assignees)
		result = append(result, t)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tickets": result})
}

// canAccessTicket decides whether the caller may view/modify a ticket.
// SuperAdmin: always. TenantAdmin/Supervisor: same tenant (tickets are a
// tenant-wide queue for those roles). Operator: same tenant AND they either
// created the ticket themselves or are one of its assignees — mirrors the
// List filter above, but every single-ticket endpoint (Get/Update/Delete/
// Assign/comments) needs its own check too, since a ticket id in the URL
// bypasses List's WHERE entirely. assigneeRef is shared with tasks.go — same
// shape, same many-to-many pattern (see ticket_assignees).
func canAccessTicket(c *jwt.Claims, tenantID *int, userID *int, assignees []assigneeRef) bool {
	if c.UserType == 0 {
		return true
	}
	if c.TenantID == nil || tenantID == nil || *c.TenantID != *tenantID {
		return false
	}
	if c.UserType != 3 {
		return true
	}
	if userID != nil && *userID == c.Sub {
		return true
	}
	return isTaskAssignee(c.Sub, assignees)
}

// ticketWriteLocked reports whether c must be blocked from any further write
// on a ticket in currentStatus — status, other fields, assignment, comments.
// Only ever locks an Operator: once a ticket is resolved or closed, they can
// still see it (canAccessTicket/Get/ListComments/ListTicketHistory are
// unaffected) but can't touch it again — only a Supervisor/TenantAdmin/
// SuperAdmin can still act on (e.g. reopen) it from there.
func ticketWriteLocked(c *jwt.Claims, currentStatus string) bool {
	if c.UserType <= 2 {
		return false
	}
	return currentStatus == "resolved" || currentStatus == "closed"
}

// ticketOwnership loads just enough of a ticket to run canAccessTicket
// (and ticketWriteLocked) against, for handlers that don't otherwise
// need the full row.
func (h *TicketsHandler) ticketOwnership(ctx context.Context, id int) (tenantID, userID *int, assignees []assigneeRef, status string, err error) {
	err = h.DB.QueryRowContext(ctx,
		`SELECT tenant_id, user_id, status FROM tickets WHERE id=$1`, id,
	).Scan(&tenantID, &userID, &status)
	if err != nil {
		return
	}
	assignees, err = h.loadTicketAssigneeRefs(ctx, id)
	return
}

// loadTicketAssigneeRefs returns the lightweight (no name) assignee shape
// used for authorization checks — mirrors TasksHandler.loadAssigneeRefs.
func (h *TicketsHandler) loadTicketAssigneeRefs(ctx context.Context, ticketID int) ([]assigneeRef, error) {
	rows, err := h.DB.QueryContext(ctx, `SELECT user_id, is_primary FROM ticket_assignees WHERE ticket_id=$1`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []assigneeRef{}
	for rows.Next() {
		var a assigneeRef
		if err := rows.Scan(&a.UserID, &a.IsPrimary); err != nil {
			continue
		}
		result = append(result, a)
	}
	return result, nil
}

// replaceTicketAssignees overwrites a ticket's full assignee set — the form
// always submits the complete list, not a delta. Mirrors
// TasksHandler.replaceAssignees.
func (h *TicketsHandler) replaceTicketAssignees(ctx context.Context, ticketID int, ids []int, primaryID *int) error {
	tx, err := h.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM ticket_assignees WHERE ticket_id=$1`, ticketID); err != nil {
		return err
	}
	for _, id := range ids {
		isPrimary := primaryID != nil && *primaryID == id
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO ticket_assignees (ticket_id, user_id, is_primary) VALUES ($1,$2,$3)`,
			ticketID, id, isPrimary); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (h *TicketsHandler) Get(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var t Ticket
	var topicNamesJSON, siteNamesJSON, assigneesJSON []byte
	var resolvedByName sql.NullString
	var resolvedAt sql.NullString
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT t.id, t.tenant_id, t.topic_id, t.site_id, t.subject, t.body, t.caller_no, t.callee_no,
		        t.user_id, t.status, t.priority, t.created_at, t.updated_at, tc.names, sc.names,
		        `+ticketAssigneesJSONExpr+`, `+callerNameSelect+`,
		        t.resolved_by, COALESCE(NULLIF(TRIM(CONCAT(ru.first_name,' ',ru.last_name)), ''), ru.username), t.resolved_at
		 FROM tickets t
		 LEFT JOIN topic_catalog tc ON tc.id = t.topic_id
		 LEFT JOIN site_catalog sc ON sc.id = t.site_id
		 LEFT JOIN users ru ON ru.id = t.resolved_by`+callerNameJoin+`
		 WHERE t.id = $1`, id,
	).Scan(&t.ID, &t.TenantID, &t.TopicID, &t.SiteID, &t.Subject, &t.Body, &t.CallerNo, &t.CalleeNo,
		&t.UserID, &t.Status, &t.Priority, &t.CreatedAt, &t.UpdatedAt, &topicNamesJSON, &siteNamesJSON,
		&assigneesJSON, &t.CallerName, &t.ResolvedBy, &resolvedByName, &resolvedAt)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	t.ResolvedByName = resolvedByName.String
	if resolvedAt.Valid {
		t.ResolvedAt = &resolvedAt.String
	}
	t.Assignees = []TicketAssignee{}
	_ = json.Unmarshal(assigneesJSON, &t.Assignees)
	refs := make([]assigneeRef, len(t.Assignees))
	for i, a := range t.Assignees {
		refs[i] = assigneeRef{UserID: a.UserID, IsPrimary: a.IsPrimary}
	}
	if !canAccessTicket(c, t.TenantID, t.UserID, refs) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if t.TopicID != nil && topicNamesJSON != nil {
		names := map[string]string{}
		if err := json.Unmarshal(topicNamesJSON, &names); err == nil {
			t.Topic = &TopicInfo{ID: *t.TopicID, Names: names}
		}
	}
	if t.SiteID != nil && siteNamesJSON != nil {
		names := map[string]string{}
		if err := json.Unmarshal(siteNamesJSON, &names); err == nil {
			t.Site = &SiteInfo{ID: *t.SiteID, Names: names}
		}
	}
	writeJSON(w, http.StatusOK, t)
}

// AssignableUsers returns every active TenantAdmin/Supervisor/Operator of
// the caller's own tenant — the pool a ticket can be assigned to. Operators
// are included because tickets are routinely assigned straight to them (see
// canAccessTicket/List, where an Operator sees a ticket if it's assigned to
// them); excluding user_type 3 here used to leave that pool down to whatever
// admins/supervisors existed, which could be a single person. Ticket
// assignment is a per-tenant concept, so SuperAdmin (attached to no tenant)
// gets an empty list here rather than every tenant's staff mixed together.
func (h *TicketsHandler) AssignableUsers(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	type assignableUser struct {
		ID        int    `json:"id"`
		Username  string `json:"username"`
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
	}
	result := []assignableUser{}
	if c.TenantID == nil {
		writeJSON(w, http.StatusOK, map[string]any{"users": result})
		return
	}

	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, username, first_name, last_name FROM users
		 WHERE tenant_id = $1 AND user_type <= 3 AND active = TRUE
		 ORDER BY user_type, first_name, last_name`, *c.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	for rows.Next() {
		var u assignableUser
		if err := rows.Scan(&u.ID, &u.Username, &u.FirstName, &u.LastName); err != nil {
			continue
		}
		result = append(result, u)
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": result})
}

// Assign replaces the full set of specialists a ticket is assigned to (an
// empty list clears it) — the form always submits the complete list, not a
// delta, same as Tasks. Each id must be a TenantAdmin/Supervisor/Operator of
// the same tenant as the caller — see AssignableUsers.
func (h *TicketsHandler) Assign(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	c := mw.GetClaims(r)

	var body struct {
		AssigneeIDs   []int `json:"assigneeIds"`
		PrimaryUserID *int  `json:"primaryUserId"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	tenantID, userID, assignees, currentStatus, err := h.ticketOwnership(r.Context(), id)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !canAccessTicket(c, tenantID, userID, assignees) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if ticketWriteLocked(c, currentStatus) {
		writeError(w, http.StatusForbidden, "ticket is resolved/closed — only a supervisor can modify it further")
		return
	}

	// Validates each id and, in the same query, grabs its display name for
	// the assignment-history row below — one lookup doing both jobs instead
	// of a COUNT(*) followed by a separate name fetch.
	assigneeNames := make([]string, 0, len(body.AssigneeIDs))
	for _, uid := range body.AssigneeIDs {
		var name string
		var err error
		if c.TenantID != nil {
			err = h.DB.QueryRowContext(r.Context(),
				`SELECT COALESCE(NULLIF(TRIM(CONCAT(first_name,' ',last_name)), ''), username) FROM users WHERE id=$1 AND tenant_id=$2 AND user_type <= 3`,
				uid, *c.TenantID).Scan(&name)
		} else {
			err = h.DB.QueryRowContext(r.Context(),
				`SELECT COALESCE(NULLIF(TRIM(CONCAT(first_name,' ',last_name)), ''), username) FROM users WHERE id=$1 AND user_type <= 3`,
				uid).Scan(&name)
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid assignee")
			return
		}
		assigneeNames = append(assigneeNames, name)
	}

	if err := h.replaceTicketAssignees(r.Context(), id, body.AssigneeIDs, body.PrimaryUserID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, _ = h.DB.ExecContext(r.Context(), `UPDATE tickets SET updated_at=NOW() WHERE id=$1`, id)

	// Record who (re)assigned the ticket and to whom — see ListTicketHistory,
	// which folds this in alongside status changes on the same timeline.
	_, _ = h.DB.ExecContext(r.Context(),
		`INSERT INTO ticket_assignment_history (ticket_id, actor_id, actor_name, assignees) VALUES ($1,$2,$3,$4)`,
		id, c.Sub, c.Username, strings.Join(assigneeNames, ", "))

	// Notify only the newly added assignees — someone already on the ticket
	// (still in the resubmitted list) doesn't need telling again.
	wasAssigned := make(map[int]bool, len(assignees))
	for _, a := range assignees {
		wasAssigned[a.UserID] = true
	}
	for _, uid := range body.AssigneeIDs {
		if !wasAssigned[uid] {
			go h.notifyTicketAssigned(id, uid)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// notifyTicketAssigned tells a newly assigned user about their ticket, over
// Telegram (if they've linked their account — see HandleTelegramMessage) and
// email (if their profile has one and SMTP is configured) — fire-and-forget,
// mirroring TasksHandler.notifyAssignee: best-effort, log-only on failure,
// never block the HTTP response. Uses a fresh DB call, not the request's own
// context, which is canceled once the response is written. The Telegram
// message carries status buttons (see ticketStatusKeyboard/
// handleTicketStatusCallback) so the assignee can move the ticket along
// without opening the web app — same pattern as Tasks.
func (h *TicketsHandler) notifyTicketAssigned(ticketID, userID int) {
	var subject, currentStatus string
	if err := h.DB.QueryRow(`SELECT subject, status FROM tickets WHERE id=$1`, ticketID).
		Scan(&subject, &currentStatus); err != nil {
		log.Printf("[Tickets] notifyTicketAssigned: load ticket %d: %v", ticketID, err)
		return
	}

	var chatID, userEmail string
	var userType int
	if err := h.DB.QueryRow(`SELECT telegram_chat_id, email, user_type FROM users WHERE id=$1`, userID).
		Scan(&chatID, &userEmail, &userType); err != nil {
		log.Printf("[Tickets] notifyTicketAssigned: load user %d: %v", userID, err)
		return
	}

	text := fmt.Sprintf("Вам назначен тикет: «%s»", subject)

	if chatID != "" {
		kb := ticketStatusKeyboard(ticketID, currentStatus, userType)
		if err := telegram.SendMessage(loadTelegramBotToken(h.DB), chatID, text, kb); err != nil {
			log.Printf("[Tickets] telegram notify user %d failed: %v", userID, err)
		}
	}
	if userEmail != "" {
		if err := email.Send(loadSMTPConfig(h.DB), userEmail, "Вам назначен тикет", text); err != nil {
			log.Printf("[Tickets] email notify user %d failed: %v", userID, err)
		}
	}
}

var validTicketStatuses = map[string]bool{
	"new": true, "open": true, "pending": true, "resolved": true, "closed": true,
}

var ticketStatusOrder = []string{"new", "open", "pending", "resolved", "closed"}

var ticketStatusLabels = map[string]string{
	"new":      "Новый",
	"open":     "Открыт",
	"pending":  "В ожидании",
	"resolved": "Решён",
	"closed":   "Закрыт",
}

func ticketStatusLabel(status string) string {
	if l, ok := ticketStatusLabels[status]; ok {
		return l
	}
	return status
}

// ticketStatusKeyboard builds one button per row for every status except the
// current one, mirroring tasks.go's statusKeyboard — with one addition:
// recipientUserType, since ticketWriteLocked (unlike Tasks' equivalent) is
// role-dependent — an Operator viewing a resolved/closed ticket gets no
// buttons at all (an explicitly *empty*, not nil, keyboard — Telegram only
// clears a message's existing buttons on edit if reply_markup is sent as
// `{inline_keyboard:[]}`), while a Supervisor+ always gets the full set.
func ticketStatusKeyboard(ticketID int, currentStatus string, recipientUserType int) *telegram.InlineKeyboard {
	if recipientUserType == 3 && (currentStatus == "resolved" || currentStatus == "closed") {
		return &telegram.InlineKeyboard{InlineKeyboard: [][]telegram.InlineButton{}}
	}
	rows := make([][]telegram.InlineButton, 0, len(ticketStatusOrder)-1)
	for _, s := range ticketStatusOrder {
		if s == currentStatus {
			continue
		}
		rows = append(rows, []telegram.InlineButton{{
			Text:         ticketStatusLabel(s),
			CallbackData: fmt.Sprintf("ticket_status:%d:%s", ticketID, s),
		}})
	}
	return &telegram.InlineKeyboard{InlineKeyboard: rows}
}

// handleTicketStatusCallback processes one ticket-status button press from
// Telegram (see notifyTicketAssigned/ticketStatusKeyboard). It reuses the
// exact same access rules as the web UI (canAccessTicket, ticketWriteLocked)
// by building a synthetic claims value from the pressing user's own linked
// Telegram account, so a button press can never do anything the web UI
// wouldn't already allow that person to do. Package-level (not a
// TicketsHandler method) because it's dispatched from
// TasksHandler.RunTelegramBot's single shared poll loop — see
// HandleTelegramMessage for why there's only one poller.
func handleTicketStatusCallback(db *sql.DB, token string, cb *telegram.CallbackQuery) {
	parts := strings.SplitN(cb.Data, ":", 3)
	if len(parts) != 3 || parts[0] != "ticket_status" {
		return
	}
	ticketID, err := strconv.Atoi(parts[1])
	newStatus := parts[2]
	if err != nil || !validTicketStatuses[newStatus] {
		_ = telegram.AnswerCallbackQuery(token, cb.ID, "Некорректный запрос")
		return
	}

	chatID := strconv.FormatInt(cb.From.ID, 10)
	var userID, userType int
	var tenantIDN sql.NullInt64
	if err := db.QueryRow(`SELECT id, user_type, tenant_id FROM users WHERE telegram_chat_id=$1`, chatID).
		Scan(&userID, &userType, &tenantIDN); err != nil {
		_ = telegram.AnswerCallbackQuery(token, cb.ID, "Ваш Telegram не привязан ни к одному пользователю")
		return
	}
	var callerTenantID *int
	if tenantIDN.Valid {
		v := int(tenantIDN.Int64)
		callerTenantID = &v
	}
	claims := &jwt.Claims{Sub: userID, UserType: userType, TenantID: callerTenantID}

	th := &TicketsHandler{DB: db}
	ticketTenantID, ticketUserID, assignees, currentStatus, err := th.ticketOwnership(context.Background(), ticketID)
	if err == sql.ErrNoRows {
		_ = telegram.AnswerCallbackQuery(token, cb.ID, "Тикет не найден")
		return
	}
	if err != nil {
		_ = telegram.AnswerCallbackQuery(token, cb.ID, "Ошибка")
		return
	}
	if !canAccessTicket(claims, ticketTenantID, ticketUserID, assignees) {
		_ = telegram.AnswerCallbackQuery(token, cb.ID, "Этот тикет вам недоступен")
		return
	}
	if ticketWriteLocked(claims, currentStatus) {
		_ = telegram.AnswerCallbackQuery(token, cb.ID, "Тикет решён — статус может менять только супервайзер")
		return
	}

	var username string
	_ = db.QueryRow(`SELECT username FROM users WHERE id=$1`, userID).Scan(&username)
	if err := th.applyTicketStatusChange(context.Background(), ticketID, userID, username, currentStatus, newStatus); err != nil {
		log.Printf("[TelegramBot] update ticket %d status: %v", ticketID, err)
		_ = telegram.AnswerCallbackQuery(token, cb.ID, "Ошибка при обновлении")
		return
	}

	_ = telegram.AnswerCallbackQuery(token, cb.ID, "Статус обновлён: "+ticketStatusLabel(newStatus))

	if cb.Message != nil {
		newText := cb.Message.Text + "\n\n✅ Текущий статус: " + ticketStatusLabel(newStatus)
		if err := telegram.EditMessageText(token, chatID, cb.Message.MessageID, newText, ticketStatusKeyboard(ticketID, newStatus, userType)); err != nil {
			log.Printf("[TelegramBot] edit message: %v", err)
		}
	}
}

func (h *TicketsHandler) Create(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	var body struct {
		Subject  string `json:"subject"`
		Body     string `json:"body"`
		CallerNo string `json:"callerNo"`
		CalleeNo string `json:"calleeNo"`
		Priority string `json:"priority"`
		Status   string `json:"status"`
		TopicID  *int   `json:"topicId"`
		SiteID   *int   `json:"siteId"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Priority == "" {
		body.Priority = "normal"
	}
	if body.Status == "" {
		body.Status = "new"
	}

	var id int
	err := h.DB.QueryRowContext(r.Context(),
		`INSERT INTO tickets (tenant_id, topic_id, site_id, subject, body, caller_no, callee_no, user_id, status, priority)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		c.TenantID, body.TopicID, body.SiteID, body.Subject, body.Body, body.CallerNo, body.CalleeNo,
		c.Sub, body.Status, body.Priority,
	).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// old_status='' marks this as the "opened" event, not a status change —
	// see ListTicketHistory.
	_, _ = h.DB.ExecContext(r.Context(),
		`INSERT INTO ticket_status_history (ticket_id, user_id, username, old_status, new_status) VALUES ($1,$2,$3,'',$4)`,
		id, c.Sub, c.Username, body.Status)
	writeJSON(w, http.StatusCreated, map[string]int{"id": id})
}

// applyTicketStatusChange writes a ticket's new status, keeps
// resolved_by/resolved_at in sync (set on a move to "resolved", left alone
// on a further move to "closed" since that's the normal next step rather
// than a reopening, cleared on a move to anything else), and — only when
// the status actually changed — records the transition in
// ticket_status_history. The one place both write paths that can change a
// ticket's status go through (Update's HTTP endpoint and the Telegram
// status buttons — see handleTicketStatusCallback), so resolved-by tracking
// can't drift between the two. Callers must already have done their own
// canAccessTicket/ticketWriteLocked check — this does none.
func (h *TicketsHandler) applyTicketStatusChange(ctx context.Context, ticketID, actorID int, actorUsername, oldStatus, newStatus string) error {
	if _, err := h.DB.ExecContext(ctx,
		`UPDATE tickets SET status=$1, updated_at=NOW() WHERE id=$2`, newStatus, ticketID); err != nil {
		return err
	}

	switch {
	case newStatus == "resolved":
		if _, err := h.DB.ExecContext(ctx,
			`UPDATE tickets SET resolved_by=$1, resolved_at=NOW() WHERE id=$2`, actorID, ticketID); err != nil {
			return err
		}
	case newStatus != "closed":
		if _, err := h.DB.ExecContext(ctx,
			`UPDATE tickets SET resolved_by=NULL, resolved_at=NULL WHERE id=$1`, ticketID); err != nil {
			return err
		}
	}

	if newStatus != oldStatus {
		_, _ = h.DB.ExecContext(ctx,
			`INSERT INTO ticket_status_history (ticket_id, user_id, username, old_status, new_status) VALUES ($1,$2,$3,$4,$5)`,
			ticketID, actorID, actorUsername, oldStatus, newStatus)
	}
	return nil
}

// Update applies a partial update — only fields actually present in the
// request body are written. Callers like TicketDetail's status dropdown send
// just {"status": "resolved"}; a blanket "SET subject=..., topic_id=..."
// would null out every other field (subject/body/caller_no/priority/topic_id)
// on every such call, since Go decodes the missing JSON keys as zero values.
func (h *TicketsHandler) Update(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	tenantID, userID, assignees, currentStatus, err := h.ticketOwnership(r.Context(), id)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !canAccessTicket(c, tenantID, userID, assignees) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if ticketWriteLocked(c, currentStatus) {
		writeError(w, http.StatusForbidden, "ticket is resolved/closed — only a supervisor can modify it further")
		return
	}

	var body struct {
		Subject  *string `json:"subject"`
		Body     *string `json:"body"`
		CallerNo *string `json:"callerNo"`
		Status   *string `json:"status"`
		Priority *string `json:"priority"`
		TopicID  *int    `json:"topicId"`
		SiteID   *int    `json:"siteId"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	sets := []string{}
	args := []any{}
	n := 1
	add := func(col string, val any) {
		sets = append(sets, fmt.Sprintf("%s=$%d", col, n))
		args = append(args, val)
		n++
	}
	if body.Subject != nil {
		add("subject", *body.Subject)
	}
	if body.Body != nil {
		add("body", *body.Body)
	}
	if body.CallerNo != nil {
		add("caller_no", *body.CallerNo)
	}
	if body.Priority != nil {
		add("priority", *body.Priority)
	}
	if body.TopicID != nil {
		add("topic_id", *body.TopicID)
	}
	if body.SiteID != nil {
		add("site_id", *body.SiteID)
	}
	// Status is applied separately (see applyTicketStatusChange) since it
	// also has to update resolved_by/resolved_at and write a history row —
	// folding it into this generic column-by-column SET would spread that
	// logic across two places instead of the one both write paths (this
	// endpoint and the Telegram status buttons) share.
	if len(sets) > 0 {
		sets = append(sets, "updated_at=NOW()")
		args = append(args, id)
		query := "UPDATE tickets SET " + strings.Join(sets, ", ") + fmt.Sprintf(" WHERE id=$%d", n)
		if _, err := h.DB.ExecContext(r.Context(), query, args...); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if body.Status != nil {
		if err := h.applyTicketStatusChange(r.Context(), id, c.Sub, c.Username, currentStatus, *body.Status); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *TicketsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	tenantID, userID, assignees, _, err := h.ticketOwnership(r.Context(), id)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !canAccessTicket(c, tenantID, userID, assignees) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	_, err = h.DB.ExecContext(r.Context(), `DELETE FROM tickets WHERE id=$1`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *TicketsHandler) ListComments(w http.ResponseWriter, r *http.Request) {
	claims := mw.GetClaims(r)
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	tenantID, userID, assignees, _, err := h.ticketOwnership(r.Context(), id)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !canAccessTicket(claims, tenantID, userID, assignees) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, ticket_id, user_id, username, text, created_at FROM ticket_comments
		 WHERE ticket_id = $1 ORDER BY created_at ASC`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	result := []TicketComment{}
	for rows.Next() {
		var c TicketComment
		if err := rows.Scan(&c.ID, &c.TicketID, &c.UserID, &c.Username, &c.Text, &c.CreatedAt); err != nil {
			continue
		}
		result = append(result, c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"comments": result})
}

// ListTicketHistory returns a ticket's combined timeline — who opened it and
// every status change (ticket_status_history) interleaved with every
// (re)assignment (ticket_assignment_history), oldest first. Same access gate
// as comments.
func (h *TicketsHandler) ListTicketHistory(w http.ResponseWriter, r *http.Request) {
	claims := mw.GetClaims(r)
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	tenantID, userID, assignees, _, err := h.ticketOwnership(r.Context(), id)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !canAccessTicket(claims, tenantID, userID, assignees) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	result := []TicketHistoryEvent{}

	statusRows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, username, old_status, new_status, created_at FROM ticket_status_history
		 WHERE ticket_id = $1`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for statusRows.Next() {
		var e TicketHistoryEvent
		if err := statusRows.Scan(&e.ID, &e.Username, &e.OldStatus, &e.NewStatus, &e.CreatedAt); err != nil {
			continue
		}
		e.Kind = "status"
		result = append(result, e)
	}
	statusRows.Close()

	assignRows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, actor_name, assignees, created_at FROM ticket_assignment_history
		 WHERE ticket_id = $1`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for assignRows.Next() {
		var e TicketHistoryEvent
		if err := assignRows.Scan(&e.ID, &e.Username, &e.Assignees, &e.CreatedAt); err != nil {
			continue
		}
		e.Kind = "assignment"
		result = append(result, e)
	}
	assignRows.Close()

	// Both queries come back independently ordered by their own table's id,
	// not interleaved by time — sort the merged set once here. CreatedAt
	// compares correctly as a plain string because lib/pq's TIMESTAMPTZ→
	// string scan always renders RFC3339Nano in UTC (same convention every
	// other created_at field in this codebase already relies on).
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt < result[j].CreatedAt })

	writeJSON(w, http.StatusOK, map[string]any{"history": result})
}

func (h *TicketsHandler) AddComment(w http.ResponseWriter, r *http.Request) {
	ticketID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	c := mw.GetClaims(r)
	var body struct {
		Text string `json:"text"`
	}
	if err := decode(r, &body); err != nil || body.Text == "" {
		writeError(w, http.StatusBadRequest, "text required")
		return
	}

	tenantID, userID, assignees, currentStatus, err := h.ticketOwnership(r.Context(), ticketID)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !canAccessTicket(c, tenantID, userID, assignees) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if ticketWriteLocked(c, currentStatus) {
		writeError(w, http.StatusForbidden, "ticket is resolved/closed — only a supervisor can modify it further")
		return
	}

	var id int
	err = h.DB.QueryRowContext(r.Context(),
		`INSERT INTO ticket_comments (ticket_id, user_id, username, text) VALUES ($1,$2,$3,$4) RETURNING id`,
		ticketID, c.Sub, c.Username, body.Text,
	).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	_, _ = h.DB.ExecContext(r.Context(),
		`UPDATE tickets SET updated_at=NOW() WHERE id=$1`, ticketID)

	writeJSON(w, http.StatusCreated, map[string]int{"id": id})
}
