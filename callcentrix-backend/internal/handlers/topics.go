package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	mw "callcentrix/internal/middleware"
)

type TopicsHandler struct{ DB *sql.DB }

type Topic struct {
	ID        int               `json:"id"`
	TenantID  int               `json:"tenantId"`
	Names     map[string]string `json:"names"`
	Active    bool              `json:"active"`
	CreatedAt string            `json:"createdAt"`
	UpdatedAt string            `json:"updatedAt"`
}

func (h *TopicsHandler) List(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	tenantID, _ := strconv.Atoi(chi.URLParam(r, "id"))

	if c.UserType != 0 && (c.TenantID == nil || *c.TenantID != tenantID) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, tenant_id, names, active, created_at, updated_at
		 FROM topic_catalog WHERE tenant_id = $1 ORDER BY id`, tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	result := []Topic{}
	for rows.Next() {
		var t Topic
		var namesJSON []byte
		if err := rows.Scan(&t.ID, &t.TenantID, &namesJSON, &t.Active, &t.CreatedAt, &t.UpdatedAt); err != nil {
			continue
		}
		if err := json.Unmarshal(namesJSON, &t.Names); err != nil || t.Names == nil {
			t.Names = map[string]string{}
		}
		result = append(result, t)
	}
	writeJSON(w, http.StatusOK, map[string]any{"topics": result})
}

func (h *TopicsHandler) Create(w http.ResponseWriter, r *http.Request) {
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
		`INSERT INTO topic_catalog (tenant_id, names, active) VALUES ($1,$2,$3) RETURNING id`,
		tenantID, namesJSON, active,
	).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int{"id": id})
}

func (h *TopicsHandler) Update(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	tenantID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	topicID, _ := strconv.Atoi(chi.URLParam(r, "topicId"))

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
		`UPDATE topic_catalog SET names=$1, active=$2, updated_at=NOW() WHERE id=$3 AND tenant_id=$4`,
		namesJSON, active, topicID, tenantID)
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

func (h *TopicsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	tenantID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	topicID, _ := strconv.Atoi(chi.URLParam(r, "topicId"))

	if c.UserType != 0 && (c.TenantID == nil || *c.TenantID != tenantID) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	_, err := h.DB.ExecContext(r.Context(),
		`DELETE FROM topic_catalog WHERE id=$1 AND tenant_id=$2`, topicID, tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListMy returns active topics for the current user's tenant (all roles).
// Used by operators when creating tickets.
func (h *TopicsHandler) ListMy(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	if c.TenantID == nil {
		writeJSON(w, http.StatusOK, map[string]any{"topics": []Topic{}})
		return
	}

	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, tenant_id, names, active, created_at, updated_at
		 FROM topic_catalog WHERE tenant_id = $1 AND active = TRUE ORDER BY id`, *c.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	result := []Topic{}
	for rows.Next() {
		var t Topic
		var namesJSON []byte
		if err := rows.Scan(&t.ID, &t.TenantID, &namesJSON, &t.Active, &t.CreatedAt, &t.UpdatedAt); err != nil {
			continue
		}
		if err := json.Unmarshal(namesJSON, &t.Names); err != nil || t.Names == nil {
			t.Names = map[string]string{}
		}
		result = append(result, t)
	}
	writeJSON(w, http.StatusOK, map[string]any{"topics": result})
}

// TopicInfo — minimal topic embedded in ticket responses
type TopicInfo struct {
	ID    int               `json:"id"`
	Names map[string]string `json:"names"`
}

func scanTopicInfo(namesJSON []byte, id int) *TopicInfo {
	if namesJSON == nil {
		return nil
	}
	t := &TopicInfo{ID: id}
	if err := json.Unmarshal(namesJSON, &t.Names); err != nil || t.Names == nil {
		t.Names = map[string]string{}
	}
	return t
}

// GetTopicNames fetches names JSONB for use in ticket JOIN scan
func GetTopicNamesFromJSON(b []byte) map[string]string {
	if b == nil {
		return nil
	}
	m := map[string]string{}
	_ = json.Unmarshal(b, &m)
	return m
}

// TopicByID returns a topic or nil (used in ticket handlers)
func (h *TopicsHandler) TopicByID(r *http.Request, id int) *TopicInfo {
	var namesJSON []byte
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT id, names FROM topic_catalog WHERE id=$1`, id).
		Scan(&id, &namesJSON)
	if err != nil {
		return nil
	}
	return scanTopicInfo(namesJSON, id)
}

// suppress unused warning for scanTopicInfo
var _ = scanTopicInfo
