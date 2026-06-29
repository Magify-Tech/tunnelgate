package realtime

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/websocket"
)

type Event struct {
	Event   string `json:"event"`
	Payload any    `json:"payload"`
}

type Hub struct {
	mu      sync.RWMutex
	clients map[*client]struct{}
	buffer  int
}

type client struct {
	send chan Event
}

func NewHub() *Hub {
	return &Hub{
		clients: map[*client]struct{}{},
		buffer:  32,
	}
}

func (h *Hub) Register(router *gin.RouterGroup) {
	server := websocket.Server{
		Handler:   h.serve,
		Handshake: func(*websocket.Config, *http.Request) error { return nil },
	}
	router.GET("/ws", func(c *gin.Context) {
		server.ServeHTTP(c.Writer, c.Request)
	})
}

func (h *Hub) Broadcast(event string, payload any) {
	message := Event{Event: event, Payload: payload}

	h.mu.RLock()
	clients := make([]*client, 0, len(h.clients))
	for client := range h.clients {
		clients = append(clients, client)
	}
	h.mu.RUnlock()

	for _, client := range clients {
		select {
		case client.send <- message:
		default:
			h.unsubscribe(client)
		}
	}
}

func (h *Hub) serve(ws *websocket.Conn) {
	client := &client{send: make(chan Event, h.buffer)}
	h.subscribe(client)
	defer h.unsubscribe(client)

	done := make(chan struct{})
	go func() {
		var ignored any
		for {
			if err := websocket.JSON.Receive(ws, &ignored); err != nil {
				close(done)
				return
			}
		}
	}()

	for {
		select {
		case message, ok := <-client.send:
			if !ok {
				return
			}
			if err := websocket.JSON.Send(ws, message); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}

func (h *Hub) subscribe(client *client) {
	h.mu.Lock()
	h.clients[client] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) unsubscribe(client *client) {
	h.mu.Lock()
	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.send)
	}
	h.mu.Unlock()
}
