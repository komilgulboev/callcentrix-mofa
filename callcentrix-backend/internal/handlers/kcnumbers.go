package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"callcentrix/internal/ami"
	"callcentrix/internal/asterisk"
	mw "callcentrix/internal/middleware"
)

// KCNumbersHandler manages call-center DIDs ("номера КЦ"). Creating/removing a
// number is SuperAdmin-only (nested under /api/tenants/{id}/...); everything
// configured for a number (greeting/menu/queue/operators) lives in IVRHandler
// and is reachable by the tenant's own Admin/Supervisor via /api/kc-numbers/{id}/ivr/*.
type KCNumbersHandler struct {
	DB  *sql.DB
	AMI *ami.Registry
}

// tenantID resolves which tenant's numbers to list: SuperAdmin may target any
// tenant via ?tenantId=, everyone else is locked to their own tenant.
func (h *KCNumbersHandler) tenantID(r *http.Request) int {
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

// ListMine returns the KC numbers (+ config status) for the caller's own
// tenant. Used by the IVR page's number overview table.
func (h *KCNumbersHandler) ListMine(w http.ResponseWriter, r *http.Request) {
	tid := h.tenantID(r)
	if tid == 0 {
		writeError(w, http.StatusBadRequest, "tenantId required")
		return
	}
	numbers, err := asterisk.ListKCNumbers(h.DB, tid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"numbers": numbers})
}

// Create adds a new KC number to a tenant (SuperAdmin only).
func (h *KCNumbersHandler) Create(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var body struct {
		Number     string `json:"number"`
		ProviderID int    `json:"providerId"`
	}
	if err := decode(r, &body); err != nil || body.Number == "" {
		writeError(w, http.StatusBadRequest, "number required")
		return
	}

	id, err := asterisk.CreateKCNumber(h.DB, tenantID, body.ProviderID, body.Number)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.AMI != nil {
		h.AMI.DialplanReloadAll()
		h.AMI.PJSIPReloadAll() // the KC number's provider may now route through a different endpoint context
	}
	writeJSON(w, http.StatusCreated, map[string]int{"id": id})
}

// Delete removes a KC number and everything configured under it (SuperAdmin only).
func (h *KCNumbersHandler) Delete(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	numberID, _ := strconv.Atoi(chi.URLParam(r, "numberId"))

	if err := asterisk.DeleteKCNumber(h.DB, tenantID, numberID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.AMI != nil {
		h.AMI.DialplanReloadAll()
		h.AMI.PJSIPReloadAll() // the number's provider may now route through a different endpoint context
	}
	w.WriteHeader(http.StatusNoContent)
}
