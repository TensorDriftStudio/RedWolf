package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tensordriftstudio/redwolf/internal/port"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow cross-origin dashboard connections
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// WSHandler manages real-time WebSocket event streaming.
type WSHandler struct {
	events port.EventBroadcaster
}

// NewWSHandler creates an initialized WSHandler.
func NewWSHandler(events port.EventBroadcaster) *WSHandler {
	return &WSHandler{events: events}
}

// ServeHTTP handles /ws/events connection upgrades.
func (h *WSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("failed to upgrade websocket connection", "error", err)
		return
	}
	defer conn.Close()

	ch, unsubscribe := h.events.Subscribe()
	defer unsubscribe()

	// Keep-alive heartbeat ticker
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Goroutine to consume client messages / detection of disconnect
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
