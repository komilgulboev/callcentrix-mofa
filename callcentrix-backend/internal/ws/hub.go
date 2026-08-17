package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
}

type Hub struct {
	clients    map[*client]bool
	mu         sync.RWMutex
	register   chan *client
	unregister chan *client
	broadcast  chan []byte
}

type client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*client]bool),
		register:   make(chan *client, 16),
		unregister: make(chan *client, 16),
		broadcast:  make(chan []byte, 64),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			h.mu.Lock()
			h.clients[c] = true
			h.mu.Unlock()
			log.Printf("[WS] client connected (total: %d)", len(h.clients))

		case c := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
			}
			h.mu.Unlock()
			log.Printf("[WS] client disconnected (total: %d)", len(h.clients))

		case msg := <-h.broadcast:
			h.mu.RLock()
			for c := range h.clients {
				select {
				case c.send <- msg:
				default:
					// Slow client — drop
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends any JSON-serialisable value to all connected clients
func (h *Hub) Broadcast(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	select {
	case h.broadcast <- b:
	default:
	}
}

// ServeWS upgrades the connection and registers the client
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] upgrade error: %v", err)
		return
	}
	c := &client{hub: h, conn: conn, send: make(chan []byte, 64)}
	h.register <- c

	go c.writePump()
	go c.readPump()
}

func (c *client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
		c.hub.unregister <- c
	}()
	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *client) readPump() {
	defer c.conn.Close()
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}
