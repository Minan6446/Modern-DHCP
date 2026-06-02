package collab

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// HubOptions tune WebSocket hub behavior.
type HubOptions struct {
	Repository        Repository
	PresenceHeartbeat time.Duration
	OptimisticLockTTL time.Duration
	MaxSubscribers    int
}

// Hub manages collaboration WebSocket sessions.
type Hub struct {
	upgrader websocket.Upgrader
	opts     HubOptions
	repo     Repository
	logger   *zap.Logger

	mu       sync.RWMutex
	sessions map[string]*client
	rooms    map[string]map[string]*client
}

// NewHub constructs a hub with sane defaults.
func NewHub(logger *zap.Logger, opts HubOptions) *Hub {
	if opts.PresenceHeartbeat <= 0 {
		opts.PresenceHeartbeat = 15 * time.Second
	}
	if opts.OptimisticLockTTL <= 0 {
		opts.OptimisticLockTTL = 2 * time.Minute
	}
	if opts.MaxSubscribers <= 0 {
		opts.MaxSubscribers = 1024
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Hub{
		upgrader: websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }},
		opts:     opts,
		repo:     opts.Repository,
		logger:   logger,
		sessions: make(map[string]*client),
		rooms:    make(map[string]map[string]*client),
	}
}

// ServeHTTP upgrades connections and registers clients.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		http.Error(w, "collaboration disabled", http.StatusServiceUnavailable)
		return
	}
	bootstrap, ok := BootstrapFromContext(r.Context())
	if !ok {
		http.Error(w, "collaboration context missing", http.StatusUnauthorized)
		return
	}
	if err := h.ensureCapacity(); err != nil {
		h.logger.Warn("collab hub capacity reached", zap.Error(err))
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Warn("collab hub upgrade failed", zap.Error(err))
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	client, err := h.newClient(ctx, cancel, bootstrap, conn)
	if err != nil {
		cancel()
		_ = conn.Close()
		h.logger.Warn("collab client register failed", zap.Error(err))
		http.Error(w, "unable to register session", http.StatusInternalServerError)
		return
	}
	client.start()
}

// Shutdown closes all sessions.
func (h *Hub) Shutdown(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		h.mu.Lock()
		for id, sess := range h.sessions {
			sess.close()
			delete(h.sessions, id)
		}
		h.mu.Unlock()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

type client struct {
	id        string
	conn      *websocket.Conn
	hub       *Hub
	send      chan outboundMessage
	bootstrap SessionBootstrap
	session   *Session
	lock      *Lock
	ctx       context.Context
	cancel    context.CancelFunc
	once      sync.Once
}

type inboundMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type outboundMessage struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

func (h *Hub) newClient(ctx context.Context, cancel context.CancelFunc, bootstrap SessionBootstrap, conn *websocket.Conn) (*client, error) {
	now := time.Now().UTC()
	sessionID := uuid.NewString()
	expires := now.Add(h.sessionTTL())
	record := &Session{
		ID:           sessionID,
		ResourceType: bootstrap.ResourceType,
		ResourceID:   bootstrap.ResourceID,
		UserID:       bootstrap.UserID,
		Status:       "active",
		LockVersion:  0,
		ExpiresAt:    expires,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := h.repo.UpsertSession(ctx, record); err != nil {
		return nil, err
	}
	client := &client{
		id:        sessionID,
		conn:      conn,
		hub:       h,
		send:      make(chan outboundMessage, 64),
		bootstrap: bootstrap,
		session:   record,
		ctx:       ctx,
		cancel:    cancel,
	}
	if err := h.register(client); err != nil {
		return nil, err
	}
	h.recordEvent(ctx, bootstrap, sessionID, "session.join", map[string]any{"sessionId": sessionID})
	client.enqueue(outboundMessage{
		Type: "session.snapshot",
		Payload: map[string]any{
			"sessionId":    sessionID,
			"participants": h.snapshotSessions(ctx, bootstrap),
			"locks":        h.snapshotLocks(ctx, bootstrap),
		},
	})
	h.broadcast(client.roomKey(), client.id, outboundMessage{
		Type: "presence.join",
		Payload: map[string]any{
			"sessionId": sessionID,
			"userId":    bootstrap.UserID,
		},
	})
	return client, nil
}

func (h *Hub) ensureCapacity() error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if len(h.sessions) >= h.opts.MaxSubscribers {
		return errors.New("collaboration hub saturated")
	}
	return nil
}

func (h *Hub) register(c *client) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.sessions[c.id]; ok {
		return errors.New("duplicate session id")
	}
	h.sessions[c.id] = c
	key := c.roomKey()
	if _, ok := h.rooms[key]; !ok {
		h.rooms[key] = make(map[string]*client)
	}
	h.rooms[key][c.id] = c
	return nil
}

func (c *client) start() {
	go c.readLoop()
	go c.writeLoop()
}

func (c *client) readLoop() {
	defer c.close()
	deadline := c.hub.sessionTTL()
	c.conn.SetReadLimit(1 << 20)
	c.conn.SetReadDeadline(time.Now().Add(deadline))
	for {
		var msg inboundMessage
		if err := c.conn.ReadJSON(&msg); err != nil {
			c.hub.logger.Debug("collab client disconnected", zap.Error(err))
			return
		}
		c.handleMessage(msg)
		c.conn.SetReadDeadline(time.Now().Add(deadline))
	}
}

func (c *client) handleMessage(msg inboundMessage) {
	switch msg.Type {
	case "presence.heartbeat":
		c.handleHeartbeat(msg.Payload)
	case "lock.acquire":
		c.handleLockAcquire()
	case "lock.release":
		c.handleLockRelease()
	default:
		c.hub.logger.Debug("collab message ignored", zap.String("type", msg.Type))
	}
}

func (c *client) handleHeartbeat(payload json.RawMessage) {
	var body struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(payload, &body)
	expires := time.Now().UTC().Add(c.hub.sessionTTL())
	if err := c.hub.repo.TouchSession(c.ctx, c.id, expires, normalizeStatus(body.Status)); err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			c.enqueue(outboundMessage{Type: "presence.error", Payload: map[string]any{"reason": "session expired"}})
			c.close()
			return
		}
		c.hub.logger.Warn("collab heartbeat failed", zap.Error(err))
	}
	c.enqueue(outboundMessage{Type: "presence.ack", Payload: map[string]any{"sessionId": c.id, "expiresAt": expires}})
}

func (c *client) handleLockAcquire() {
	if c.lock != nil {
		c.enqueue(outboundMessage{Type: "lock.granted", Payload: c.lock})
		return
	}
	now := time.Now().UTC()
	lock := &Lock{
		ID:           uuid.NewString(),
		ResourceType: c.bootstrap.ResourceType,
		ResourceID:   c.bootstrap.ResourceID,
		SessionID:    c.id,
		Status:       "granted",
		AcquiredAt:   now,
		ExpiresAt:    now.Add(c.hub.lockTTL()),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := c.hub.repo.AcquireLock(c.ctx, lock); err != nil {
		if errors.Is(err, ErrLockConflict) {
			c.enqueue(outboundMessage{Type: "lock.error", Payload: map[string]any{"reason": "conflict"}})
			return
		}
		c.hub.logger.Warn("collab lock acquire failed", zap.Error(err))
		c.enqueue(outboundMessage{Type: "lock.error", Payload: map[string]any{"reason": "internal"}})
		return
	}
	c.lock = lock
	payload := map[string]any{
		"lockId":    lock.ID,
		"sessionId": c.id,
		"status":    "granted",
		"expiresAt": lock.ExpiresAt,
	}
	c.enqueue(outboundMessage{Type: "lock.granted", Payload: payload})
	c.hub.broadcast(c.roomKey(), "", outboundMessage{Type: "lock.state", Payload: payload})
	c.hub.recordEvent(c.ctx, c.bootstrap, c.id, "lock.granted", payload)
}

func (c *client) handleLockRelease() {
	if c.lock == nil {
		return
	}
	lockID := c.lock.ID
	if err := c.hub.repo.ReleaseLock(c.ctx, lockID); err != nil {
		c.hub.logger.Warn("collab lock release failed", zap.Error(err))
	}
	payload := map[string]any{
		"lockId":    lockID,
		"sessionId": c.id,
		"status":    "released",
	}
	c.hub.broadcast(c.roomKey(), "", outboundMessage{Type: "lock.state", Payload: payload})
	c.hub.recordEvent(c.ctx, c.bootstrap, c.id, "lock.released", payload)
	c.lock = nil
}

func (c *client) writeLoop() {
	for msg := range c.send {
		if err := c.conn.WriteJSON(msg); err != nil {
			c.hub.logger.Debug("collab send failed", zap.Error(err))
			return
		}
	}
}

func (c *client) close() {
	c.once.Do(func() {
		if c.lock != nil {
			_ = c.hub.repo.ReleaseLock(c.ctx, c.lock.ID)
		}
		_ = c.hub.repo.DeleteSession(c.ctx, c.id)
		c.hub.broadcast(c.roomKey(), c.id, outboundMessage{Type: "presence.leave", Payload: map[string]any{"sessionId": c.id}})
		c.hub.recordEvent(c.ctx, c.bootstrap, c.id, "session.leave", map[string]any{"sessionId": c.id})
		c.hub.unregister(c)
		close(c.send)
		_ = c.conn.Close()
		c.cancel()
	})
}

func (c *client) enqueue(msg outboundMessage) {
	select {
	case c.send <- msg:
	default:
		c.hub.logger.Warn("collab backpressure dropping message", zap.String("sessionId", c.id))
	}
}

func (c *client) roomKey() string {
	return roomKey(c.bootstrap)
}

func (h *Hub) unregister(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.sessions, c.id)
	key := c.roomKey()
	if room := h.rooms[key]; room != nil {
		delete(room, c.id)
		if len(room) == 0 {
			delete(h.rooms, key)
		}
	}
}

func (h *Hub) broadcast(roomKey, exclude string, msg outboundMessage) {
	h.mu.RLock()
	peers := h.rooms[roomKey]
	for id, c := range peers {
		if id == exclude {
			continue
		}
		c.enqueue(msg)
	}
	h.mu.RUnlock()
}

func (h *Hub) snapshotSessions(ctx context.Context, bootstrap SessionBootstrap) []Session {
	if h.repo == nil {
		return nil
	}
	sessions, err := h.repo.ListSessions(ctx, "", bootstrap.ResourceType, bootstrap.ResourceID, 100)
	if err != nil {
		h.logger.Warn("collab list sessions", zap.Error(err))
		return nil
	}
	return sessions
}

func (h *Hub) snapshotLocks(ctx context.Context, bootstrap SessionBootstrap) []Lock {
	locks, err := h.repo.ListLocks(ctx, "", bootstrap.ResourceType, bootstrap.ResourceID)
	if err != nil {
		h.logger.Warn("collab list locks", zap.Error(err))
		return nil
	}
	return locks
}

func (h *Hub) recordEvent(ctx context.Context, bootstrap SessionBootstrap, sessionID string, eventType string, payload map[string]any) {
	if h.repo == nil {
		return
	}
	var raw json.RawMessage
	if payload != nil {
		if data, err := json.Marshal(payload); err == nil {
			raw = data
		}
	}
	resourceType := bootstrap.ResourceType
	resourceID := bootstrap.ResourceID
	sessionRef := optionalString(sessionID)
	evt := &Event{
		SessionID:    sessionRef,
		ResourceType: &resourceType,
		ResourceID:   &resourceID,
		EventType:    eventType,
		Payload:      raw,
		CreatedAt:    time.Now().UTC(),
	}
	if err := h.repo.RecordEvent(ctx, evt); err != nil {
		h.logger.Debug("collab event record failed", zap.Error(err))
	}
}

func (h *Hub) sessionTTL() time.Duration {
	return h.opts.PresenceHeartbeat * 2
}

func (h *Hub) lockTTL() time.Duration {
	return h.opts.OptimisticLockTTL
}

func normalizeStatus(status string) string {
	if status == "" {
		return "active"
	}
	return status
}

func roomKey(bootstrap SessionBootstrap) string {
	return bootstrap.ResourceType + "|" + bootstrap.ResourceID
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}
