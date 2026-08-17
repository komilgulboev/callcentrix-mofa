package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"callcentrix/internal/ami"
	"callcentrix/internal/asterisk"
)

// ProvidersHandler manages SIP provider trunks (carrier connections). A KC
// number is created against a provider so inbound routing/identify knows
// which trunk a DID belongs to. SuperAdmin only.
type ProvidersHandler struct {
	DB  *sql.DB
	AMI *ami.Registry
}

// providerView adds a live connection status to a provider, for display only
// (never persisted). Status is one of: "" (registration disabled for this
// provider — nothing to show), "unknown" (AMI unavailable/query failed), or
// Asterisk's own registration status string ("Registered", "Unregistered",
// "Rejected", ...).
type providerView struct {
	asterisk.Provider
	Status               string `json:"status"`
	OutboundTenantsCount int    `json:"outboundTenantsCount"` // tenants using this trunk for outbound calls
}

// List returns every provider trunk, each with its live connection status
// (queried from Asterisk via AMI — PJSIPShowRegistrationsOutbound) and how
// many tenants have it set as their outbound trunk.
func (h *ProvidersHandler) List(w http.ResponseWriter, r *http.Request) {
	providers, err := asterisk.ListProviders(h.DB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var statuses map[string]string
	var statusErr error
	if h.AMI != nil {
		// Provider trunks currently all live on the default server — queues/
		// inbound routing aren't sharded across boxes yet, so a single
		// client's view is authoritative.
		statuses, statusErr = h.AMI.Get(nil).ProviderRegistrationStatuses()
	}

	outboundCounts := map[int]int{}
	if rows, err := h.DB.QueryContext(r.Context(),
		`SELECT outbound_provider_id, COUNT(*) FROM tenants WHERE outbound_provider_id IS NOT NULL GROUP BY outbound_provider_id`,
	); err == nil {
		for rows.Next() {
			var pid, cnt int
			if rows.Scan(&pid, &cnt) == nil {
				outboundCounts[pid] = cnt
			}
		}
		rows.Close()
	}

	views := make([]providerView, 0, len(providers))
	for _, p := range providers {
		v := providerView{Provider: p, OutboundTenantsCount: outboundCounts[p.ID]}
		switch {
		case !p.Register:
			v.Status = "" // no registration configured for this trunk — nothing to report
		case h.AMI == nil || statusErr != nil:
			v.Status = "unknown"
		default:
			s, ok := statuses[asterisk.ProviderRegistrationID(p.ID)]
			if !ok || s == "" {
				s = "unknown"
			}
			v.Status = s
		}
		views = append(views, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": views})
}

func decodeProviderBody(r *http.Request) (asterisk.Provider, error) {
	var p asterisk.Provider
	err := decode(r, &p)
	return p, err
}

// Create adds a new provider trunk.
func (h *ProvidersHandler) Create(w http.ResponseWriter, r *http.Request) {
	p, err := decodeProviderBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	id, err := asterisk.CreateProvider(h.DB, p)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.AMI != nil {
		h.AMI.PJSIPReloadAll()
	}
	writeJSON(w, http.StatusCreated, map[string]int{"id": id})
}

// Update changes a provider trunk's settings.
func (h *ProvidersHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	p, err := decodeProviderBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	p.ID = id

	if err := asterisk.UpdateProvider(h.DB, p); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.AMI != nil {
		h.AMI.PJSIPReloadAll()
	}
	w.WriteHeader(http.StatusNoContent)
}

// Delete removes a provider trunk (refuses while KC numbers still use it).
func (h *ProvidersHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if err := asterisk.DeleteProvider(h.DB, id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.AMI != nil {
		h.AMI.PJSIPReloadAll()
	}
	w.WriteHeader(http.StatusNoContent)
}
