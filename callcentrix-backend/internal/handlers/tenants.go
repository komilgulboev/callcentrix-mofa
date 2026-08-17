package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"callcentrix/internal/ami"
	"callcentrix/internal/asterisk"
	mw "callcentrix/internal/middleware"
)

type TenantsHandler struct {
	DB  *sql.DB
	AMI *ami.Registry
}

type Tenant struct {
	ID                 int     `json:"id"`
	Name               string  `json:"name"`
	Domain             *string `json:"domain"`
	MaxUsers           int     `json:"maxUsers"`
	Active             bool    `json:"active"`
	CreatedAt          string  `json:"createdAt"`
	OutboundProviderID *int    `json:"outboundProviderId"`
	OutboundCallerID   string  `json:"outboundCallerId"`
}

func (h *TenantsHandler) List(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	var rows *sql.Rows
	var err error
	if c.UserType == 0 {
		rows, err = h.DB.QueryContext(r.Context(),
			`SELECT id, name, domain, max_users, active, created_at, outbound_provider_id, outbound_caller_id FROM tenants ORDER BY id`)
	} else {
		rows, err = h.DB.QueryContext(r.Context(),
			`SELECT id, name, domain, max_users, active, created_at, outbound_provider_id, outbound_caller_id FROM tenants WHERE id = $1`, c.TenantID)
	}
	if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }
	defer rows.Close()

	result := []Tenant{}
	for rows.Next() {
		var t Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.Domain, &t.MaxUsers, &t.Active, &t.CreatedAt,
			&t.OutboundProviderID, &t.OutboundCallerID); err != nil { continue }
		result = append(result, t)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tenants": result})
}

func (h *TenantsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var t Tenant
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT id, name, domain, max_users, active, created_at, outbound_provider_id, outbound_caller_id FROM tenants WHERE id = $1`, id,
	).Scan(&t.ID, &t.Name, &t.Domain, &t.MaxUsers, &t.Active, &t.CreatedAt,
		&t.OutboundProviderID, &t.OutboundCallerID)
	if err == sql.ErrNoRows { writeError(w, http.StatusNotFound, "not found"); return }
	if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }
	writeJSON(w, http.StatusOK, t)
}

func (h *TenantsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name               string  `json:"name"`
		Domain             *string `json:"domain"`
		MaxUsers           int     `json:"maxUsers"`
		OutboundProviderID *int    `json:"outboundProviderId"`
		OutboundCallerID   string  `json:"outboundCallerId"`
	}
	if err := decode(r, &body); err != nil { writeError(w, http.StatusBadRequest, "invalid body"); return }
	if body.MaxUsers == 0 { body.MaxUsers = 50 }
	var id int
	err := h.DB.QueryRowContext(r.Context(),
		`INSERT INTO tenants (name, domain, max_users, outbound_provider_id, outbound_caller_id) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		body.Name, body.Domain, body.MaxUsers, body.OutboundProviderID, body.OutboundCallerID,
	).Scan(&id)
	if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }

	// Create Asterisk dialplan context for this tenant (internal calling by
	// mobile number, plus its outbound-trunk rule if one was set above).
	if err := asterisk.CreateTenantContext(h.DB, id); err != nil {
		log.Printf("[Tenants] Dialplan context warning for tenant %d: %v", id, err)
	} else if h.AMI != nil {
		h.AMI.DialplanReloadAll()
	}

	// KC numbers (and their own IVR/queue) are added separately by SuperAdmin afterwards.

	writeJSON(w, http.StatusCreated, map[string]int{"id": id})
}

func (h *TenantsHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var body struct {
		Name               string  `json:"name"`
		Domain             *string `json:"domain"`
		MaxUsers           int     `json:"maxUsers"`
		OutboundProviderID *int    `json:"outboundProviderId"`
		OutboundCallerID   string  `json:"outboundCallerId"`
	}
	if err := decode(r, &body); err != nil { writeError(w, http.StatusBadRequest, "invalid body"); return }
	_, err := h.DB.ExecContext(r.Context(),
		`UPDATE tenants SET name=$1, domain=$2, max_users=$3, outbound_provider_id=$4, outbound_caller_id=$5, updated_at=NOW() WHERE id=$6`,
		body.Name, body.Domain, body.MaxUsers, body.OutboundProviderID, body.OutboundCallerID, id)
	if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }

	// Regenerate the tenant's dialplan so a changed outbound trunk (or its
	// removal) takes effect immediately.
	if err := asterisk.CreateTenantContext(h.DB, id); err != nil {
		log.Printf("[Tenants] Dialplan context warning on update for tenant %d: %v", id, err)
	} else if h.AMI != nil {
		h.AMI.DialplanReloadAll()
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *TenantsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	// Remove Asterisk dialplan context before deleting tenant.
	// KC numbers cascade-delete at the DB level (ON DELETE CASCADE), but that
	// only removes our own rows — their Asterisk-side context/queue/inbound
	// route are cleaned up explicitly first, same as DeleteKCNumber would.
	if err := asterisk.DeleteTenantContext(h.DB, id); err != nil {
		log.Printf("[Tenants] Dialplan context delete warning for tenant %d: %v", id, err)
	}
	numbers, _ := asterisk.ListKCNumbers(h.DB, id)
	for _, n := range numbers {
		if err := asterisk.DeleteKCNumber(h.DB, id, n.ID); err != nil {
			log.Printf("[Tenants] KC number delete warning (tenant %d, number %s): %v", id, n.Number, err)
		}
	}

	_, err := h.DB.ExecContext(r.Context(), `DELETE FROM tenants WHERE id=$1`, id)
	if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }
	w.WriteHeader(http.StatusNoContent)
}

func (h *TenantsHandler) Activate(w http.ResponseWriter, r *http.Request)   { h.setActive(w, r, true) }
func (h *TenantsHandler) Deactivate(w http.ResponseWriter, r *http.Request) { h.setActive(w, r, false) }

func (h *TenantsHandler) setActive(w http.ResponseWriter, r *http.Request, active bool) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	_, err := h.DB.ExecContext(r.Context(),
		`UPDATE tenants SET active=$1, updated_at=NOW() WHERE id=$2`, active, id)
	if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }
	w.WriteHeader(http.StatusNoContent)
}

// AssignUser sets tenant_id and updates SIP context in Asterisk
func (h *TenantsHandler) AssignUser(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var body struct {
		UserID int `json:"userId"`
	}
	if err := decode(r, &body); err != nil || body.UserID == 0 {
		writeError(w, http.StatusBadRequest, "userId required")
		return
	}

	// Get username for Asterisk update
	var username string
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT username FROM users WHERE id=$1`, body.UserID).Scan(&username)

	_, err := h.DB.ExecContext(r.Context(),
		`UPDATE users SET tenant_id=$1, updated_at=NOW() WHERE id=$2`, tenantID, body.UserID)
	if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }

	if username != "" {
		ctx := asterisk.TenantContext(tenantID)

		// Ensure dialplan context exists for this tenant (idempotent)
		if err := asterisk.CreateTenantContext(h.DB, tenantID); err != nil {
			log.Printf("[Tenants] Dialplan context warning on assign user: %v", err)
		} else if h.AMI != nil {
			h.AMI.DialplanReloadAll()
		}

		// Move user's SIP endpoint to tenant's dialplan context
		_ = asterisk.UpdateSIPContext(h.DB, username, ctx)
	}

	w.WriteHeader(http.StatusNoContent)
}

// UnassignUser clears tenant_id and resets context to "default"
func (h *TenantsHandler) UnassignUser(w http.ResponseWriter, r *http.Request) {
	userID, _ := strconv.Atoi(chi.URLParam(r, "userId"))

	var username string
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT username FROM users WHERE id=$1`, userID).Scan(&username)

	_, err := h.DB.ExecContext(r.Context(),
		`UPDATE users SET tenant_id=NULL, updated_at=NOW() WHERE id=$1`, userID)
	if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }

	if username != "" {
		_ = asterisk.UpdateSIPContext(h.DB, username, "default")
	}

	w.WriteHeader(http.StatusNoContent)
}
