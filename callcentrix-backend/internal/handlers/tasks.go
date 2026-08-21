package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"callcentrix/internal/jwt"
	mw "callcentrix/internal/middleware"
	"callcentrix/internal/telegram"
	"github.com/go-chi/chi/v5"
)

type TasksHandler struct{ DB *sql.DB }

type Task struct {
	ID          int            `json:"id"`
	TenantID    *int           `json:"tenantId"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Status      string         `json:"status"`
	CreatedBy   *int           `json:"createdBy"`
	CreatorName string         `json:"creatorName,omitempty"`
	Assignees   []TaskAssignee `json:"assignees"`
	CreatedAt   string         `json:"createdAt"`
	UpdatedAt   string         `json:"updatedAt"`
}

// TaskAssignee is one row of task_assignees, with the user's display name
// joined in for the API response.
type TaskAssignee struct {
	UserID    int    `json:"userId"`
	Name      string `json:"name"`
	IsPrimary bool   `json:"isPrimary"`
}

// assigneeRef is the lightweight (no name) shape used internally for
// authorization checks, so those paths don't need a join to `users`.
type assigneeRef struct {
	UserID    int
	IsPrimary bool
}

var validTaskStatuses = map[string]bool{
	"todo": true, "in_progress": true, "waiting": true, "resolved": true,
}

// canAccessTask decides whether the caller may read/status-change a task.
// SuperAdmin: always. TenantAdmin: same tenant. Supervisor/Operator: same
// tenant AND they're one of the assignees — Tasks is a personal work queue
// for those roles, unlike Tickets, where Supervisor/TenantAdmin see the
// whole tenant.
func canAccessTask(c *jwt.Claims, tenantID *int, assignees []assigneeRef) bool {
	if c.UserType == 0 {
		return true
	}
	if c.TenantID == nil || tenantID == nil || *c.TenantID != *tenantID {
		return false
	}
	if c.UserType == 1 {
		return true
	}
	for _, a := range assignees {
		if a.UserID == c.Sub {
			return true
		}
	}
	return false
}

// primaryAssignee returns the designated primary's user id, if any.
func primaryAssignee(assignees []assigneeRef) *int {
	for _, a := range assignees {
		if a.IsPrimary {
			id := a.UserID
			return &id
		}
	}
	return nil
}

// canChangeStatus applies the "only the primary assignee may change status"
// rule — opt-in: with no primary designated (single assignee, or several
// with none marked as primary), any assignee already cleared by
// canAccessTask may act. Admins always may.
func canChangeStatus(c *jwt.Claims, assignees []assigneeRef) bool {
	if c.UserType <= 1 {
		return true
	}
	if p := primaryAssignee(assignees); p != nil {
		return *p == c.Sub
	}
	return true
}

// assigneesJSONExpr is the shared SELECT fragment that aggregates a task's
// assignees into a JSON array, used by both List (many tasks) and Get (one).
const assigneesJSONExpr = `COALESCE((
	SELECT json_agg(json_build_object(
		'userId', ta.user_id,
		'name', COALESCE(NULLIF(TRIM(CONCAT(au.first_name,' ',au.last_name)), ''), au.username),
		'isPrimary', ta.is_primary
	) ORDER BY ta.is_primary DESC, au.first_name)
	FROM task_assignees ta JOIN users au ON au.id = ta.user_id
	WHERE ta.task_id = t.id
), '[]')`

func (h *TasksHandler) List(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	q := r.URL.Query()
	status := q.Get("status")

	query := `SELECT t.id, t.tenant_id, t.title, t.description, t.status,
	           t.created_by, COALESCE(NULLIF(TRIM(CONCAT(cu.first_name,' ',cu.last_name)), ''), cu.username),
	           ` + assigneesJSONExpr + `,
	           t.created_at, t.updated_at
	          FROM tasks t
	          LEFT JOIN users cu ON cu.id = t.created_by
	          WHERE 1=1`
	args := []any{}
	n := 1

	if c.UserType != 0 && c.TenantID != nil {
		query += ` AND t.tenant_id = $` + strconv.Itoa(n)
		args = append(args, *c.TenantID)
		n++
	} else if c.UserType == 0 {
		if tidStr := q.Get("tenantId"); tidStr != "" {
			if tid, err := strconv.Atoi(tidStr); err == nil {
				query += ` AND t.tenant_id = $` + strconv.Itoa(n)
				args = append(args, tid)
				n++
			}
		}
	}
	if c.UserType == 2 || c.UserType == 3 {
		query += ` AND EXISTS (SELECT 1 FROM task_assignees ta WHERE ta.task_id = t.id AND ta.user_id = $` + strconv.Itoa(n) + `)`
		args = append(args, c.Sub)
		n++
	}
	if status != "" {
		query += ` AND t.status = $` + strconv.Itoa(n)
		args = append(args, status)
		n++
	}
	query += ` ORDER BY t.created_at DESC LIMIT 500`

	rows, err := h.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	result := []Task{}
	for rows.Next() {
		var t Task
		var creatorName sql.NullString
		var assigneesJSON []byte
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Title, &t.Description, &t.Status,
			&t.CreatedBy, &creatorName, &assigneesJSON, &t.CreatedAt, &t.UpdatedAt); err != nil {
			continue
		}
		t.CreatorName = creatorName.String
		t.Assignees = []TaskAssignee{}
		_ = json.Unmarshal(assigneesJSON, &t.Assignees)
		result = append(result, t)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": result})
}

func (h *TasksHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	c := mw.GetClaims(r)

	var t Task
	var creatorName sql.NullString
	var assigneesJSON []byte
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT t.id, t.tenant_id, t.title, t.description, t.status,
		        t.created_by, COALESCE(NULLIF(TRIM(CONCAT(cu.first_name,' ',cu.last_name)), ''), cu.username),
		        `+assigneesJSONExpr+`,
		        t.created_at, t.updated_at
		 FROM tasks t
		 LEFT JOIN users cu ON cu.id = t.created_by
		 WHERE t.id = $1`, id,
	).Scan(&t.ID, &t.TenantID, &t.Title, &t.Description, &t.Status,
		&t.CreatedBy, &creatorName, &assigneesJSON, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	t.CreatorName = creatorName.String
	t.Assignees = []TaskAssignee{}
	_ = json.Unmarshal(assigneesJSON, &t.Assignees)

	refs := make([]assigneeRef, len(t.Assignees))
	for i, a := range t.Assignees {
		refs[i] = assigneeRef{UserID: a.UserID, IsPrimary: a.IsPrimary}
	}
	if !canAccessTask(c, t.TenantID, refs) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// AssignableUsers returns the Supervisors/Operators of a tenant — the pool a
// task can be assigned to. TenantAdmin is always locked to their own tenant;
// SuperAdmin (attached to no tenant) must pass ?tenantId=.
func (h *TasksHandler) AssignableUsers(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	type assignableUser struct {
		ID        int    `json:"id"`
		Username  string `json:"username"`
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
		UserType  int    `json:"userType"`
	}
	result := []assignableUser{}

	var tenantID int
	if c.TenantID != nil {
		tenantID = *c.TenantID
	} else if tid, err := strconv.Atoi(r.URL.Query().Get("tenantId")); err == nil {
		tenantID = tid
	} else {
		writeJSON(w, http.StatusOK, map[string]any{"users": result})
		return
	}

	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, username, first_name, last_name, user_type FROM users
		 WHERE tenant_id = $1 AND user_type IN (2,3) AND active = TRUE
		 ORDER BY user_type, first_name, last_name`, tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	for rows.Next() {
		var u assignableUser
		if err := rows.Scan(&u.ID, &u.Username, &u.FirstName, &u.LastName, &u.UserType); err != nil {
			continue
		}
		result = append(result, u)
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": result})
}

// validateAssignees confirms every id is an active Supervisor/Operator of
// tenantID — the same pool AssignableUsers offers.
func validateAssignees(db *sql.DB, ids []int, tenantID int) bool {
	if len(ids) == 0 {
		return false
	}
	for _, id := range ids {
		var cnt int
		db.QueryRow(
			`SELECT COUNT(*) FROM users WHERE id=$1 AND tenant_id=$2 AND user_type IN (2,3) AND active=TRUE`,
			id, tenantID).Scan(&cnt)
		if cnt == 0 {
			return false
		}
	}
	return true
}

// replaceAssignees overwrites a task's full assignee set — the form always
// submits the complete list, not a delta.
func (h *TasksHandler) replaceAssignees(taskID int, ids []int, primaryID *int) error {
	tx, err := h.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM task_assignees WHERE task_id=$1`, taskID); err != nil {
		return err
	}
	for _, id := range ids {
		isPrimary := primaryID != nil && *primaryID == id
		if _, err := tx.Exec(
			`INSERT INTO task_assignees (task_id, user_id, is_primary) VALUES ($1,$2,$3)`,
			taskID, id, isPrimary); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (h *TasksHandler) loadAssigneeRefs(taskID int) ([]assigneeRef, error) {
	rows, err := h.DB.Query(`SELECT user_id, is_primary FROM task_assignees WHERE task_id=$1`, taskID)
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

func (h *TasksHandler) Create(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	var body struct {
		Title         string `json:"title"`
		Description   string `json:"description"`
		AssigneeIDs   []int  `json:"assigneeIds"`
		PrimaryUserID *int   `json:"primaryUserId"`
		TenantID      *int   `json:"tenantId"`
	}
	if err := decode(r, &body); err != nil || strings.TrimSpace(body.Title) == "" || len(body.AssigneeIDs) == 0 {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	tenantID := c.TenantID
	if tenantID == nil {
		if body.TenantID == nil {
			writeError(w, http.StatusBadRequest, "tenantId required")
			return
		}
		tenantID = body.TenantID
	}

	if !validateAssignees(h.DB, body.AssigneeIDs, *tenantID) {
		writeError(w, http.StatusBadRequest, "invalid assignee")
		return
	}

	var id int
	err := h.DB.QueryRowContext(r.Context(),
		`INSERT INTO tasks (tenant_id, title, description, status, created_by)
		 VALUES ($1,$2,$3,'todo',$4) RETURNING id`,
		tenantID, body.Title, body.Description, c.Sub,
	).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := h.replaceAssignees(id, body.AssigneeIDs, body.PrimaryUserID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, assigneeID := range body.AssigneeIDs {
		go h.notifyAssignee(id, assigneeID)
	}
	writeJSON(w, http.StatusCreated, map[string]int{"id": id})
}

// Update is a partial update for title/description/assignees — status
// changes only ever go through UpdateStatus, so notification-firing logic
// for status transitions stays in one place.
func (h *TasksHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	c := mw.GetClaims(r)

	var tenantID *int
	err := h.DB.QueryRowContext(r.Context(), `SELECT tenant_id FROM tasks WHERE id=$1`, id).Scan(&tenantID)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	oldAssignees, err := h.loadAssigneeRefs(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !canAccessTask(c, tenantID, oldAssignees) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	var body struct {
		Title         *string `json:"title"`
		Description   *string `json:"description"`
		AssigneeIDs   []int   `json:"assigneeIds"`
		PrimaryUserID *int    `json:"primaryUserId"`
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
	if body.Title != nil {
		add("title", *body.Title)
	}
	if body.Description != nil {
		add("description", *body.Description)
	}
	if len(sets) > 0 {
		sets = append(sets, "updated_at=NOW()")
		args = append(args, id)
		query := "UPDATE tasks SET " + strings.Join(sets, ", ") + fmt.Sprintf(" WHERE id=$%d", n)
		if _, err := h.DB.ExecContext(r.Context(), query, args...); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if body.AssigneeIDs != nil {
		if tenantID == nil {
			writeError(w, http.StatusBadRequest, "task has no tenant")
			return
		}
		if !validateAssignees(h.DB, body.AssigneeIDs, *tenantID) {
			writeError(w, http.StatusBadRequest, "invalid assignee")
			return
		}
		if err := h.replaceAssignees(id, body.AssigneeIDs, body.PrimaryUserID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		oldIDs := map[int]bool{}
		for _, a := range oldAssignees {
			oldIDs[a.UserID] = true
		}
		for _, newID := range body.AssigneeIDs {
			if !oldIDs[newID] {
				go h.notifyAssignee(id, newID)
			}
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// UpdateStatus is the drag-and-drop endpoint — open to every role that can
// see the task (see canAccessTask), gated further by canChangeStatus (the
// opt-in "only the primary assignee" rule).
func (h *TasksHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	c := mw.GetClaims(r)

	var body struct {
		Status string `json:"status"`
	}
	if err := decode(r, &body); err != nil || !validTaskStatuses[body.Status] {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}

	var tenantID, createdBy *int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT tenant_id, created_by FROM tasks WHERE id=$1`, id,
	).Scan(&tenantID, &createdBy)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	assignees, err := h.loadAssigneeRefs(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !canAccessTask(c, tenantID, assignees) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if !canChangeStatus(c, assignees) {
		writeError(w, http.StatusForbidden, "only the primary assignee can change status")
		return
	}

	if err := h.updateTaskStatus(id, c.Sub, createdBy, assignees, body.Status); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// updateTaskStatus is the shared core of UpdateStatus (HTTP, drag-and-drop)
// and handleTelegramCallback (Telegram status buttons): writes the new
// status and notifies the creator plus every other assignee — the task is
// shared work, not just the actor's. Callers must do their own authorization
// check first (canAccessTask + canChangeStatus) — this does none.
func (h *TasksHandler) updateTaskStatus(taskID, actorUserID int, createdBy *int, assignees []assigneeRef, newStatus string) error {
	if _, err := h.DB.Exec(
		`UPDATE tasks SET status=$1, updated_at=NOW() WHERE id=$2`, newStatus, taskID,
	); err != nil {
		return err
	}

	if createdBy != nil && *createdBy != actorUserID {
		go h.notifyStatusChange(taskID, *createdBy, newStatus, true)
	}
	for _, a := range assignees {
		if a.UserID != actorUserID {
			go h.notifyStatusChange(taskID, a.UserID, newStatus, false)
		}
	}
	return nil
}

func (h *TasksHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if _, err := h.DB.ExecContext(r.Context(), `DELETE FROM tasks WHERE id=$1`, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type taskNotification struct {
	ID        int    `json:"id"`
	TaskID    int    `json:"taskId"`
	TaskTitle string `json:"taskTitle"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	IsRead    bool   `json:"isRead"`
	CreatedAt string `json:"createdAt"`
}

func (h *TasksHandler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT n.id, n.task_id, t.title, n.status, n.message, n.is_read, n.created_at
		 FROM task_notifications n
		 JOIN tasks t ON t.id = n.task_id
		 WHERE n.user_id = $1
		 ORDER BY n.created_at DESC LIMIT 50`, c.Sub)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	result := []taskNotification{}
	for rows.Next() {
		var n taskNotification
		if err := rows.Scan(&n.ID, &n.TaskID, &n.TaskTitle, &n.Status, &n.Message, &n.IsRead, &n.CreatedAt); err != nil {
			continue
		}
		result = append(result, n)
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": result})
}

func (h *TasksHandler) MarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	c := mw.GetClaims(r)
	_, err := h.DB.ExecContext(r.Context(),
		`UPDATE task_notifications SET is_read=TRUE WHERE id=$1 AND user_id=$2`, id, c.Sub)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *TasksHandler) MarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	_, err := h.DB.ExecContext(r.Context(),
		`UPDATE task_notifications SET is_read=TRUE WHERE user_id=$1 AND is_read=FALSE`, c.Sub)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var taskStatusLabels = map[string]string{
	"todo":        "Задачи",
	"in_progress": "В работе",
	"waiting":     "Ожидание",
	"resolved":    "Решено",
}

func taskStatusLabel(status string) string {
	if l, ok := taskStatusLabels[status]; ok {
		return l
	}
	return status
}

var taskStatusOrder = []string{"todo", "in_progress", "waiting", "resolved"}

// statusKeyboard builds one button per row for every status except the
// current one — lets the Telegram message offer any transition, matching
// the Kanban board's own any-column-to-any-column drag. Once a task is
// resolved there's nothing left to do from Telegram, so it returns an
// explicitly *empty* (not nil) keyboard: Telegram only clears a message's
// existing buttons on edit if reply_markup is sent as `{inline_keyboard:[]}`
// — omitting the field entirely leaves old buttons in place.
func statusKeyboard(taskID int, currentStatus string) *telegram.InlineKeyboard {
	if currentStatus == "resolved" {
		return &telegram.InlineKeyboard{InlineKeyboard: [][]telegram.InlineButton{}}
	}
	rows := make([][]telegram.InlineButton, 0, len(taskStatusOrder)-1)
	for _, s := range taskStatusOrder {
		if s == currentStatus {
			continue
		}
		rows = append(rows, []telegram.InlineButton{{
			Text:         taskStatusLabel(s),
			CallbackData: fmt.Sprintf("task_status:%d:%s", taskID, s),
		}})
	}
	return &telegram.InlineKeyboard{InlineKeyboard: rows}
}

// truncate keeps Telegram messages under its ~4096-character limit — task
// descriptions are unbounded free text.
func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// notifyAssignee and notifyStatusChange are fire-and-forget, mirroring
// RegistrationHandler.sendCode (register.go): best-effort, log-only on
// failure, never block the HTTP response. They use fresh DB calls (not the
// request's context, which is canceled once the response is written).
func (h *TasksHandler) notifyAssignee(taskID, assigneeID int) {
	var title, description string
	if err := h.DB.QueryRow(`SELECT title, description FROM tasks WHERE id=$1`, taskID).
		Scan(&title, &description); err != nil {
		log.Printf("[Tasks] notifyAssignee: load task %d: %v", taskID, err)
		return
	}
	var chatID string
	if err := h.DB.QueryRow(`SELECT telegram_chat_id FROM users WHERE id=$1`, assigneeID).Scan(&chatID); err != nil {
		log.Printf("[Tasks] notifyAssignee: load user %d: %v", assigneeID, err)
		return
	}
	if chatID == "" {
		return
	}
	text := fmt.Sprintf("Вам назначена новая задача: %s", title)
	if description != "" {
		text += "\n\n" + truncate(description, 1000)
	}
	// Status buttons let the assignee update progress right from Telegram,
	// without opening the web app — see handleTelegramCallback.
	if err := telegram.SendMessage(h.loadBotToken(), chatID, text, statusKeyboard(taskID, "todo")); err != nil {
		log.Printf("[Tasks] telegram notify assignee %d failed: %v", assigneeID, err)
	}
}

// notifyStatusChange tells one recipient (the creator, or another assignee)
// that a task's status changed. recordInApp controls whether an in-app
// task_notifications row (for the Dashboard header bell) is written — that
// bell is admin-only today, so it's true only for the creator; other
// assignees get Telegram alone, since they're already looking at their own
// Kanban board where the shared status is already visible.
func (h *TasksHandler) notifyStatusChange(taskID, recipientID int, newStatus string, recordInApp bool) {
	var title string
	if err := h.DB.QueryRow(`SELECT title FROM tasks WHERE id=$1`, taskID).Scan(&title); err != nil {
		log.Printf("[Tasks] notifyStatusChange: load task %d: %v", taskID, err)
		return
	}
	message := fmt.Sprintf("Статус задачи «%s» изменён на: %s", title, taskStatusLabel(newStatus))

	if recordInApp {
		if _, err := h.DB.Exec(
			`INSERT INTO task_notifications (task_id, user_id, status, message) VALUES ($1,$2,$3,$4)`,
			taskID, recipientID, newStatus, message); err != nil {
			log.Printf("[Tasks] notifyStatusChange: insert notification: %v", err)
		}
	}

	var chatID string
	if err := h.DB.QueryRow(`SELECT telegram_chat_id FROM users WHERE id=$1`, recipientID).Scan(&chatID); err != nil {
		log.Printf("[Tasks] notifyStatusChange: load recipient %d: %v", recipientID, err)
		return
	}
	if chatID == "" {
		return
	}
	// Informational only — no interactive buttons on a status-change notice.
	if err := telegram.SendMessage(h.loadBotToken(), chatID, message, nil); err != nil {
		log.Printf("[Tasks] telegram notify %d failed: %v", recipientID, err)
	}
}

func (h *TasksHandler) loadBotToken() string {
	var token string
	_ = h.DB.QueryRow(`SELECT bot_token FROM telegram_settings WHERE id=1`).Scan(&token)
	return token
}

// ── Telegram bot: interactive status buttons ────────────────────────────

// RunTelegramBot long-polls Telegram for inline-button presses on task
// notification messages (see notifyAssignee) and applies the requested
// status change. Runs for the process's lifetime; call as
// `go tasksH.RunTelegramBot()`. The update offset is persisted in
// telegram_settings so a restart doesn't replay already-handled button
// presses.
func (h *TasksHandler) RunTelegramBot() {
	offset := h.loadBotUpdateOffset()
	for {
		token := h.loadBotToken()
		if token == "" {
			time.Sleep(15 * time.Second)
			continue
		}

		updates, err := telegram.GetUpdates(token, offset, 30)
		if err != nil {
			log.Printf("[TelegramBot] getUpdates: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		for _, u := range updates {
			offset = u.UpdateID + 1
			if u.CallbackQuery != nil {
				h.handleTelegramCallback(token, u.CallbackQuery)
			}
		}
		if len(updates) > 0 {
			h.saveBotUpdateOffset(offset)
		}
	}
}

// handleTelegramCallback processes one task-status button press. Only the
// task's assignees may act this way (and, if a primary is designated, only
// the primary — same canChangeStatus rule as the web UI); admins manage
// tasks from the web Kanban instead, so no equivalent path is offered for
// them here.
func (h *TasksHandler) handleTelegramCallback(token string, cb *telegram.CallbackQuery) {
	parts := strings.SplitN(cb.Data, ":", 3)
	if len(parts) != 3 || parts[0] != "task_status" {
		return
	}
	taskID, err := strconv.Atoi(parts[1])
	newStatus := parts[2]
	if err != nil || !validTaskStatuses[newStatus] {
		_ = telegram.AnswerCallbackQuery(token, cb.ID, "Некорректный запрос")
		return
	}

	var userID int
	chatID := strconv.FormatInt(cb.From.ID, 10)
	if err := h.DB.QueryRow(`SELECT id FROM users WHERE telegram_chat_id=$1`, chatID).Scan(&userID); err != nil {
		_ = telegram.AnswerCallbackQuery(token, cb.ID, "Ваш Telegram не привязан ни к одному пользователю")
		return
	}

	var createdBy *int
	if err := h.DB.QueryRow(`SELECT created_by FROM tasks WHERE id=$1`, taskID).Scan(&createdBy); err != nil {
		_ = telegram.AnswerCallbackQuery(token, cb.ID, "Задача не найдена")
		return
	}
	assignees, err := h.loadAssigneeRefs(taskID)
	if err != nil {
		_ = telegram.AnswerCallbackQuery(token, cb.ID, "Ошибка")
		return
	}
	isAssignee := false
	for _, a := range assignees {
		if a.UserID == userID {
			isAssignee = true
			break
		}
	}
	if !isAssignee {
		_ = telegram.AnswerCallbackQuery(token, cb.ID, "Эта задача назначена не вам")
		return
	}
	if p := primaryAssignee(assignees); p != nil && *p != userID {
		_ = telegram.AnswerCallbackQuery(token, cb.ID, "Статус может менять только ответственный за задачу")
		return
	}

	if err := h.updateTaskStatus(taskID, userID, createdBy, assignees, newStatus); err != nil {
		log.Printf("[TelegramBot] update task %d status: %v", taskID, err)
		_ = telegram.AnswerCallbackQuery(token, cb.ID, "Ошибка при обновлении")
		return
	}

	_ = telegram.AnswerCallbackQuery(token, cb.ID, "Статус обновлён: "+taskStatusLabel(newStatus))

	if cb.Message != nil {
		newText := cb.Message.Text + "\n\n✅ Текущий статус: " + taskStatusLabel(newStatus)
		if err := telegram.EditMessageText(token, chatID, cb.Message.MessageID, newText, statusKeyboard(taskID, newStatus)); err != nil {
			log.Printf("[TelegramBot] edit message: %v", err)
		}
	}
}

func (h *TasksHandler) loadBotUpdateOffset() int {
	var offset int
	_ = h.DB.QueryRow(`SELECT update_offset FROM telegram_settings WHERE id=1`).Scan(&offset)
	return offset
}

func (h *TasksHandler) saveBotUpdateOffset(offset int) {
	_, _ = h.DB.Exec(`UPDATE telegram_settings SET update_offset=$1 WHERE id=1`, offset)
}
