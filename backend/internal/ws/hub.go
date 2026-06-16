package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"ultrasound-annotation/internal/service"
)

const (
	writeWait   = 3 * time.Second
	pongWait    = 30 * time.Second
	pingPeriod  = (pongWait * 9) / 10
	maxMsgSize  = 1024 * 1024 * 4
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	ImageID  uint64
	UserID   string
	UserName string
	seq      int64
}

type Hub struct {
	collab service.CollaborationService

	imageClients map[uint64]map[*Client]struct{}
	mu           sync.RWMutex
	register     chan *Client
	unregister   chan *Client
	broadcast    chan BroadcastPayload
}

type BroadcastPayload struct {
	ImageID uint64
	Data    []byte
	Except  *Client
}

type incomingMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type ackMessage struct {
	Type      string `json:"type"`
	OK        bool   `json:"ok"`
	Seq       int64  `json:"seq,omitempty"`
	Message   string `json:"message,omitempty"`
	Timestamp int64  `json:"ts"`
}

func NewHub(collab service.CollaborationService) *Hub {
	h := &Hub{
		collab:       collab,
		imageClients: make(map[uint64]map[*Client]struct{}),
		register:     make(chan *Client, 64),
		unregister:   make(chan *Client, 64),
		broadcast:    make(chan BroadcastPayload, 256),
	}
	go h.run()
	return h
}

func (h *Hub) run() {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case c := <-h.register:
			h.mu.Lock()
			if _, ok := h.imageClients[c.ImageID]; !ok {
				h.imageClients[c.ImageID] = make(map[*Client]struct{})
				go h.bridgeRedis(c.ImageID)
			}
			h.imageClients[c.ImageID][c] = struct{}{}
			h.mu.Unlock()
			_ = h.collab.JoinImage(context.Background(), c.ImageID, service.CollabSession{
				UserID:   c.UserID,
				UserName: c.UserName,
			})
			h.broadcastPresence(c.ImageID)

		case c := <-h.unregister:
			h.mu.Lock()
			if m, ok := h.imageClients[c.ImageID]; ok {
				if _, exists := m[c]; exists {
					delete(m, c)
					close(c.send)
					if len(m) == 0 {
						delete(h.imageClients, c.ImageID)
					}
				}
			}
			h.mu.Unlock()
			_ = h.collab.LeaveImage(context.Background(), c.ImageID, c.UserID)
			h.broadcastPresence(c.ImageID)

		case bp := <-h.broadcast:
			h.mu.RLock()
			m := h.imageClients[bp.ImageID]
			for c := range m {
				if c == bp.Except {
					continue
				}
				select {
				case c.send <- bp.Data:
				default:
					go func(cl *Client) {
						h.unregister <- cl
					}(c)
				}
			}
			h.mu.RUnlock()

		case <-t.C:
			h.cleanup()
		}
	}
}

func (h *Hub) cleanup() {
	ctx := context.Background()
	h.mu.RLock()
	ids := make([]uint64, 0, len(h.imageClients))
	for id := range h.imageClients {
		ids = append(ids, id)
	}
	h.mu.RUnlock()
	for _, id := range ids {
		h.broadcastPresence(id)
		_ = h.collab.ListUsers(ctx, id)
	}
}

func (h *Hub) bridgeRedis(imageID uint64) {
	ctx := context.Background()
	ch := h.collab.SubscribeOps(ctx, imageID)
	for op := range ch {
		data, _ := json.Marshal(op)
		h.mu.RLock()
		m := h.imageClients[imageID]
		for c := range m {
			if c.UserID == op.UserID {
				continue
			}
			select {
			case c.send <- data:
			default:
			}
		}
		h.mu.RUnlock()
	}
}

func (h *Hub) broadcastPresence(imageID uint64) {
	ctx := context.Background()
	users, _ := h.collab.ListUsers(ctx, imageID)
	payload, _ := json.Marshal(service.OperationMessage{
		Type:      "presence",
		Timestamp: time.Now().UnixMilli(),
		Payload:   mustJSON(users),
	})
	h.mu.RLock()
	m := h.imageClients[imageID]
	for c := range m {
		select {
		case c.send <- payload:
		default:
		}
	}
	h.mu.RUnlock()
}

func mustJSON(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func (h *Hub) ServeWS(c *gin.Context) {
	imageID, err := parseUintParam(c, "imageId")
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid imageId"})
		return
	}
	userID := c.Query("userId")
	if userID == "" {
		userID = fmt.Sprintf("guest-%d", time.Now().UnixNano())
	}
	userName := c.Query("userName")
	if userName == "" {
		userName = userID
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("ws upgrade error: %v", err)
		return
	}
	conn.SetReadLimit(maxMsgSize)
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	client := &Client{
		hub:      h,
		conn:     conn,
		send:     make(chan []byte, 256),
		ImageID:  imageID,
		UserID:   userID,
		UserName: userName,
	}
	h.register <- client

	go client.writePump()
	go client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		_ = c.conn.Close()
	}()
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("ws read error: %v", err)
			}
			return
		}
		c.handleMessage(msg)
	}
}

func (c *Client) writePump() {
	t := time.NewTicker(pingPeriod)
	defer func() {
		t.Stop()
		_ = c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			_, _ = w.Write(msg)
			if err := w.Close(); err != nil {
				return
			}
		case <-t.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) handleMessage(raw []byte) {
	var m incomingMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		c.sendAck("error", false, 0, "bad message")
		return
	}
	seq := atomic.AddInt64(&c.seq, 1)
	ctx := context.Background()

	switch m.Type {
	case "ping":
		c.sendAck("pong", true, seq, "")
	case "heartbeat":
		var cursor modelsPoint
		_ = json.Unmarshal(m.Payload, &cursor)
		_ = c.hub.collab.Heartbeat(ctx, c.ImageID, service.CollabSession{
			UserID:    c.UserID,
			UserName:  c.UserName,
			CursorPos: &cursor,
		})
		c.sendAck("heartbeat", true, seq, "")
	case "cursor":
		op := service.OperationMessage{
			Type:     service.OpTypeCursor,
			UserID:   c.UserID,
			UserName: c.UserName,
			Sequence: seq,
			Payload:  m.Payload,
		}
		_ = c.hub.collab.PublishOp(ctx, c.ImageID, op)
		c.sendAck("cursor", true, seq, "")
	case "broadcast":
		var inner service.OperationMessage
		if err := json.Unmarshal(m.Payload, &inner); err != nil {
			c.sendAck("broadcast", false, seq, err.Error())
			return
		}
		inner.UserID = c.UserID
		inner.UserName = c.UserName
		inner.Sequence = seq
		_ = c.hub.collab.PublishOp(ctx, c.ImageID, inner)
		c.sendAck("broadcast", true, seq, "")
	default:
		c.sendAck("error", false, seq, "unknown type: "+m.Type)
	}
}

type modelsPoint = struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

func (c *Client) sendAck(kind string, ok bool, seq int64, msg string) {
	b, _ := json.Marshal(ackMessage{
		Type:      kind,
		OK:        ok,
		Seq:       seq,
		Message:   msg,
		Timestamp: time.Now().UnixMilli(),
	})
	select {
	case c.send <- b:
	default:
	}
}

func parseUintParam(c *gin.Context, name string) (uint64, error) {
	s := c.Param(name)
	var v uint64
	n, err := fmt.Sscanf(s, "%d", &v)
	if err != nil || n != 1 || v == 0 {
		return 0, errors.New("invalid uint")
	}
	return v, nil
}
