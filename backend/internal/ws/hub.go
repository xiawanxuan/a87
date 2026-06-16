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
	writeWait        = 5 * time.Second
	pongWait         = 30 * time.Second
	pingPeriod       = (pongWait * 9) / 10
	maxMsgSize       = 1024 * 1024 * 4
	sendBufferSize   = 2048
	historySize      = 1000
	ackTimeout       = 3 * time.Second
	maxRetries       = 3
	retryInterval    = 500 * time.Millisecond
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  16 * 1024,
	WriteBufferSize: 16 * 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type MessagePriority int

const (
	PriorityLow    MessagePriority = iota // cursor
	PriorityMedium                        // presence
	PriorityHigh                          // add/update/delete/rollback/bulk_replace
)

type queuedMessage struct {
	data      []byte
	priority  MessagePriority
	seq       int64
	timestamp int64
}

type Client struct {
	hub                 *Hub
	conn                *websocket.Conn
	send                chan queuedMessage
	ImageID             uint64
	UserID              string
	UserName            string
	seq                 int64
	lastAckSeq          int64
	lastProcessedSeq    int64
	joinedAt            int64
	pendingAcks         map[int64]pendingAck
	pendingAcksMu       sync.Mutex
	slowClient          bool
	dropCount           atomic.Int64
	recvCount           atomic.Int64
	sendCount           atomic.Int64
}

type pendingAck struct {
	msg       []byte
	seq       int64
	retries   int
	createdAt int64
	timer     *time.Timer
}

type Hub struct {
	collab service.CollaborationService

	imageClients map[uint64]map[*Client]struct{}
	mu           sync.RWMutex
	register     chan *Client
	unregister   chan *Client
	broadcast    chan BroadcastPayload
	shutdown     chan struct{}

	globalSeq    atomic.Int64
	messageCache *ringBuffer
	cacheMu      sync.RWMutex

	metrics *HubMetrics
}

type HubMetrics struct {
	broadcastTotal   atomic.Int64
	broadcastDropped atomic.Int64
	ackRetries       atomic.Int64
	activeClients    atomic.Int64
}

type BroadcastPayload struct {
	ImageID  uint64
	Data     []byte
	Except   *Client
	Priority MessagePriority
}

type incomingMessage struct {
	Type              string          `json:"type"`
	Payload           json.RawMessage `json:"payload,omitempty"`
	Seq               int64           `json:"seq,omitempty"`
	LastProcessedSeq  int64           `json:"lastProcessedSeq,omitempty"`
}

type ackMessage struct {
	Type      string `json:"type"`
	OK        bool   `json:"ok"`
	Seq       int64  `json:"seq,omitempty"`
	Message   string `json:"message,omitempty"`
	Timestamp int64  `json:"ts"`
}

type syncMessage struct {
	Type           string `json:"type"`
	StartSeq       int64  `json:"startSeq"`
	EndSeq         int64  `json:"endSeq"`
	MessageCount   int    `json:"messageCount"`
	ClientJoinedAt int64  `json:"clientJoinedAt"`
}

type ringBuffer struct {
	buf   []cachedMessage
	start int
	end   int
	size  int
	mu    sync.Mutex
}

type cachedMessage struct {
	seq       int64
	data      []byte
	priority  MessagePriority
	timestamp int64
	imageID   uint64
}

func newRingBuffer(size int) *ringBuffer {
	return &ringBuffer{
		buf:   make([]cachedMessage, size),
		size:  size,
		start: 0,
		end:   0,
	}
}

func (rb *ringBuffer) add(m cachedMessage) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.buf[rb.end] = m
	rb.end = (rb.end + 1) % rb.size
	if rb.end == rb.start {
		rb.start = (rb.start + 1) % rb.size
	}
}

func (rb *ringBuffer) getFromSeq(startSeq int64, imageID uint64) []cachedMessage {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	var result []cachedMessage
	i := rb.start
	for i != rb.end {
		m := rb.buf[i]
		if m.seq >= startSeq && m.imageID == imageID {
			result = append(result, m)
		}
		i = (i + 1) % rb.size
	}
	return result
}

func (rb *ringBuffer) latestSeq() int64 {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	lastIdx := (rb.end - 1 + rb.size) % rb.size
	if lastIdx == rb.end && rb.start == rb.end {
		return 0
	}
	return rb.buf[lastIdx].seq
}

func NewHub(collab service.CollaborationService) *Hub {
	h := &Hub{
		collab:       collab,
		imageClients: make(map[uint64]map[*Client]struct{}),
		register:     make(chan *Client, 64),
		unregister:   make(chan *Client, 64),
		broadcast:    make(chan BroadcastPayload, 1024),
		shutdown:     make(chan struct{}),
		messageCache: newRingBuffer(historySize),
		metrics:      &HubMetrics{},
	}
	go h.run()
	go h.metricsReporter()
	return h
}

func (h *Hub) Shutdown() {
	close(h.shutdown)
}

func (h *Hub) metricsReporter() {
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			log.Printf("[hub-metrics] active=%d broadcast=%d dropped=%d retries=%d",
				h.metrics.activeClients.Load(),
				h.metrics.broadcastTotal.Load(),
				h.metrics.broadcastDropped.Load(),
				h.metrics.ackRetries.Load(),
			)
		case <-h.shutdown:
			return
		}
	}
}

func (h *Hub) nextSeq() int64 {
	return h.globalSeq.Add(1)
}

func (h *Hub) Broadcast(imageID uint64, data []byte, priority int) {
	p := MessagePriority(priority)
	h.broadcast <- BroadcastPayload{
		ImageID:  imageID,
		Data:     data,
		Priority: p,
	}
}

func (h *Hub) cacheMessage(imageID uint64, data []byte, priority MessagePriority, seq int64) {
	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()
	h.messageCache.add(cachedMessage{
		seq:       seq,
		data:      data,
		priority:  priority,
		timestamp: time.Now().UnixMilli(),
		imageID:   imageID,
	})
}

func (h *Hub) run() {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case c := <-h.register:
			h.handleRegister(c)
		case c := <-h.unregister:
			h.handleUnregister(c)
		case bp := <-h.broadcast:
			h.handleBroadcast(bp)
		case <-t.C:
			h.cleanup()
		case <-h.shutdown:
			return
		}
	}
}

func (h *Hub) handleRegister(c *Client) {
	h.mu.Lock()
	if _, ok := h.imageClients[c.ImageID]; !ok {
		h.imageClients[c.ImageID] = make(map[*Client]struct{})
		go h.bridgeRedis(c.ImageID)
	}
	h.imageClients[c.ImageID][c] = struct{}{}
	h.mu.Unlock()
	h.metrics.activeClients.Add(1)

	_ = h.collab.JoinImage(context.Background(), c.ImageID, service.CollabSession{
		UserID:   c.UserID,
		UserName: c.UserName,
	})

	go func() {
		time.Sleep(100 * time.Millisecond)
		h.sendSyncToClient(c)
		h.broadcastPresence(c.ImageID)
	}()
}

func (h *Hub) handleUnregister(c *Client) {
	c.pendingAcksMu.Lock()
	for _, pa := range c.pendingAcks {
		if pa.timer != nil {
			pa.timer.Stop()
		}
	}
	c.pendingAcks = nil
	c.pendingAcksMu.Unlock()

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
	h.metrics.activeClients.Add(-1)

	_ = h.collab.LeaveImage(context.Background(), c.ImageID, c.UserID)
	h.broadcastPresence(c.ImageID)
}

func (h *Hub) handleBroadcast(bp BroadcastPayload) {
	seq := h.nextSeq()
	h.cacheMessage(bp.ImageID, bp.Data, bp.Priority, seq)

	envelope := map[string]interface{}{
		"seq":       seq,
		"timestamp": time.Now().UnixMilli(),
	}
	var rawMsg json.RawMessage
	_ = json.Unmarshal(bp.Data, &rawMsg)
	envelope["payload"] = rawMsg

	var opType string
	var temp map[string]interface{}
	if json.Unmarshal(bp.Data, &temp) == nil {
		if t, ok := temp["type"].(string); ok {
			opType = t
			envelope["type"] = t
		}
		if u, ok := temp["userId"].(string); ok {
			envelope["userId"] = u
		}
		if un, ok := temp["userName"].(string); ok {
			envelope["userName"] = un
		}
	}

	envelopeData, _ := json.Marshal(envelope)
	qm := queuedMessage{
		data:      envelopeData,
		priority:  bp.Priority,
		seq:       seq,
		timestamp: time.Now().UnixMilli(),
	}

	h.mu.RLock()
	m := h.imageClients[bp.ImageID]
	clients := make([]*Client, 0, len(m))
	for c := range m {
		if c != bp.Except {
			clients = append(clients, c)
		}
	}
	h.mu.RUnlock()

	h.metrics.broadcastTotal.Add(int64(len(clients)))

	for _, c := range clients {
		if !c.enqueueMessage(qm) {
			h.metrics.broadcastDropped.Add(1)
			if c.dropCount.Add(1)%10 == 0 {
				log.Printf("[hub] slow client %s on image %d: dropped %d messages",
					c.UserID, c.ImageID, c.dropCount.Load())
			}
		}
	}
}

func (h *Hub) sendSyncToClient(c *Client) {
	h.cacheMu.RLock()
	missing := h.messageCache.getFromSeq(c.lastProcessedSeq+1, c.ImageID)
	currentSeq := h.messageCache.latestSeq()
	h.cacheMu.RUnlock()

	syncMsg, _ := json.Marshal(syncMessage{
		Type:           "sync",
		StartSeq:       c.lastProcessedSeq + 1,
		EndSeq:         currentSeq,
		MessageCount:   len(missing),
		ClientJoinedAt: c.joinedAt,
	})

	envelope, _ := json.Marshal(map[string]interface{}{
		"seq":       0,
		"type":      "sync",
		"timestamp": time.Now().UnixMilli(),
		"payload":   json.RawMessage(syncMsg),
	})

	c.enqueueMessage(queuedMessage{
		data:      envelope,
		priority:  PriorityHigh,
		seq:       0,
		timestamp: time.Now().UnixMilli(),
	})

	for _, m := range missing {
		qm := queuedMessage{
			data:      m.data,
			priority:  m.priority,
			seq:       m.seq,
			timestamp: m.timestamp,
		}
		if !c.enqueueMessage(qm) {
			h.metrics.broadcastDropped.Add(1)
		}
	}

	if len(missing) > 0 {
		log.Printf("[hub] sync client %s: replayed %d messages (seq %d->%d)",
			c.UserID, len(missing), c.lastProcessedSeq+1, currentSeq)
	}
}

func (h *Hub) bridgeRedis(imageID uint64) {
	ctx := context.Background()
	ch := h.collab.SubscribeOps(ctx, imageID)
	for {
		select {
		case op, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(op)
			priority := PriorityLow
			switch op.Type {
			case service.OpTypeAdd, service.OpTypeUpdate, service.OpTypeDelete,
				service.OpTypeBulkReplace, service.OpTypeRollback:
				priority = PriorityHigh
			case "presence":
				priority = PriorityMedium
			}

			var userID string
			var temp map[string]interface{}
			if json.Unmarshal(op.Payload, &temp) == nil {
				if u, ok := temp["userId"].(string); ok {
					userID = u
				}
			}

			h.mu.RLock()
			m := h.imageClients[imageID]
			clients := make([]*Client, 0, len(m))
			for c := range m {
				if c.UserID != op.UserID && c.UserID != userID {
					clients = append(clients, c)
				}
			}
			h.mu.RUnlock()

			seq := h.nextSeq()
			h.cacheMessage(imageID, data, priority, seq)

			envelope := map[string]interface{}{
				"seq":       seq,
				"timestamp": time.Now().UnixMilli(),
				"type":      op.Type,
				"userId":    op.UserID,
				"userName":  op.UserName,
				"payload":   op.Payload,
			}
			envelopeData, _ := json.Marshal(envelope)
			qm := queuedMessage{
				data:      envelopeData,
				priority:  priority,
				seq:       seq,
				timestamp: time.Now().UnixMilli(),
			}

			h.metrics.broadcastTotal.Add(int64(len(clients)))
			for _, c := range clients {
				if !c.enqueueMessage(qm) {
					h.metrics.broadcastDropped.Add(1)
				}
			}

		case <-h.shutdown:
			return
		}
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

	var except *Client
	h.mu.RLock()
	m := h.imageClients[imageID]
	clients := make([]*Client, 0, len(m))
	for c := range m {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	seq := h.nextSeq()
	h.cacheMessage(imageID, payload, PriorityMedium, seq)

	envelope := map[string]interface{}{
		"seq":       seq,
		"type":      "presence",
		"timestamp": time.Now().UnixMilli(),
		"payload":   json.RawMessage(mustJSON(users)),
	}
	envelopeData, _ := json.Marshal(envelope)
	qm := queuedMessage{
		data:      envelopeData,
		priority:  PriorityMedium,
		seq:       seq,
		timestamp: time.Now().UnixMilli(),
	}

	_ = except
	for _, c := range clients {
		c.enqueueMessage(qm)
	}
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
	lastSeq := int64(0)
	if ls := c.Query("lastSeq"); ls != "" {
		fmt.Sscanf(ls, "%d", &lastSeq)
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
		hub:              h,
		conn:             conn,
		send:             make(chan queuedMessage, sendBufferSize),
		ImageID:          imageID,
		UserID:           userID,
		UserName:         userName,
		lastProcessedSeq: lastSeq,
		joinedAt:         time.Now().UnixMilli(),
		pendingAcks:      make(map[int64]pendingAck),
	}
	h.register <- client

	go client.writePump()
	go client.readPump()

	log.Printf("[ws] client %s connected to image %d (lastSeq=%d)", userID, imageID, lastSeq)
}

func (c *Client) enqueueMessage(qm queuedMessage) bool {
	if qm.priority == PriorityLow && len(c.send) >= cap(c.send)*95/100 {
		c.dropCount.Add(1)
		return false
	}

	select {
	case c.send <- qm:
		c.sendCount.Add(1)
		if qm.priority == PriorityHigh && qm.seq > 0 {
			c.scheduleAckRetry(qm)
		}
		return true
	default:
		if qm.priority == PriorityHigh {
			if len(c.send) > 0 {
				dropped := <-c.send
				if dropped.priority < PriorityHigh {
					c.send <- qm
					c.dropCount.Add(1)
					return true
				} else {
					c.send <- dropped
				}
			}
		}
		c.dropCount.Add(1)
		return false
	}
}

func (c *Client) scheduleAckRetry(qm queuedMessage) {
	c.pendingAcksMu.Lock()
	defer c.pendingAcksMu.Unlock()
	if c.pendingAcks == nil {
		return
	}

	pa := pendingAck{
		msg:       qm.data,
		seq:       qm.seq,
		retries:   0,
		createdAt: time.Now().UnixMilli(),
	}
	pa.timer = time.AfterFunc(ackTimeout, func() {
		c.retryAck(pa.seq)
	})
	c.pendingAcks[qm.seq] = pa
}

func (c *Client) retryAck(seq int64) {
	c.pendingAcksMu.Lock()
	pa, ok := c.pendingAcks[seq]
	if !ok {
		c.pendingAcksMu.Unlock()
		return
	}

	if pa.retries >= maxRetries {
		delete(c.pendingAcks, seq)
		c.pendingAcksMu.Unlock()
		log.Printf("[ws] client %s: message %d failed after %d retries", c.UserID, seq, maxRetries)
		return
	}

	pa.retries++
	c.hub.metrics.ackRetries.Add(1)

	qm := queuedMessage{
		data:      pa.msg,
		priority:  PriorityHigh,
		seq:       seq,
		timestamp: time.Now().UnixMilli(),
	}

	select {
	case c.send <- qm:
		c.sendCount.Add(1)
		pa.timer = time.AfterFunc(ackTimeout, func() {
			c.retryAck(seq)
		})
		c.pendingAcks[seq] = pa
		c.pendingAcksMu.Unlock()
		log.Printf("[ws] client %s: retry message %d (attempt %d)", c.UserID, seq, pa.retries)
	default:
		delete(c.pendingAcks, seq)
		c.pendingAcksMu.Unlock()
		log.Printf("[ws] client %s: retry buffer full, dropping seq %d", c.UserID, seq)
	}
}

func (c *Client) handleAck(seq int64) {
	c.pendingAcksMu.Lock()
	defer c.pendingAcksMu.Unlock()
	if pa, ok := c.pendingAcks[seq]; ok {
		if pa.timer != nil {
			pa.timer.Stop()
		}
		delete(c.pendingAcks, seq)
	}
	if seq > c.lastAckSeq {
		c.lastAckSeq = seq
	}
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		_ = c.conn.Close()
		log.Printf("[ws] client %s disconnected: recv=%d send=%d dropped=%d",
			c.UserID, c.recvCount.Load(), c.sendCount.Load(), c.dropCount.Load())
	}()
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("ws read error for %s: %v", c.UserID, err)
			}
			return
		}
		c.recvCount.Add(1)
		c.handleMessage(msg)
	}
}

func (c *Client) writePump() {
	t := time.NewTicker(pingPeriod)
	defer func() {
		t.Stop()
		_ = c.conn.Close()
	}()

	type deadline struct {
		messages []queuedMessage
	}
	highBuffer := make([]queuedMessage, 0, 64)
	mediumBuffer := make([]queuedMessage, 0, 64)
	lowBuffer := make([]queuedMessage, 0, 128)

	flush := func(buf *[]queuedMessage) {
		for len(*buf) > 0 {
			qm := (*buf)[0]
			*buf = (*buf)[1:]
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			_, _ = w.Write(qm.data)
			if err := w.Close(); err != nil {
				return
			}
		}
	}

	for {
		select {
		case qm, ok := <-c.send:
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			switch qm.priority {
			case PriorityHigh:
				highBuffer = append(highBuffer, qm)
			case PriorityMedium:
				mediumBuffer = append(mediumBuffer, qm)
			default:
				lowBuffer = append(lowBuffer, qm)
			}

			if len(c.send) == 0 {
				flush(&highBuffer)
				flush(&mediumBuffer)
				flush(&lowBuffer)
			} else if len(highBuffer) >= 32 {
				flush(&highBuffer)
			}

		case <-t.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
			flush(&highBuffer)
			flush(&mediumBuffer)
			flush(&lowBuffer)
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

	if m.LastProcessedSeq > c.lastProcessedSeq {
		c.lastProcessedSeq = m.LastProcessedSeq
	}

	switch m.Type {
	case "ping":
		c.sendAck("pong", true, seq, "")
	case "ack":
		c.handleAck(m.Seq)
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
	case "resync":
		go c.hub.sendSyncToClient(c)
		c.sendAck("resync", true, seq, "")
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
	case c.send <- queuedMessage{
		data:      b,
		priority:  PriorityMedium,
		seq:       seq,
		timestamp: time.Now().UnixMilli(),
	}:
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
