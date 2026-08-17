package handlers

import (
	"crypto/tls"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	mw "callcentrix/internal/middleware"
)

var phoneWSUpgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	Subprotocols:    []string{"sip"},
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
}

// syncConn pairs a websocket.Conn with a mutex so every writer (the relay
// loop and the ping handler below) serializes through the same lock —
// gorilla/websocket only allows one concurrent writer per connection.
type syncConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *syncConn) writeMessage(mt int, msg []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteMessage(mt, msg)
}

func (c *syncConn) installPingHandler() {
	c.conn.SetPingHandler(func(appData string) error {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(5*time.Second))
	})
}

// ServeWS proxies a browser WebSocket connection through to the internal
// Asterisk WSS endpoint (AsteriskWSURI), so browsers only ever need to reach
// the app server — never the internal Asterisk address or its self-signed
// certificate directly.
func (h *PhoneHandler) ServeWS(w http.ResponseWriter, r *http.Request) {
	targetWSURI := h.AsteriskWSURI
	if c := mw.GetClaims(r); c != nil {
		targetWSURI = h.resolveAsteriskWSURI(r.Context(), c.Sub)
	}

	clientWS, err := phoneWSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[phone ws] upgrade error: %v", err)
		return
	}
	defer clientWS.Close()

	dialer := websocket.Dialer{
		Subprotocols:    []string{"sip"},
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	upstreamWS, _, err := dialer.Dial(targetWSURI, nil)
	if err != nil {
		log.Printf("[phone ws] upstream dial error: %v", err)
		return
	}
	defer upstreamWS.Close()

	client := &syncConn{conn: clientWS}
	upstream := &syncConn{conn: upstreamWS}
	client.installPingHandler()
	upstream.installPingHandler()

	errc := make(chan error, 2)
	relay := func(dst *syncConn, src *syncConn) {
		for {
			mt, msg, err := src.conn.ReadMessage()
			if err != nil {
				errc <- err
				return
			}
			if err := dst.writeMessage(mt, msg); err != nil {
				errc <- err
				return
			}
		}
	}
	go relay(upstream, client)
	go relay(client, upstream)
	<-errc
}
