package ami

import (
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
)

type AgentState struct {
	SipNo        string `json:"sipNo"`
	Name         string `json:"name"`
	Status       string `json:"status"`    // available busy paused ringing offline
	Direction    string `json:"direction"` // "in" (being called) | "out" (placing a call) | ""
	CallDuration int    `json:"callDuration"`
	Channel      string `json:"channel"`
	CallerNumber string `json:"callerNumber"` // joined from calls[Channel].Src, see Snapshot
	CalleeNumber string `json:"calleeNumber"` // joined from calls[Channel].Dst, see Snapshot

	// "First seen online today" and "paused time today" — both reset at
	// local midnight, lazily (checked against the current date on write and
	// on Snapshot rather than via a timer).
	FirstOnlineToday   string `json:"firstOnlineToday"`   // "HH:MM", "" if not seen yet today
	PausedTodaySeconds int    `json:"pausedTodaySeconds"`

	firstOnlineDate string    // date FirstOnlineToday was recorded for
	pauseStartedAt  time.Time // zero if not currently paused
	pausedDate      string    // date PausedTodaySeconds has been accumulating for
}

type LiveCall struct {
	Channel   string `json:"channel"`
	Src       string `json:"src"`
	Dst       string `json:"dst"`
	Duration  int    `json:"duration"`
	StartTime int64  `json:"-"`
}

type QueueStats struct {
	Name      string `json:"name"`
	Waiting   int    `json:"waiting"`
	Completed int    `json:"completed"`
	Members   int    `json:"members"`
}

// WaitingCaller is one caller currently on hold in a queue, not yet connected
// to an agent.
type WaitingCaller struct {
	Channel     string `json:"channel"`
	Queue       string `json:"queue"`
	CallerID    string `json:"callerId"`
	WaitSeconds int    `json:"waitSeconds"`
	JoinedAt    int64  `json:"-"`
}

type Snapshot struct {
	Type           string                `json:"type"`
	Agents         map[string]AgentState `json:"agents"`
	Calls          map[string]LiveCall   `json:"calls"`
	Queues         map[string]QueueStats `json:"queues"`
	WaitingCallers []WaitingCaller       `json:"waitingCallers"`
}

// pendingReconnect records a caller still held in the dialplan (see
// writeKCDialplan's post-Queue Wait) waiting to see whether the agent whose
// leg just vanished mid-call comes back within the grace window.
type pendingReconnect struct {
	callerChannel string
	expiresAt     time.Time
}

// Must match the Wait() duration writeKCDialplan puts after Queue() — that's
// how long the caller is actually held on Asterisk's side, this is just our
// own bookkeeping of the same deadline.
const pendingReconnectTTL = 5 * time.Second

// How long a MarkDeliberateHangup flag stays valid — just long enough to
// race against the SIP BYE / AMI Hangup+BridgeLeave it's meant to precede.
const deliberateIntentTTL = 3 * time.Second

type Monitor struct {
	mu             sync.RWMutex
	agents         map[string]AgentState
	calls          map[string]LiveCall
	queues         map[string]QueueStats
	waitingCallers map[string]WaitingCaller // Uniqueid -> caller currently waiting in a queue

	clients       []*Client          // every attached AMI client (one per Asterisk server), for RefreshFromAMI
	agentClient   map[string]*Client // agent ext -> AMI client for the box it was last active on
	channelClient map[string]*Client // channel -> AMI client for the box that channel is on

	bridgeChannels   map[string]map[string]bool // BridgeUniqueid -> member channels currently in it
	channelBridge    map[string]string          // channel -> BridgeUniqueid it's currently in
	pendingReconnect map[string]pendingReconnect // agent ext -> caller channel waiting to be picked back up
	deliberateIntent map[string]time.Time        // agent ext -> best-effort "next hangup is deliberate" flag expiry
}

func NewMonitor() *Monitor {
	return &Monitor{
		agents:           make(map[string]AgentState),
		calls:            make(map[string]LiveCall),
		queues:           make(map[string]QueueStats),
		waitingCallers:   make(map[string]WaitingCaller),
		agentClient:      make(map[string]*Client),
		channelClient:    make(map[string]*Client),
		bridgeChannels:   make(map[string]map[string]bool),
		channelBridge:    make(map[string]string),
		pendingReconnect: make(map[string]pendingReconnect),
		deliberateIntent: make(map[string]time.Time),
	}
}

// Attach wires up an AMI client as one of this monitor's event sources —
// call once per configured Asterisk server (plus the single-box fallback).
// Multiple servers' events are merged into one shared snapshot; agentClient/
// channelClient track which source each agent/channel was last seen on, so
// actions (pause, hangup, redirect) can be dispatched back to the right box.
func (m *Monitor) Attach(client *Client) {
	m.mu.Lock()
	m.clients = append(m.clients, client)
	m.mu.Unlock()
	client.OnEvent(func(e Event) { m.handleEvent(client, e) })
}

// ClientForAgent returns the AMI client for the Asterisk box an agent
// extension was last active on, or nil if it's never been seen in an event.
func (m *Monitor) ClientForAgent(ext string) *Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.agentClient[ext]
}

// ClientForChannel returns the AMI client for the Asterisk box a channel is
// known to be on, or nil if unknown.
func (m *Monitor) ClientForChannel(channel string) *Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.channelClient[channel]
}

// setAgent stores an agent's updated state and records which client its
// triggering event came from. Must be called with m.mu held.
func (m *Monitor) setAgent(ext string, a AgentState, client *Client) {
	m.agents[ext] = a
	if client != nil {
		m.agentClient[ext] = client
	}
}

// ClaimPendingReconnect returns (and consumes) the channel of a caller still
// held on Asterisk waiting for ext to come back, if there is one and it
// hasn't expired (the caller's own dialplan Wait() timed out and hung them
// up already).
func (m *Monitor) ClaimPendingReconnect(ext string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.pendingReconnect[ext]
	if !ok {
		return "", false
	}
	delete(m.pendingReconnect, ext)
	if time.Now().After(p.expiresAt) {
		return "", false
	}
	return p.callerChannel, true
}

// MarkDeliberateHangup flags that ext is about to deliberately end their
// current call (see store/phone.js hangup()) — so the caller gets dropped
// immediately instead of held for the reconnect grace window. This races
// against the AMI BridgeLeave the SIP BYE triggers, so it handles both
// orderings: if BridgeLeave already stashed a pendingReconnect for ext by
// the time this arrives, drop that caller right now; otherwise leave a flag
// for BridgeLeave to see when it does arrive. If this signal is lost or too
// slow entirely, ClaimPendingReconnect's TTL / the dialplan's own Wait()
// timeout is the fallback — the caller just sits through the short window.
func (m *Monitor) MarkDeliberateHangup(ext string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.pendingReconnect[ext]; ok {
		delete(m.pendingReconnect, ext)
		if time.Now().Before(p.expiresAt) {
			if c := m.channelClient[p.callerChannel]; c != nil {
				c.HangupChannel(p.callerChannel)
			}
		}
		return
	}
	m.deliberateIntent[ext] = time.Now().Add(deliberateIntentTTL)
}

func (m *Monitor) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	today := now.Format("2006-01-02")

	// Update call durations
	calls := make(map[string]LiveCall, len(m.calls))
	for k, c := range m.calls {
		if c.StartTime > 0 {
			c.Duration = int(now.Unix() - c.StartTime)
		}
		calls[k] = c
	}

	agents := make(map[string]AgentState, len(m.agents))
	for k, v := range m.agents {
		if v.Channel != "" {
			if call, ok := calls[v.Channel]; ok {
				v.CallerNumber = call.Src
				v.CalleeNumber = call.Dst
				v.CallDuration = call.Duration
			}
		}
		// Both "today" stats are reset lazily here for display — the
		// underlying stored value only actually resets next time an event
		// touches it (see QueueMemberPaused / markFirstOnline), which is
		// fine since this is purely cosmetic for a day-old value no client
		// should still be showing.
		if v.firstOnlineDate != today {
			v.FirstOnlineToday = ""
		}
		pausedSecs := v.PausedTodaySeconds
		if v.pausedDate != today {
			pausedSecs = 0
		}
		if v.Status == "paused" && !v.pauseStartedAt.IsZero() {
			pausedSecs += int(now.Sub(v.pauseStartedAt).Seconds())
		}
		v.PausedTodaySeconds = pausedSecs
		agents[k] = v
	}
	queues := make(map[string]QueueStats, len(m.queues))
	for k, v := range m.queues {
		queues[k] = v
	}

	waitingCallers := make([]WaitingCaller, 0, len(m.waitingCallers))
	for _, wc := range m.waitingCallers {
		if wc.JoinedAt > 0 {
			wc.WaitSeconds = int(now.Unix() - wc.JoinedAt)
		}
		waitingCallers = append(waitingCallers, wc)
	}

	return Snapshot{
		Type:           "snapshot",
		Agents:         agents,
		Calls:          calls,
		Queues:         queues,
		WaitingCallers: waitingCallers,
	}
}

// SnapshotForTenant restricts a Snapshot to the given extensions and queue
// names — used to scope the dashboard's live view to one tenant. exts == nil
// means unfiltered (SuperAdmin, who isn't attached to any single tenant).
func (m *Monitor) SnapshotForTenant(exts map[string]bool, queueNames map[string]bool) Snapshot {
	full := m.Snapshot()
	if exts == nil {
		return full
	}

	agents := make(map[string]AgentState, len(exts))
	for k, v := range full.Agents {
		if exts[k] {
			agents[k] = v
		}
	}

	// Derived from the filtered agents' own channels rather than filtering
	// m.calls directly — calls are keyed by arbitrary channel names, not
	// extensions, so there's no other reliable way to tell which ones
	// belong to this tenant.
	calls := make(map[string]LiveCall)
	for _, a := range agents {
		if a.Channel == "" {
			continue
		}
		if c, ok := full.Calls[a.Channel]; ok {
			calls[a.Channel] = c
		}
	}

	queues := make(map[string]QueueStats, len(queueNames))
	for k, v := range full.Queues {
		if queueNames[k] {
			queues[k] = v
		}
	}

	waitingCallers := make([]WaitingCaller, 0)
	for _, wc := range full.WaitingCallers {
		if queueNames[wc.Queue] {
			waitingCallers = append(waitingCallers, wc)
		}
	}

	return Snapshot{Type: "snapshot", Agents: agents, Calls: calls, Queues: queues, WaitingCallers: waitingCallers}
}

// markFirstOnline records the first time today this agent was seen in any
// non-offline status. Resets automatically across a day boundary since it
// checks against the current date rather than a one-time flag.
func (m *Monitor) markFirstOnline(a *AgentState) {
	today := time.Now().Format("2006-01-02")
	if a.firstOnlineDate != today {
		a.firstOnlineDate = today
		a.FirstOnlineToday = time.Now().Format("15:04")
	}
}

// RefreshFromAMI pulls current state from every attached Asterisk server.
func (m *Monitor) RefreshFromAMI() {
	m.mu.RLock()
	clients := append([]*Client(nil), m.clients...)
	m.mu.RUnlock()
	for _, c := range clients {
		c.CoreShowChannels()
		c.QueueStatus()
	}
}

func (m *Monitor) handleEvent(client *Client, e Event) {
	evtType := e["Event"]
	if evtType == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if ch := e["Channel"]; ch != "" {
		m.channelClient[ch] = client
	}

	switch evtType {
	case "BridgeEnter":
		bridgeID := e["BridgeUniqueid"]
		channel := e["Channel"]
		if bridgeID == "" || channel == "" {
			return
		}
		if m.bridgeChannels[bridgeID] == nil {
			m.bridgeChannels[bridgeID] = map[string]bool{}
		}
		m.bridgeChannels[bridgeID][channel] = true
		m.channelBridge[channel] = bridgeID

		// The agent's own channel actually joining a bridge means the call
		// was answered — flip them from "ringing" to "busy" (talking).
		// Direction (in/out), set at Dial time, is left as-is so the UI can
		// still show whether this was an incoming or outgoing call.
		if ext := extractExtension(channel); ext != "" {
			a := m.getOrCreateAgent(ext)
			a.Status = "busy"
			m.setAgent(ext, a, client)
		}

	case "BridgeLeave":
		bridgeID := e["BridgeUniqueid"]
		channel := e["Channel"]
		if bridgeID == "" || channel == "" {
			return
		}
		members := m.bridgeChannels[bridgeID]
		var partner string
		if len(members) == 2 {
			for ch := range members {
				if ch != channel {
					partner = ch
				}
			}
		}
		delete(members, channel)
		if len(members) == 0 {
			delete(m.bridgeChannels, bridgeID)
		}
		delete(m.channelBridge, channel)

		if ext := extractExtension(channel); ext != "" && partner != "" {
			if exp, ok := m.deliberateIntent[ext]; ok && time.Now().Before(exp) {
				// Agent hung up on purpose (see MarkDeliberateHangup) —
				// don't hold the caller, end their call now.
				delete(m.deliberateIntent, ext)
				if c := m.channelClient[partner]; c != nil {
					c.HangupChannel(partner)
				}
			} else {
				// Agent's leg vanished unexpectedly (e.g. a page reload
				// killed their WebRTC) — the caller is held briefly in the
				// dialplan (see writeKCDialplan) in case this agent's
				// browser reconnects within the grace window.
				m.pendingReconnect[ext] = pendingReconnect{
					callerChannel: partner,
					expiresAt:     time.Now().Add(pendingReconnectTTL),
				}
			}
		}

	case "DeviceStateChange":
		device := e["Device"] // e.g. PJSIP/1001
		state := e["State"]
		ext := extractExtension(device)
		if ext == "" {
			return
		}
		agent := m.getOrCreateAgent(ext)
		agent.Status = deviceStateToStatus(state)
		if agent.Status != "offline" {
			m.markFirstOnline(&agent)
		}
		m.setAgent(ext, agent, client)

	case "Hangup":
		channel := e["Channel"]
		// Remove from calls
		delete(m.calls, channel)
		delete(m.channelClient, channel)
		// A caller can hang up while still on hold in a queue — normally
		// caught by QueueCallerAbandon, but Uniqueid is a reliable enough key
		// to also clean this up here in case that event is missed.
		if uid := e["Uniqueid"]; uid != "" {
			delete(m.waitingCallers, uid)
		}
		// Mark agent available
		ext := extractExtension(channel)
		if ext != "" {
			a := m.getOrCreateAgent(ext)
			a.Status = "available"
			a.Direction = ""
			a.Channel = ""
			a.CallDuration = 0
			m.setAgent(ext, a, client)
		}

	case "Dial":
		if e["SubEvent"] != "Begin" {
			return
		}
		channel := e["Channel"]
		dest := e["Destination"]
		src := e["CallerIDNum"]
		dst := e["DestCallerIDNum"]
		if dst == "" {
			dst = e["Dialstring"]
		}
		call := LiveCall{
			Channel:   channel,
			Src:       src,
			Dst:       dst,
			StartTime: time.Now().Unix(),
		}
		m.calls[channel] = call

		// Agent placing an outbound call: their own extension is the
		// dialing channel (e.g. via the tenant context's Dial()).
		if outExt := extractExtension(channel); outExt != "" {
			a := m.getOrCreateAgent(outExt)
			a.Status = "ringing"
			a.Direction = "out"
			a.Channel = channel
			m.setAgent(outExt, a, client)
		}
		// Agent being rung — by a queue (ringall dials each member this
		// way) or a direct/internal call: their extension is the
		// destination channel.
		if inExt := extractExtension(dest); inExt != "" {
			a := m.getOrCreateAgent(inExt)
			a.Status = "ringing"
			a.Direction = "in"
			a.Channel = channel
			m.setAgent(inExt, a, client)
		}

	case "AgentLogin", "AgentConnect":
		ext := e["Agent"]
		if ext == "" {
			ext = extractExtension(e["Channel"])
		}
		if ext != "" {
			agent := m.getOrCreateAgent(ext)
			agent.Status = "available"
			m.markFirstOnline(&agent)
			m.setAgent(ext, agent, client)
			log.Printf("[Monitor] agent %s logged in", ext)
		}

	case "AgentLogoff", "AgentComplete":
		ext := e["Agent"]
		if ext == "" {
			ext = extractExtension(e["Channel"])
		}
		if ext != "" {
			if a, ok := m.agents[ext]; ok {
				a.Status = "offline"
				m.setAgent(ext, a, client)
			}
		}

	case "QueueMemberPaused":
		ext := extractExtension(e["MemberName"])
		paused := e["Paused"] == "1"
		if ext != "" {
			a := m.getOrCreateAgent(ext)
			now := time.Now()
			today := now.Format("2006-01-02")
			if a.pausedDate != today {
				a.PausedTodaySeconds = 0
				a.pausedDate = today
			}
			if paused {
				a.Status = "paused"
				a.pauseStartedAt = now
			} else {
				if !a.pauseStartedAt.IsZero() {
					a.PausedTodaySeconds += int(now.Sub(a.pauseStartedAt).Seconds())
					a.pauseStartedAt = time.Time{}
				}
				a.Status = "available"
			}
			m.setAgent(ext, a, client)
		}

	case "QueueCallerJoin":
		queue := e["Queue"]
		q := m.queues[queue]
		q.Name = queue
		q.Waiting++
		m.queues[queue] = q

		if uid := e["Uniqueid"]; uid != "" {
			m.waitingCallers[uid] = WaitingCaller{
				Channel:  e["Channel"],
				Queue:    queue,
				CallerID: e["CallerIDNum"],
				JoinedAt: time.Now().Unix(),
			}
		}

	case "QueueCallerLeave", "QueueCallerAbandon":
		queue := e["Queue"]
		if q, ok := m.queues[queue]; ok {
			if q.Waiting > 0 {
				q.Waiting--
			}
			if evtType == "QueueCallerLeave" {
				q.Completed++
			}
			m.queues[queue] = q
		}
		if uid := e["Uniqueid"]; uid != "" {
			delete(m.waitingCallers, uid)
		}

	// QueueEntry is emitted once per currently-waiting caller in response to
	// QueueStatus (see RefreshFromAMI) — used to (re)seed waitingCallers on
	// startup/periodic refresh so a caller already waiting when this process
	// started, or a missed QueueCallerJoin, doesn't leave a gap. Wait is the
	// caller's already-elapsed hold time in seconds, per Asterisk; JoinedAt is
	// backdated by that much so the live counter keeps counting from the
	// right base instead of resetting to 0.
	case "QueueEntry":
		queue := e["Queue"]
		uid := e["Uniqueid"]
		if uid == "" {
			return
		}
		waited := 0
		if w := e["Wait"]; w != "" {
			if n, err := strconv.Atoi(w); err == nil {
				waited = n
			}
		}
		m.waitingCallers[uid] = WaitingCaller{
			Channel:  e["Channel"],
			Queue:    queue,
			CallerID: e["CallerIDNum"],
			JoinedAt: time.Now().Unix() - int64(waited),
		}

	case "QueueMemberStatus":
		queue := e["Queue"]
		q := m.queues[queue]
		q.Name = queue
		m.queues[queue] = q

	// CoreShowChannels response
	case "CoreShowChannel":
		channel := e["Channel"]
		src := e["CallerIDNum"]
		dst := e["ConnectedLineNum"]
		if channel != "" && src != "" {
			m.calls[channel] = LiveCall{
				Channel:   channel,
				Src:       src,
				Dst:       dst,
				StartTime: time.Now().Unix(),
			}
			ext := extractExtension(channel)
			if ext != "" {
				a := m.getOrCreateAgent(ext)
				a.Status = "busy"
				a.Channel = channel
				m.setAgent(ext, a, client)
			}
		}
	}
}

func (m *Monitor) getOrCreateAgent(ext string) AgentState {
	if a, ok := m.agents[ext]; ok {
		return a
	}
	return AgentState{SipNo: ext, Name: ext, Status: "offline"}
}

func extractExtension(channel string) string {
	// PJSIP/1001-00000001 → 1001
	// SIP/1001-00000001   → 1001
	for _, prefix := range []string{"PJSIP/", "SIP/"} {
		if strings.HasPrefix(channel, prefix) {
			s := strings.TrimPrefix(channel, prefix)
			if i := strings.Index(s, "-"); i > 0 {
				return s[:i]
			}
			return s
		}
	}
	return ""
}

func deviceStateToStatus(state string) string {
	switch strings.ToUpper(state) {
	case "NOT_INUSE":
		return "available"
	case "INUSE", "RINGING", "RINGINUSE":
		return "busy"
	case "ONHOLD":
		return "on_hold"
	case "UNAVAILABLE", "INVALID":
		return "offline"
	default:
		return "available"
	}
}
