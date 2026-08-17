package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	mw "callcentrix/internal/middleware"
)

type TicketsHandler struct{ DB *sql.DB }

type Ticket struct {
	ID        int        `json:"id"`
	TenantID  *int       `json:"tenantId"`
	TopicID   *int       `json:"topicId"`
	Topic     *TopicInfo `json:"topic,omitempty"`
	Subject   string     `json:"subject"`
	Body      string     `json:"body"`
	CallerNo  string     `json:"callerNo"`
	CalleeNo  string     `json:"calleeNo"`
	UserID    *int       `json:"userId"`
	Status    string     `json:"status"`
	Priority  string     `json:"priority"`
	CreatedAt string     `json:"createdAt"`
	UpdatedAt string     `json:"updatedAt"`
}

type TicketComment struct {
	ID        int    `json:"id"`
	TicketID  int    `json:"ticketId"`
	UserID    *int   `json:"userId"`
	Username  string `json:"username"`
	Text      string `json:"text"`
	CreatedAt string `json:"createdAt"`
}

func (h *TicketsHandler) List(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	q := r.URL.Query()
	status      := q.Get("status")
	search      := q.Get("search")
	callerNo    := q.Get("caller_no")
	topicIDStr  := q.Get("topicId")

	query := `SELECT t.id, t.tenant_id, t.topic_id, t.subject, t.body, t.caller_no, t.callee_no,
	           t.user_id, t.status, t.priority, t.created_at, t.updated_at,
	           tc.names
	          FROM tickets t
	          LEFT JOIN topic_catalog tc ON tc.id = t.topic_id
	          WHERE 1=1`
	args := []any{}
	n := 1

	if c.UserType != 0 && c.TenantID != nil {
		query += ` AND t.tenant_id = $` + strconv.Itoa(n)
		args = append(args, *c.TenantID)
		n++
	}
	if c.UserType == 3 {
		query += ` AND t.user_id = $` + strconv.Itoa(n)
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
		var namesJSON []byte
		if err := rows.Scan(
			&t.ID, &t.TenantID, &t.TopicID, &t.Subject, &t.Body,
			&t.CallerNo, &t.CalleeNo, &t.UserID, &t.Status, &t.Priority,
			&t.CreatedAt, &t.UpdatedAt, &namesJSON,
		); err != nil {
			continue
		}
		if t.TopicID != nil && namesJSON != nil {
			names := map[string]string{}
			if err := json.Unmarshal(namesJSON, &names); err == nil {
				t.Topic = &TopicInfo{ID: *t.TopicID, Names: names}
			}
		}
		result = append(result, t)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tickets": result})
}

func (h *TicketsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var t Ticket
	var namesJSON []byte
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT t.id, t.tenant_id, t.topic_id, t.subject, t.body, t.caller_no, t.callee_no,
		        t.user_id, t.status, t.priority, t.created_at, t.updated_at, tc.names
		 FROM tickets t
		 LEFT JOIN topic_catalog tc ON tc.id = t.topic_id
		 WHERE t.id = $1`, id,
	).Scan(&t.ID, &t.TenantID, &t.TopicID, &t.Subject, &t.Body, &t.CallerNo, &t.CalleeNo,
		&t.UserID, &t.Status, &t.Priority, &t.CreatedAt, &t.UpdatedAt, &namesJSON)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if t.TopicID != nil && namesJSON != nil {
		names := map[string]string{}
		if err := json.Unmarshal(namesJSON, &names); err == nil {
			t.Topic = &TopicInfo{ID: *t.TopicID, Names: names}
		}
	}
	writeJSON(w, http.StatusOK, t)
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
		`INSERT INTO tickets (tenant_id, topic_id, subject, body, caller_no, callee_no, user_id, status, priority)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		c.TenantID, body.TopicID, body.Subject, body.Body, body.CallerNo, body.CalleeNo,
		c.Sub, body.Status, body.Priority,
	).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int{"id": id})
}

func (h *TicketsHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var body struct {
		Subject  string `json:"subject"`
		Body     string `json:"body"`
		CallerNo string `json:"callerNo"`
		Status   string `json:"status"`
		Priority string `json:"priority"`
		TopicID  *int   `json:"topicId"`
		UserID   *int   `json:"userId"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	_, err := h.DB.ExecContext(r.Context(),
		`UPDATE tickets SET subject=$1, body=$2, caller_no=$3, status=$4, priority=$5, topic_id=$6, updated_at=NOW() WHERE id=$7`,
		body.Subject, body.Body, body.CallerNo, body.Status, body.Priority, body.TopicID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *TicketsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	_, err := h.DB.ExecContext(r.Context(), `DELETE FROM tickets WHERE id=$1`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *TicketsHandler) ListComments(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
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

	var id int
	err := h.DB.QueryRowContext(r.Context(),
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
