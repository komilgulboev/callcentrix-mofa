package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"callcentrix/internal/ami"
	"callcentrix/internal/asterisk"
	mw "callcentrix/internal/middleware"
)

type PhoneHandler struct {
	DB            *sql.DB
	AsteriskWSURI string // fallback for users with no server_id assigned
	SIPDomain     string
	Monitor       *ami.Monitor
	AMI           *ami.Registry
}

// resolveAsteriskWSURI returns the WS URI of the Asterisk server userID is
// assigned to, falling back to the single default when unassigned or no
// matching active server is configured — see ServeWS.
func (h *PhoneHandler) resolveAsteriskWSURI(ctx context.Context, userID int) string {
	var wsURI sql.NullString
	err := h.DB.QueryRowContext(ctx, `
		SELECT s.ws_uri FROM users u
		JOIN asterisk_servers s ON s.id = u.server_id AND s.active = TRUE
		WHERE u.id = $1`, userID).Scan(&wsURI)
	if err != nil || !wsURI.Valid || wsURI.String == "" {
		return h.AsteriskWSURI
	}
	return wsURI.String
}

type PhoneConfig struct {
	WSUri       string `json:"wsUri"`
	SIPUri      string `json:"sipUri"`
	SIPDomain   string `json:"sipDomain"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
}

func (h *PhoneHandler) Config(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)

	var sipNo, sipPassword, firstName, lastName string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT sip_no, sip_password, first_name, last_name FROM users WHERE id = $1`, c.Sub,
	).Scan(&sipNo, &sipPassword, &firstName, &lastName)

	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sipNo == "" {
		writeError(w, http.StatusNotFound, "no SIP extension assigned")
		return
	}

	displayName := fmt.Sprintf("%s %s", firstName, lastName)
	if displayName == " " {
		displayName = c.Username
	}

	cfg := PhoneConfig{
		// Same-origin proxy (see ServeWS) — browsers never talk to Asterisk directly.
		WSUri:       buildPhoneWSURI(r),
		SIPUri:      fmt.Sprintf("sip:%s@%s", sipNo, h.SIPDomain),
		SIPDomain:   h.SIPDomain,
		Password:    sipPassword,
		DisplayName: displayName,
	}
	writeJSON(w, http.StatusOK, cfg)
}

// ActiveCall reports whether the authenticated agent's extension currently
// has a live channel on Asterisk, per the AMI monitor. The webphone calls
// this on load to rehydrate call info/timer after a page reload, since a
// full reload destroys the JsSIP session and there's no way to reattach to
// it directly — this is the source of truth the client's own optimistic
// sessionStorage guess gets reconciled against.
func (h *PhoneHandler) ActiveCall(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)

	var sipNo string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT sip_no FROM users WHERE id = $1`, c.Sub,
	).Scan(&sipNo)
	if err != nil || sipNo == "" {
		writeJSON(w, http.StatusOK, map[string]any{"active": false})
		return
	}

	snap := h.Monitor.Snapshot()
	agent, ok := snap.Agents[sipNo]
	if !ok || agent.Channel == "" {
		writeJSON(w, http.StatusOK, map[string]any{"active": false})
		return
	}
	call, ok := snap.Calls[agent.Channel]
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"active": false})
		return
	}

	remoteNumber := call.Src
	if call.Src == sipNo {
		remoteNumber = call.Dst
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"active":       true,
		"channel":      call.Channel,
		"remoteNumber": remoteNumber,
		"onHold":       agent.Status == "on_hold",
		"duration":     call.Duration,
	})
}

// Hangup terminates the caller's own currently active channel via AMI. The
// webphone falls back to this when it has rehydrated a reattached call
// (after a page reload) and so has no live JsSIP session to hang up locally.
func (h *PhoneHandler) Hangup(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)

	var body struct {
		Channel string `json:"channel"`
	}
	if err := decode(r, &body); err != nil || body.Channel == "" {
		writeError(w, http.StatusBadRequest, "channel required")
		return
	}

	var sipNo string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT sip_no FROM users WHERE id = $1`, c.Sub,
	).Scan(&sipNo)
	if err != nil || sipNo == "" {
		writeError(w, http.StatusForbidden, "no SIP extension assigned")
		return
	}

	// Only allow hanging up a channel AMI currently attributes to this
	// agent's own extension — prevents hanging up someone else's call.
	agent, ok := h.Monitor.Snapshot().Agents[sipNo]
	if !ok || agent.Channel != body.Channel {
		writeError(w, http.StatusForbidden, "not your active channel")
		return
	}

	client := h.Monitor.ClientForChannel(body.Channel)
	if client == nil {
		client = h.AMI.Get(nil)
	}
	client.HangupChannel(body.Channel)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ResumeCall claims any caller still held on Asterisk waiting for this agent
// to come back (see ami.Monitor.pendingReconnect, populated by BridgeLeave
// when an agent's leg vanishes mid-call) and redirects them into a fresh
// Dial() at the agent's own extension. From the browser's side this just
// arrives as an ordinary new incoming call — no special "reconnected" UI
// path needed, it reuses the existing ringing/answer flow. Call this once
// the webphone has re-registered after a page reload.
func (h *PhoneHandler) ResumeCall(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)

	var sipNo string
	var tenantID int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT sip_no, COALESCE(tenant_id, 0) FROM users WHERE id = $1`, c.Sub,
	).Scan(&sipNo, &tenantID)
	if err != nil || sipNo == "" || tenantID == 0 {
		writeJSON(w, http.StatusOK, map[string]bool{"resumed": false})
		return
	}

	callerChannel, ok := h.Monitor.ClaimPendingReconnect(sipNo)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]bool{"resumed": false})
		return
	}

	client := h.Monitor.ClientForChannel(callerChannel)
	if client == nil {
		client = h.AMI.Get(nil)
	}
	if err := client.Redirect(callerChannel, asterisk.TenantContext(tenantID), sipNo, "1"); err != nil {
		writeJSON(w, http.StatusOK, map[string]bool{"resumed": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"resumed": true})
}

// HangupIntent is a best-effort signal that the agent is about to end their
// current call deliberately (see store/phone.js hangup()) — lets the
// monitor skip holding the caller for a possible reconnect when the agent's
// leg disappears moments later, so an ordinary call end isn't delayed by the
// reconnect grace window. See Monitor.MarkDeliberateHangup for how the race
// against the actual SIP BYE is handled.
func (h *PhoneHandler) HangupIntent(w http.ResponseWriter, r *http.Request) {
	c := mw.GetClaims(r)

	var sipNo string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT sip_no FROM users WHERE id = $1`, c.Sub,
	).Scan(&sipNo)
	if err != nil || sipNo == "" {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": false})
		return
	}
	h.Monitor.MarkDeliberateHangup(sipNo)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func buildPhoneWSURI(r *http.Request) string {
	scheme := "ws"
	host := r.Host

	if forwardedProto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwardedProto != "" {
		scheme = strings.ToLower(strings.Split(forwardedProto, ",")[0])
	}
	if forwardedHost := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); forwardedHost != "" {
		host = forwardedHost
	}
	if r.TLS != nil || scheme == "https" || strings.EqualFold(scheme, "wss") {
		scheme = "wss"
	} else {
		scheme = "ws"
	}

	return fmt.Sprintf("%s://%s/ws/phone", scheme, host)
}
