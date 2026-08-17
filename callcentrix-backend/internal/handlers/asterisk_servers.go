package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"callcentrix/internal/ami"
	"callcentrix/internal/asterisk"
)

// AsteriskServersHandler manages the registry of physical Asterisk servers
// for a multi-box deployment sharing one Postgres realtime schema. SuperAdmin
// only. See ASTERISK_CLUSTER_SETUP.md for the one-time Asterisk-side setup
// this depends on.
type AsteriskServersHandler struct {
	DB      *sql.DB
	AMI     *ami.Registry
	Monitor *ami.Monitor
}

func decodeServerBody(r *http.Request) (asterisk.AsteriskServer, error) {
	var s asterisk.AsteriskServer
	err := decode(r, &s)
	return s, err
}

// List returns every configured Asterisk server.
func (h *AsteriskServersHandler) List(w http.ResponseWriter, r *http.Request) {
	servers, err := asterisk.ListServers(h.DB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": servers})
}

// Create adds a new Asterisk server: writes the row + inter-server trust
// mesh, starts an AMI connection to it, and attaches that connection to the
// live monitor and dispatch registry so it's immediately usable.
func (h *AsteriskServersHandler) Create(w http.ResponseWriter, r *http.Request) {
	s, err := decodeServerBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	id, err := asterisk.CreateServer(h.DB, s)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	client := ami.NewClient(s.AMIHost, s.AMIUser, s.AMIPass)
	go client.ConnectWithRetry()
	h.Monitor.Attach(client)
	h.AMI.Set(id, client)

	h.AMI.PJSIPReloadAll()
	h.AMI.DialplanReloadAll()
	writeJSON(w, http.StatusCreated, map[string]int{"id": id})
}

// Update changes a server's settings, re-derives the trust mesh, and
// reconnects AMI with the (possibly new) credentials.
func (h *AsteriskServersHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	s, err := decodeServerBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	s.ID = id

	if err := asterisk.UpdateServer(h.DB, s); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	client := ami.NewClient(s.AMIHost, s.AMIUser, s.AMIPass)
	go client.ConnectWithRetry()
	h.Monitor.Attach(client)
	h.AMI.Set(id, client)

	h.AMI.PJSIPReloadAll()
	h.AMI.DialplanReloadAll()
	w.WriteHeader(http.StatusNoContent)
}

// Delete removes an Asterisk server (refuses while users are still assigned
// to it) and re-derives the trust mesh for the rest.
func (h *AsteriskServersHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if err := asterisk.DeleteServer(h.DB, id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.AMI.Remove(id)
	h.AMI.PJSIPReloadAll()
	h.AMI.DialplanReloadAll()
	w.WriteHeader(http.StatusNoContent)
}
