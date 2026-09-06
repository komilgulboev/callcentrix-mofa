package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	mw "callcentrix/internal/middleware"
)

// SitesHandler manages the "Сайты" (Sites) catalog — same shape and access
// pattern as TopicsHandler (see topics.go): a per-tenant, admin-managed
// directory that operators pick from when filing a ticket, independent of
// the ticket's topic.
type SitesHandler struct{ DB *sql.DB }

type Site struct {
	ID        int               `json:"id"`
	TenantID  int               `json:"tenantId"`
	Names     map[string]string `json:"names"`
	Active    bool              `json:"active"`
	CreatedAt string            `json:"createdAt"`
	UpdatedAt string            `json:"updatedAt"`
}

func (h *SitesHandler) List(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	tenantID, _ := strconv.Atoi(chi.URLParam(r, "id"))

	if c.UserType != 0 && (c.TenantID == nil || *c.TenantID != tenantID) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, tenant_id, names, active, created_at, updated_at
		 FROM site_catalog WHERE tenant_id = $1 ORDER BY id`, tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	result := []Site{}
	for rows.Next() {
		var s Site
		var namesJSON []byte
		if err := rows.Scan(&s.ID, &s.TenantID, &namesJSON, &s.Active, &s.CreatedAt, &s.UpdatedAt); err != nil {
			continue
		}
		if err := json.Unmarshal(namesJSON, &s.Names); err != nil || s.Names == nil {
			s.Names = map[string]string{}
		}
		result = append(result, s)
	}
	writeJSON(w, http.StatusOK, map[string]any{"sites": result})
}

func (h *SitesHandler) Create(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	tenantID, _ := strconv.Atoi(chi.URLParam(r, "id"))

	if c.UserType != 0 && (c.TenantID == nil || *c.TenantID != tenantID) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	var body struct {
		Names  map[string]string `json:"names"`
		Active *bool             `json:"active"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(body.Names) == 0 {
		writeError(w, http.StatusBadRequest, "names required")
		return
	}

	active := true
	if body.Active != nil {
		active = *body.Active
	}

	namesJSON, err := json.Marshal(body.Names)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid names")
		return
	}

	var id int
	err = h.DB.QueryRowContext(r.Context(),
		`INSERT INTO site_catalog (tenant_id, names, active) VALUES ($1,$2,$3) RETURNING id`,
		tenantID, namesJSON, active,
	).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int{"id": id})
}

func (h *SitesHandler) Update(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	tenantID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteId"))

	if c.UserType != 0 && (c.TenantID == nil || *c.TenantID != tenantID) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	var body struct {
		Names  map[string]string `json:"names"`
		Active *bool             `json:"active"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	active := true
	if body.Active != nil {
		active = *body.Active
	}

	namesJSON, err := json.Marshal(body.Names)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid names")
		return
	}

	result, err := h.DB.ExecContext(r.Context(),
		`UPDATE site_catalog SET names=$1, active=$2, updated_at=NOW() WHERE id=$3 AND tenant_id=$4`,
		namesJSON, active, siteID, tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SitesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	tenantID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteId"))

	if c.UserType != 0 && (c.TenantID == nil || *c.TenantID != tenantID) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	_, err := h.DB.ExecContext(r.Context(),
		`DELETE FROM site_catalog WHERE id=$1 AND tenant_id=$2`, siteID, tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListMy returns active sites for the current user's tenant (all roles).
// Used by operators when creating tickets.
func (h *SitesHandler) ListMy(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	if c.TenantID == nil {
		writeJSON(w, http.StatusOK, map[string]any{"sites": []Site{}})
		return
	}

	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, tenant_id, names, active, created_at, updated_at
		 FROM site_catalog WHERE tenant_id = $1 AND active = TRUE ORDER BY id`, *c.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	result := []Site{}
	for rows.Next() {
		var s Site
		var namesJSON []byte
		if err := rows.Scan(&s.ID, &s.TenantID, &namesJSON, &s.Active, &s.CreatedAt, &s.UpdatedAt); err != nil {
			continue
		}
		if err := json.Unmarshal(namesJSON, &s.Names); err != nil || s.Names == nil {
			s.Names = map[string]string{}
		}
		result = append(result, s)
	}
	writeJSON(w, http.StatusOK, map[string]any{"sites": result})
}

// SiteInfo — minimal site embedded in ticket responses
type SiteInfo struct {
	ID    int               `json:"id"`
	Names map[string]string `json:"names"`
}

func scanSiteInfo(namesJSON []byte, id int) *SiteInfo {
	if namesJSON == nil {
		return nil
	}
	s := &SiteInfo{ID: id}
	if err := json.Unmarshal(namesJSON, &s.Names); err != nil || s.Names == nil {
		s.Names = map[string]string{}
	}
	return s
}

// suppress unused warning for scanSiteInfo
var _ = scanSiteInfo
