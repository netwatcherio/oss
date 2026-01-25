package web

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kataras/iris/v12"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"netwatcher-controller/internal/share"
)

// RawShareHub manages raw WebSocket connections for share-link authenticated clients
type RawShareHub struct {
	mu    sync.RWMutex
	conns map[string]*RawShareConn

	// Subscriptions by agentID -> probeID -> set of connection IDs
	// (share links are agent-scoped, so we key by agent instead of workspace)
	subscriptions map[uint]map[uint]map[string]struct{}
}

type RawShareConn struct {
	ID      string
	Token   string // Share link token
	AgentID uint   // Agent this share gives access to
	Conn    *websocket.Conn
	Send    chan []byte
}

// Global share hub
var rawShareHub = &RawShareHub{
	conns:         make(map[string]*RawShareConn),
	subscriptions: make(map[uint]map[uint]map[string]struct{}),
}

// GetRawShareHub returns the global share hub
func GetRawShareHub() *RawShareHub {
	return rawShareHub
}

// RegisterRawShareWS registers the raw WebSocket endpoint for share-link access
func RegisterRawShareWS(app *iris.Application, db *gorm.DB) {
	app.Get("/ws/share/raw", func(ctx iris.Context) {
		// Authenticate via share token + optional password in query params
		token := ctx.URLParam("token")
		password := ctx.URLParam("password")

		if token == "" {
			ctx.StatusCode(http.StatusUnauthorized)
			ctx.JSON(iris.Map{"error": "missing token"})
			return
		}

		// Validate share link (one-time auth at connection)
		link, err := share.Validate(ctx.Request().Context(), db, share.ValidateInput{
			Token:    token,
			Password: password,
		})
		if err != nil {
			ctx.StatusCode(http.StatusUnauthorized)
			ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		// Upgrade to WebSocket
		ws, err := rawUpgrader.Upgrade(ctx.ResponseWriter(), ctx.Request(), nil)
		if err != nil {
			log.Errorf("[RawShareWS] Upgrade error: %v", err)
			return
		}

		connID := ctx.Request().Header.Get("Sec-WebSocket-Key")
		if connID == "" {
			connID = time.Now().Format(time.RFC3339Nano)
		}

		conn := &RawShareConn{
			ID:      connID,
			Token:   token,
			AgentID: link.AgentID,
			Conn:    ws,
			Send:    make(chan []byte, 256),
		}

		rawShareHub.Register(conn)
		log.Infof("[RawShareWS] Share token connected for agent %d as %s", link.AgentID, connID)

		// Auto-subscribe to this agent's probes (probeID 0 = all probes for this agent)
		rawShareHub.Subscribe(connID, link.AgentID, 0)

		// Start read/write pumps
		go conn.writePump()
		go conn.readPump(rawShareHub)
	})
}

// Register adds a connection to the hub
func (h *RawShareHub) Register(conn *RawShareConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.conns[conn.ID] = conn
}

// Unregister removes a connection from the hub
func (h *RawShareHub) Unregister(connID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Remove all subscriptions
	for agentID, probeMap := range h.subscriptions {
		for probeID, connMap := range probeMap {
			delete(connMap, connID)
			if len(connMap) == 0 {
				delete(probeMap, probeID)
			}
		}
		if len(probeMap) == 0 {
			delete(h.subscriptions, agentID)
		}
	}

	if conn, ok := h.conns[connID]; ok {
		close(conn.Send)
		delete(h.conns, connID)
	}
}

// Subscribe adds a subscription
func (h *RawShareHub) Subscribe(connID string, agentID, probeID uint) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.subscriptions[agentID]; !ok {
		h.subscriptions[agentID] = make(map[uint]map[string]struct{})
	}
	if _, ok := h.subscriptions[agentID][probeID]; !ok {
		h.subscriptions[agentID][probeID] = make(map[string]struct{})
	}
	h.subscriptions[agentID][probeID][connID] = struct{}{}

	log.Infof("[RawShareWS] Subscription added: conn=%s agent=%d probe=%d", connID, agentID, probeID)
}

// BroadcastToShare sends probe data to share-link clients subscribed to this agent
func (h *RawShareHub) BroadcastToShare(agentID uint, data ProbeDataBroadcast) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	payload, err := json.Marshal(map[string]interface{}{
		"event": "probe_data",
		"data":  data,
	})
	if err != nil {
		log.Errorf("[RawShareWS] Marshal error: %v", err)
		return
	}

	recipients := make(map[string]struct{})

	if agentMap, ok := h.subscriptions[agentID]; ok {
		// Specific probe subscriptions
		if connMap, ok := agentMap[data.ProbeID]; ok {
			for connID := range connMap {
				recipients[connID] = struct{}{}
			}
		}
		// Agent-wide subscriptions (probeID = 0)
		if connMap, ok := agentMap[0]; ok {
			for connID := range connMap {
				recipients[connID] = struct{}{}
			}
		}
	}

	for connID := range recipients {
		if conn, ok := h.conns[connID]; ok {
			select {
			case conn.Send <- payload:
			default:
				log.Warnf("[RawShareWS] Send buffer full for %s", connID)
			}
		}
	}

	if len(recipients) > 0 {
		log.Debugf("[RawShareWS] Broadcast sent to %d share clients for agent %d", len(recipients), agentID)
	}
}

// Read pump for raw WebSocket
func (c *RawShareConn) readPump(hub *RawShareHub) {
	defer func() {
		hub.Unregister(c.ID)
		c.Conn.Close()
		log.Infof("[RawShareWS] Connection %s closed", c.ID)
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
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Warnf("[RawShareWS] Read error: %v", err)
			}
			break
		}

		// Parse incoming message
		var msg struct {
			Event string          `json:"event"`
			Data  json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Warnf("[RawShareWS] Parse error: %v", err)
			continue
		}

		switch msg.Event {
		case "subscribe":
			// Share clients can only subscribe to probes belonging to their agent
			var sub struct {
				ProbeID uint `json:"probe_id"`
			}
			if err := json.Unmarshal(msg.Data, &sub); err == nil {
				hub.Subscribe(c.ID, c.AgentID, sub.ProbeID)
				// Send ack
				ack, _ := json.Marshal(map[string]interface{}{
					"event": "subscribe_ok",
					"data":  map[string]uint{"agent_id": c.AgentID, "probe_id": sub.ProbeID},
				})
				c.Send <- ack
			}

		case "ping":
			pong, _ := json.Marshal(map[string]string{"event": "pong"})
			c.Send <- pong
		}
	}
}

// Write pump for raw WebSocket
func (c *RawShareConn) writePump() {
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
