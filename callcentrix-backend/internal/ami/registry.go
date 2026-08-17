package ami

import "sync"

// Registry holds one AMI Client per Asterisk server (keyed by
// asterisk_servers.id), plus a fallback client used for users/actions with
// no server assigned — keeps single-box deployments working unchanged.
type Registry struct {
	mu       sync.RWMutex
	clients  map[int]*Client
	fallback *Client
}

func NewRegistry(fallback *Client) *Registry {
	return &Registry{clients: make(map[int]*Client), fallback: fallback}
}

// Set registers the AMI client for a given Asterisk server.
func (r *Registry) Set(serverID int, c *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[serverID] = c
}

// Remove drops a server's client from the registry (e.g. after deletion).
// The client's own ConnectWithRetry goroutine, if any, keeps running — there
// is no clean shutdown hook on Client — but it's harmless once nothing
// references it for dispatch anymore.
func (r *Registry) Remove(serverID int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.clients, serverID)
}

// Get resolves the client for a user's assigned server, falling back to the
// default single-server client when serverID is nil (unassigned/unknown).
func (r *Registry) Get(serverID *int) *Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if serverID != nil {
		if c, ok := r.clients[*serverID]; ok {
			return c
		}
	}
	return r.fallback
}

// All returns every distinct client in the registry (including the
// fallback) — used to broadcast reload actions that must reach every box
// sharing the same realtime config.
func (r *Registry) All() []*Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := make(map[*Client]bool)
	var all []*Client
	if r.fallback != nil {
		seen[r.fallback] = true
		all = append(all, r.fallback)
	}
	for _, c := range r.clients {
		if !seen[c] {
			seen[c] = true
			all = append(all, c)
		}
	}
	return all
}

// PJSIPReloadAll reloads res_pjsip on every configured Asterisk server —
// every box shares the same realtime ast_ps_* tables, so a change made
// against one needs to be picked up everywhere.
func (r *Registry) PJSIPReloadAll() {
	for _, c := range r.All() {
		c.PJSIPReload()
	}
}

// DialplanReloadAll reloads the dialplan on every configured Asterisk server.
func (r *Registry) DialplanReloadAll() {
	for _, c := range r.All() {
		c.DialplanReload()
	}
}
