package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/mochi-ai/server/internal/models"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Message struct {
	Type      string      `json:"type"`
	Data      interface{} `json:"data"`
	Timestamp int64       `json:"timestamp"`
}

type Connection struct {
	UserID uint64
	Conn   *websocket.Conn
	Send   chan []byte
}

type Hub struct {
	mu          sync.RWMutex
	connections map[uint64]*Connection
}

func NewHub() *Hub {
	return &Hub{connections: make(map[uint64]*Connection)}
}

func (h *Hub) BroadcastState(userID uint64, state models.LifeState, animation string) {
	h.send(userID, Message{
		Type: "state_update",
		Data: map[string]interface{}{
			"state":     state,
			"animation": animation,
		},
		Timestamp: time.Now().Unix(),
	})
}

func (h *Hub) SendProactive(userID uint64, message, animation string) {
	h.send(userID, Message{
		Type: "proactive_message",
		Data: map[string]string{
			"message":   message,
			"animation": animation,
		},
		Timestamp: time.Now().Unix(),
	})
}

func (h *Hub) send(userID uint64, msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	h.mu.RLock()
	conn, ok := h.connections[userID]
	h.mu.RUnlock()

	if !ok {
		return
	}

	select {
	case conn.Send <- data:
	default:
		log.Printf("[WS] send buffer full for user %d", userID)
	}
}

func (h *Hub) HandleWS(c *gin.Context, userID uint64) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	connection := &Connection{
		UserID: userID,
		Conn:   conn,
		Send:   make(chan []byte, 64),
	}

	h.mu.Lock()
	if old, ok := h.connections[userID]; ok {
		close(old.Send)
		old.Conn.Close()
	}
	h.connections[userID] = connection
	h.mu.Unlock()

	go h.writePump(connection)
	h.readPump(connection)
}

func (h *Hub) readPump(c *Connection) {
	defer func() {
		h.mu.Lock()
		if conn, ok := h.connections[c.UserID]; ok && conn == c {
			delete(h.connections, c.UserID)
		}
		h.mu.Unlock()
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(4096)
	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			break
		}

		var msg Message
		if json.Unmarshal(message, &msg) == nil && msg.Type == "heartbeat" {
			c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		}
	}
}

func (h *Hub) writePump(c *Connection) {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
