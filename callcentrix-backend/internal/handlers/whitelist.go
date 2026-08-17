package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	mw "callcentrix/internal/middleware"
)

type WhitelistHandler struct{ DB *sql.DB }

type WhitelistEntry struct {
	ID        int    `json:"id"`
	TenantID  int    `json:"tenantId"`
	Phone     string `json:"phone"`
	Comment   string `json:"comment"`
	Active    bool   `json:"active"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func (h *WhitelistHandler) tenantID(r *http.Request) int {
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

func (h *WhitelistHandler) List(w http.ResponseWriter, r *http.Request) {
	tid := h.tenantID(r)
	if tid == 0 {
		writeError(w, http.StatusBadRequest, "tenantId required")
		return
	}

	search := r.URL.Query().Get("search")
	query := `SELECT id, tenant_id, phone, comment, active, created_at, updated_at
	          FROM whitelist WHERE tenant_id = $1`
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

	result := []WhitelistEntry{}
	for rows.Next() {
		var e WhitelistEntry
		if err := rows.Scan(&e.ID, &e.TenantID, &e.Phone, &e.Comment,
			&e.Active, &e.CreatedAt, &e.UpdatedAt); err != nil {
			continue
		}
		result = append(result, e)
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": result, "total": len(result)})
}

func (h *WhitelistHandler) Create(w http.ResponseWriter, r *http.Request) {
	tid := h.tenantID(r)
	if tid == 0 {
		writeError(w, http.StatusBadRequest, "tenantId required")
		return
	}

	var body struct {
		Phone   string `json:"phone"`
		Comment string `json:"comment"`
		Active  *bool  `json:"active"`
	}
	if err := decode(r, &body); err != nil || body.Phone == "" {
		writeError(w, http.StatusBadRequest, "phone required")
		return
	}

	active := true
	if body.Active != nil {
		active = *body.Active
	}

	// Check duplicate
	var cnt int
	h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM whitelist WHERE tenant_id=$1 AND phone=$2`,
		tid, body.Phone).Scan(&cnt)
	if cnt > 0 {
		writeError(w, http.StatusConflict, "phone_exists")
		return
	}

	var id int
	err := h.DB.QueryRowContext(r.Context(),
		`INSERT INTO whitelist (tenant_id, phone, comment, active)
		 VALUES ($1,$2,$3,$4) RETURNING id`,
		tid, body.Phone, body.Comment, active,
	).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int{"id": id})
}

func (h *WhitelistHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	tid := h.tenantID(r)

	var body struct {
		Phone   string `json:"phone"`
		Comment string `json:"comment"`
		Active  bool   `json:"active"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	_, err := h.DB.ExecContext(r.Context(),
		`UPDATE whitelist SET phone=$1, comment=$2, active=$3, updated_at=NOW()
		 WHERE id=$4 AND tenant_id=$5`,
		body.Phone, body.Comment, body.Active, id, tid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *WhitelistHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	tid := h.tenantID(r)

	_, err := h.DB.ExecContext(r.Context(),
		`DELETE FROM whitelist WHERE id=$1 AND tenant_id=$2`, id, tid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *WhitelistHandler) Toggle(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	tid := h.tenantID(r)

	_, err := h.DB.ExecContext(r.Context(),
		`UPDATE whitelist SET active = NOT active, updated_at=NOW()
		 WHERE id=$1 AND tenant_id=$2`, id, tid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Check returns whether a phone number is currently allowed by the whitelist
// (JSON, for API clients).
func (h *WhitelistHandler) Check(w http.ResponseWriter, r *http.Request) {
	phone := r.URL.Query().Get("phone")
	tid := h.tenantID(r)
	if phone == "" {
		writeError(w, http.StatusBadRequest, "phone required")
		return
	}

	np := normalizePhone(phone)
	var cnt int
	h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM whitelist
		 WHERE tenant_id=$1 AND active=TRUE
		   AND $2 <> '' AND (
		     regexp_replace(phone,'\D','','g') LIKE '%'||$2
		     OR $2 LIKE '%'||regexp_replace(phone,'\D','','g')
		   )`,
		tid, np).Scan(&cnt)

	writeJSON(w, http.StatusOK, map[string]bool{"allowed": cnt > 0})
}

// CheckPlain is a manual/debug lookup — the dialplan itself checks the
// whitelist directly over ODBC (ODBC_WHITELISTCHECK(), see
// asterisk.EnsureWhitelistCheckSubroutine) rather than calling this endpoint.
// Returns plain text "1" (allowed) or "0" (not allowed) — no authentication
// required beyond the ASTERISK_KEY env variable, to prevent public abuse.
func (h *WhitelistHandler) CheckPlain(asteriskKey string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
			`SELECT COUNT(*) FROM whitelist
			 WHERE tenant_id=$1 AND active=TRUE
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
