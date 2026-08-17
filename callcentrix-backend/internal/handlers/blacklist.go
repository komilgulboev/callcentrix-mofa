package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	mw "callcentrix/internal/middleware"
)

// normalizePhone strips everything but digits, so a blacklist entry matches
// a caller regardless of +/00/country-code formatting differences — e.g. a
// carrier delivering CALLERID(num) as "918111133" must still match an entry
// stored as "+992918111133".
func normalizePhone(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

type BlacklistHandler struct{ DB *sql.DB }

type BlacklistEntry struct {
	ID        int     `json:"id"`
	TenantID  int     `json:"tenantId"`
	Phone     string  `json:"phone"`
	Comment   string  `json:"comment"`
	Active    bool    `json:"active"`
	ExpiresAt *string `json:"expiresAt"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
}

func (h *BlacklistHandler) tenantID(r *http.Request) int {
	c := mw.GetClaims(r)
	if c.UserType == 0 {
		if id, err := strconv.Atoi(r.URL.Query().Get("tenantId")); err == nil && id > 0 {
			return id
		}
		return 0
	}
	if c.TenantID != nil {
		return *c.TenantID
	}
	return 0
}

// expiresAtFromDuration turns a duration-in-days selection into an absolute
// timestamp, nil/0 meaning a permanent block (no expiry).
func expiresAtFromDuration(durationDays *int) *time.Time {
	if durationDays == nil || *durationDays <= 0 {
		return nil
	}
	t := time.Now().Add(time.Duration(*durationDays) * 24 * time.Hour)
	return &t
}

func (h *BlacklistHandler) List(w http.ResponseWriter, r *http.Request) {
	tid := h.tenantID(r)
	if tid == 0 {
		writeError(w, http.StatusBadRequest, "tenantId required")
		return
	}

	search := r.URL.Query().Get("search")
	query := `SELECT id, tenant_id, phone, comment, active, expires_at, created_at, updated_at
	          FROM blacklist WHERE tenant_id = $1`
	args := []any{tid}
	if search != "" {
		query += ` AND (phone ILIKE $2 OR comment ILIKE $2)`
		args = append(args, "%"+search+"%")
	}
	query += ` ORDER BY created_at DESC`

	rows, err := h.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	result := []BlacklistEntry{}
	for rows.Next() {
		var e BlacklistEntry
		var expiresAt sql.NullTime
		if err := rows.Scan(&e.ID, &e.TenantID, &e.Phone, &e.Comment,
			&e.Active, &expiresAt, &e.CreatedAt, &e.UpdatedAt); err != nil {
			continue
		}
		if expiresAt.Valid {
			s := expiresAt.Time.Format(time.RFC3339)
			e.ExpiresAt = &s
		}
		result = append(result, e)
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": result, "total": len(result)})
}

func (h *BlacklistHandler) Create(w http.ResponseWriter, r *http.Request) {
	tid := h.tenantID(r)
	if tid == 0 {
		writeError(w, http.StatusBadRequest, "tenantId required")
		return
	}

	var body struct {
		Phone        string `json:"phone"`
		Comment      string `json:"comment"`
		Active       *bool  `json:"active"`
		DurationDays *int   `json:"durationDays"`
	}
	if err := decode(r, &body); err != nil || body.Phone == "" {
		writeError(w, http.StatusBadRequest, "phone required")
		return
	}

	active := true
	if body.Active != nil {
		active = *body.Active
	}
	expiresAt := expiresAtFromDuration(body.DurationDays)

	// Check duplicate
	var cnt int
	h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM blacklist WHERE tenant_id=$1 AND phone=$2`,
		tid, body.Phone).Scan(&cnt)
	if cnt > 0 {
		writeError(w, http.StatusConflict, "phone_exists")
		return
	}

	var id int
	err := h.DB.QueryRowContext(r.Context(),
		`INSERT INTO blacklist (tenant_id, phone, comment, active, expires_at)
		 VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		tid, body.Phone, body.Comment, active, expiresAt,
	).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int{"id": id})
}

func (h *BlacklistHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	tid := h.tenantID(r)

	var body struct {
		Phone        string `json:"phone"`
		Comment      string `json:"comment"`
		Active       bool   `json:"active"`
		DurationDays *int   `json:"durationDays"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	expiresAt := expiresAtFromDuration(body.DurationDays)

	_, err := h.DB.ExecContext(r.Context(),
		`UPDATE blacklist SET phone=$1, comment=$2, active=$3, expires_at=$4, updated_at=NOW()
		 WHERE id=$5 AND tenant_id=$6`,
		body.Phone, body.Comment, body.Active, expiresAt, id, tid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *BlacklistHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	tid := h.tenantID(r)

	_, err := h.DB.ExecContext(r.Context(),
		`DELETE FROM blacklist WHERE id=$1 AND tenant_id=$2`, id, tid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *BlacklistHandler) Toggle(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	tid := h.tenantID(r)

	_, err := h.DB.ExecContext(r.Context(),
		`UPDATE blacklist SET active = NOT active, updated_at=NOW()
		 WHERE id=$1 AND tenant_id=$2`, id, tid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Check returns whether a phone number is currently blocked (JSON, for API clients).
func (h *BlacklistHandler) Check(w http.ResponseWriter, r *http.Request) {
	phone := r.URL.Query().Get("phone")
	tid := h.tenantID(r)
	if phone == "" {
		writeError(w, http.StatusBadRequest, "phone required")
		return
	}

	np := normalizePhone(phone)
	var cnt int
	h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM blacklist
		 WHERE tenant_id=$1 AND active=TRUE AND (expires_at IS NULL OR expires_at > NOW())
		   AND $2 <> '' AND (
		     regexp_replace(phone,'\D','','g') LIKE '%'||$2
		     OR $2 LIKE '%'||regexp_replace(phone,'\D','','g')
		   )`,
		tid, np).Scan(&cnt)

	writeJSON(w, http.StatusOK, map[string]bool{"blocked": cnt > 0})
}

// CheckPlain is a manual/debug lookup — the dialplan itself checks the
// blacklist directly over ODBC (BLACKLISTCHECK(), see
// asterisk.EnsureBlacklistCheckSubroutine) rather than calling this endpoint.
// Returns plain text "1" (blocked) or "0" (allowed) — no authentication required.
// Protected by ASTERISK_KEY env variable to prevent public abuse.
func (h *BlacklistHandler) CheckPlain(asteriskKey string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Validate internal key
		if asteriskKey != "" && r.URL.Query().Get("key") != asteriskKey {
			http.Error(w, "0", http.StatusForbidden)
			return
		}

		phone := r.URL.Query().Get("phone")
		tenantID, _ := strconv.Atoi(r.URL.Query().Get("tenantId"))
		if phone == "" || tenantID == 0 {
			w.Write([]byte("0"))
			return
		}

		np := normalizePhone(phone)
		var cnt int
		h.DB.QueryRowContext(r.Context(),
			`SELECT COUNT(*) FROM blacklist
			 WHERE tenant_id=$1 AND active=TRUE AND (expires_at IS NULL OR expires_at > NOW())
			   AND $2 <> '' AND (
			     regexp_replace(phone,'\D','','g') LIKE '%'||$2
			     OR $2 LIKE '%'||regexp_replace(phone,'\D','','g')
			   )`,
			tenantID, np).Scan(&cnt)

		if cnt > 0 {
			w.Write([]byte("1"))
		} else {
			w.Write([]byte("0"))
		}
	}
}
