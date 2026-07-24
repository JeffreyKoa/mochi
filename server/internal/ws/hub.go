package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"

	"github.com/mochi-ai/server/internal/models"
)

const pendingProactivePrefix = "mochi:proactive:pending:"

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Message struct {
	Type      string      `json:"type"`
	Data      interface{} `json:"data"`
	Timestamp int64       `json:"timestamp"`
}

type pendingItem struct {
	ReminderID uint64 `json:"reminder_id,omitempty"`
	Message    string `json:"message"`
	Animation  string `json:"animation"`
}

type Connection struct {
	UserID uint64
	Conn   *websocket.Conn
	Send   chan outboundMsg
}

type outboundMsg struct {
	payload []byte
	onSent  func()
}

type Hub struct {
	mu          sync.RWMutex
	connections map[uint64]*Connection
	rdb         *redis.Client
	onDelivered func(reminderID uint64)
}

func NewHub(rdb *redis.Client) *Hub {
	return &Hub{connections: make(map[uint64]*Connection), rdb: rdb}
}

func (h *Hub) SetReminderDeliveredHook(fn func(reminderID uint64)) {
	h.onDelivered = fn
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

func (h *Hub) SendProactive(userID uint64, message, animation string) bool {
	return h.sendProactive(userID, pendingItem{Message: message, Animation: animation})
}

func (h *Hub) SendProactiveReminder(userID uint64, reminderID uint64, message, animation string) bool {
	return h.sendProactive(userID, pendingItem{
		ReminderID: reminderID,
		Message:    message,
		Animation:  animation,
	})
}

func (h *Hub) sendProactive(userID uint64, item pendingItem) bool {
	msg := Message{
		Type: "proactive_message",
		Data: map[string]string{
			"message":   item.Message,
			"animation": item.Animation,
		},
		Timestamp: time.Now().Unix(),
	}
	var onSent func()
	if item.ReminderID > 0 && h.onDelivered != nil {
		id := item.ReminderID
		onSent = func() { h.onDelivered(id) }
	}
	if h.trySend(userID, msg, onSent) {
		return true
	}
	h.enqueuePending(userID, item)
	return false
}

func (h *Hub) SendLifeStageChanged(userID uint64, data map[string]interface{}) {
	h.send(userID, Message{
		Type:      "life_stage_changed",
		Data:      data,
		Timestamp: time.Now().Unix(),
	})
}

func (h *Hub) send(userID uint64, msg Message) {
	_ = h.trySend(userID, msg, nil)
}

func (h *Hub) trySend(userID uint64, msg Message, onSent func()) bool {
	data, err := json.Marshal(msg)
	if err != nil {
		return false
	}

	h.mu.RLock()
	conn, ok := h.connections[userID]
	h.mu.RUnlock()

	if !ok {
		return false
	}

	select {
	case conn.Send <- outboundMsg{payload: data, onSent: onSent}:
		return true
	default:
		log.Printf("[WS] send buffer full for user %d", userID)
		return false
	}
}

func (h *Hub) enqueuePending(userID uint64, item pendingItem) {
	if h.rdb == nil {
		log.Printf("[WS] proactive queued but redis unavailable user=%d", userID)
		return
	}
	ctx := context.Background()
	key := fmt.Sprintf("%s%d", pendingProactivePrefix, userID)
	payload, _ := json.Marshal(item)
	if err := h.rdb.RPush(ctx, key, payload).Err(); err != nil {
		log.Printf("[WS] enqueue pending failed user=%d: %v", userID, err)
		return
	}
	_ = h.rdb.Expire(ctx, key, 7*24*time.Hour)
	log.Printf("[WS] proactive queued user=%d reminder=%d", userID, item.ReminderID)
}

func (h *Hub) flushPending(userID uint64) {
	if h.rdb == nil {
		return
	}
	ctx := context.Background()
	key := fmt.Sprintf("%s%d", pendingProactivePrefix, userID)
	for {
		raw, err := h.rdb.LPop(ctx, key).Result()
		if err == redis.Nil {
			return
		}
		if err != nil {
			log.Printf("[WS] flush pending pop failed user=%d: %v", userID, err)
			return
		}
		var item pendingItem
		if json.Unmarshal([]byte(raw), &item) != nil || item.Message == "" {
			continue
		}
		if !h.sendProactive(userID, item) {
			_ = h.rdb.LPush(ctx, key, raw).Err()
			return
		}
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
		Send:   make(chan outboundMsg, 64),
	}

	h.mu.Lock()
	if old, ok := h.connections[userID]; ok {
		close(old.Send)
		old.Conn.Close()
	}
	h.connections[userID] = connection
	h.mu.Unlock()

	go h.flushPending(userID)
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
		case item, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.Conn.WriteMessage(websocket.TextMessage, item.payload); err != nil {
				return
			}
			if item.onSent != nil {
				item.onSent()
			}
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
