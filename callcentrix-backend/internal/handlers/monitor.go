package handlers

import (
	"database/sql"
	"net/http"

	"callcentrix/internal/ami"
	mw "callcentrix/internal/middleware"
	"callcentrix/internal/ws"
)

type MonitorHandler struct {
	DB      *sql.DB
	Monitor *ami.Monitor
	Hub     *ws.Hub
	AMI     *ami.Registry
}

// clientFor resolves the AMI client to dispatch an action against for a
// known live channel, falling back to the registry's default single-server
// client when the channel isn't (yet) tracked by the monitor.
func (h *MonitorHandler) clientFor(channel string) *ami.Client {
	if c := h.Monitor.ClientForChannel(channel); c != nil {
		return c
	}
	return h.AMI.Get(nil)
}

// clientForAgent is clientFor's counterpart keyed by agent extension —
// needed for actions like pause/unpause where the agent may have no active
// channel at all right now.
func (h *MonitorHandler) clientForAgent(ext string) *ami.Client {
	if c := h.Monitor.ClientForAgent(ext); c != nil {
		return c
	}
	return h.AMI.Get(nil)
}

// ServeWS upgrades to WebSocket — auth via ?token= query param
func (h *MonitorHandler) ServeWS(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	if c == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	h.Hub.ServeWS(w, r)
}

// Snapshot returns current state as JSON (for polling fallback)
func (h *MonitorHandler) Snapshot(w http.ResponseWriter, r *http.Request) {
	snap := h.Monitor.Snapshot()
	writeJSON(w, http.StatusOK, snap)
}

// TenantSnapshot returns the AMI monitor snapshot scoped to the requesting
// user's own tenant — SuperAdmin (who isn't attached to any tenant) gets the
// unfiltered view. Unlike Snapshot, this isn't gated to Supervisor+: every
// role needs it for the dashboard's live agent/call widget, Operators
// included, so it must never leak another tenant's data.
func (h *MonitorHandler) TenantSnapshot(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)
	if c.TenantID == nil {
		writeJSON(w, http.StatusOK, h.Monitor.Snapshot())
		return
	}

	exts := map[string]bool{}
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT sip_no FROM users WHERE tenant_id = $1 AND sip_no != ''`, *c.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for rows.Next() {
		var sipNo string
		if err := rows.Scan(&sipNo); err == nil {
			exts[sipNo] = true
		}
	}
	rows.Close()

	queueNames := map[string]bool{}
	qRows, err := h.DB.QueryContext(r.Context(), `
		SELECT DISTINCT qm.queue_name
		FROM ast_queue_members qm
		JOIN users u ON u.sip_no = split_part(qm.interface, '/', 2)
		WHERE u.tenant_id = $1`, *c.TenantID)
	if err == nil {
		for qRows.Next() {
			var name string
			if err := qRows.Scan(&name); err == nil {
				queueNames[name] = true
			}
		}
		qRows.Close()
	}

	writeJSON(w, http.StatusOK, h.Monitor.SnapshotForTenant(exts, queueNames))
}

// AgentsInfo returns agent names from users table
func (h *MonitorHandler) AgentsInfo(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT sip_no, first_name, last_name FROM users WHERE sip_no != '' AND active = TRUE`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type Agent struct {
		SipNo     string `json:"sipNo"`
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
	}
	agents := []Agent{}
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.SipNo, &a.FirstName, &a.LastName); err != nil {
			continue
		}
		agents = append(agents, a)
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": agents})
}

// Pause pauses an agent in the queue
func (h *MonitorHandler) Pause(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AgentID string `json:"agentId"`
	}
	if err := decode(r, &body); err != nil || body.AgentID == "" {
		writeError(w, http.StatusBadRequest, "agentId required")
		return
	}
	h.clientForAgent(body.AgentID).PauseQueueMember("PJSIP/"+body.AgentID, true)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Unpause resumes an agent in the queue
func (h *MonitorHandler) Unpause(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AgentID string `json:"agentId"`
	}
	if err := decode(r, &body); err != nil || body.AgentID == "" {
		writeError(w, http.StatusBadRequest, "agentId required")
		return
	}
	h.clientForAgent(body.AgentID).PauseQueueMember("PJSIP/"+body.AgentID, false)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Hangup terminates an active channel
func (h *MonitorHandler) Hangup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Channel string `json:"channel"`
	}
	if err := decode(r, &body); err != nil || body.Channel == "" {
		writeError(w, http.StatusBadRequest, "channel required")
		return
	}
	h.clientFor(body.Channel).HangupChannel(body.Channel)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
