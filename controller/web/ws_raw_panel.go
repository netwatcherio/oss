package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kataras/iris/v12"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"netwatcher-controller/internal/users"
)

// RawPanelHub manages raw WebSocket connections for panel clients
type RawPanelHub struct {
	mu    sync.RWMutex
	conns map[string]*RawPanelConn

	// Subscriptions: workspaceID -> probeID -> set of connection IDs
	subscriptions map[uint]map[uint]map[string]struct{}
}

type RawPanelConn struct {
	ID     string
	UserID uint
	Conn   *websocket.Conn
	Send   chan []byte
}

// Raw WebSocket upgrader
var rawUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Global raw panel hub
var rawPanelHub = &RawPanelHub{
	conns:         make(map[string]*RawPanelConn),
	subscriptions: make(map[uint]map[uint]map[string]struct{}),
}

// GetRawPanelHub returns the global raw panel hub
func GetRawPanelHub() *RawPanelHub {
	return rawPanelHub
}

// RegisterRawPanelWS registers the raw WebSocket endpoint for panel
func RegisterRawPanelWS(app *iris.Application, db *gorm.DB) {
	app.Get("/ws/panel/raw", func(ctx iris.Context) {
		// Authenticate via query param token
		token := ctx.URLParam("token")
		if token == "" {
			authHeader := ctx.GetHeader("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				token = strings.TrimPrefix(authHeader, "Bearer ")
			}
		}

		if token == "" {
			ctx.StatusCode(http.StatusUnauthorized)
			ctx.JSON(iris.Map{"error": "missing token"})
			return
		}

		u, _, err := users.GetUserFromToken(context.Background(), db, token)
		if err != nil {
			ctx.StatusCode(http.StatusUnauthorized)
			ctx.JSON(iris.Map{"error": "invalid token"})
			return
		}

		// Upgrade to WebSocket
		ws, err := rawUpgrader.Upgrade(ctx.ResponseWriter(), ctx.Request(), nil)
		if err != nil {
			log.Errorf("[RawPanelWS] Upgrade error: %v", err)
			return
		}

		connID := ctx.Request().Header.Get("Sec-WebSocket-Key")
		if connID == "" {
			connID = time.Now().Format(time.RFC3339Nano)
		}

		conn := &RawPanelConn{
			ID:     connID,
			UserID: u.ID,
			Conn:   ws,
			Send:   make(chan []byte, 256),
		}

		rawPanelHub.Register(conn)
		log.Infof("[RawPanelWS] User %d connected as %s", u.ID, connID)

		// Start read/write pumps
		go conn.writePump()
		go conn.readPump(rawPanelHub)
	})
}

// Register adds a connection to the hub
func (h *RawPanelHub) Register(conn *RawPanelConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.conns[conn.ID] = conn
}

// Unregister removes a connection from the hub
func (h *RawPanelHub) Unregister(connID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Remove all subscriptions
	for wsID, probeMap := range h.subscriptions {
		for probeID, connMap := range probeMap {
			delete(connMap, connID)
			if len(connMap) == 0 {
				delete(probeMap, probeID)
			}
		}
		if len(probeMap) == 0 {
			delete(h.subscriptions, wsID)
		}
	}

	if conn, ok := h.conns[connID]; ok {
		close(conn.Send)
		delete(h.conns, connID)
	}
}

// Subscribe adds a subscription
func (h *RawPanelHub) Subscribe(connID string, workspaceID, probeID uint) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.subscriptions[workspaceID]; !ok {
		h.subscriptions[workspaceID] = make(map[uint]map[string]struct{})
	}
	if _, ok := h.subscriptions[workspaceID][probeID]; !ok {
		h.subscriptions[workspaceID][probeID] = make(map[string]struct{})
	}
	h.subscriptions[workspaceID][probeID][connID] = struct{}{}

	log.Infof("[RawPanelWS] Subscription added: conn=%s ws=%d probe=%d", connID, workspaceID, probeID)
}

// BroadcastRaw sends data to subscribed raw panel clients
func (h *RawPanelHub) BroadcastRaw(data ProbeDataBroadcast) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	payload, err := json.Marshal(map[string]interface{}{
		"event": "probe_data",
		"data":  data,
	})
	if err != nil {
		log.Errorf("[RawPanelWS] Marshal error: %v", err)
		return
	}

	recipients := make(map[string]struct{})

	if wsMap, ok := h.subscriptions[data.WorkspaceID]; ok {
		// Specific probe subscriptions
		if connMap, ok := wsMap[data.ProbeID]; ok {
			for connID := range connMap {
				recipients[connID] = struct{}{}
			}
		}
		// Workspace-wide subscriptions (probeID = 0)
		if connMap, ok := wsMap[0]; ok {
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
				log.Warnf("[RawPanelWS] Send buffer full for %s", connID)
			}
		}
	}

	if len(recipients) > 0 {
		log.Debugf("[RawPanelWS] Broadcast sent to %d clients", len(recipients))
	}
}

// SpeedtestUpdate represents a speedtest queue status change
type SpeedtestUpdate struct {
	QueueID     uint   `json:"queue_id"`
	WorkspaceID uint   `json:"workspace_id"`
	AgentID     uint   `json:"agent_id"`
	Status      string `json:"status"` // "completed", "failed", "running"
	Error       string `json:"error,omitempty"`
	ServerID    string `json:"server_id,omitempty"`
	ServerName  string `json:"server_name,omitempty"`
}

// BroadcastSpeedtestUpdate sends speedtest queue updates to all subscribed panel clients for the workspace
func (h *RawPanelHub) BroadcastSpeedtestUpdate(update SpeedtestUpdate) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	payload, err := json.Marshal(map[string]interface{}{
		"event": "speedtest_update",
		"data":  update,
	})
	if err != nil {
		log.Errorf("[RawPanelWS] Marshal speedtest update error: %v", err)
		return
	}

	recipients := make(map[string]struct{})

	// Send to all connections subscribed to this workspace (probeID 0 = workspace-wide)
	if wsMap, ok := h.subscriptions[update.WorkspaceID]; ok {
		if connMap, ok := wsMap[0]; ok {
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
				log.Warnf("[RawPanelWS] Send buffer full for %s (speedtest update)", connID)
			}
		}
	}

	if len(recipients) > 0 {
		log.Debugf("[RawPanelWS] Speedtest update broadcast to %d clients (queue=%d, status=%s)", len(recipients), update.QueueID, update.Status)
	}
}

// Read pump for raw WebSocket
func (c *RawPanelConn) readPump(hub *RawPanelHub) {
	defer func() {
		hub.Unregister(c.ID)
		c.Conn.Close()
		log.Infof("[RawPanelWS] Connection %s closed", c.ID)
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
				log.Warnf("[RawPanelWS] Read error: %v", err)
			}
			break
		}

		// Parse incoming message
		var msg struct {
			Event string          `json:"event"`
			Data  json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Warnf("[RawPanelWS] Parse error: %v", err)
			continue
		}

		switch msg.Event {
		case "subscribe":
			var sub struct {
				WorkspaceID uint `json:"workspace_id"`
				ProbeID     uint `json:"probe_id"`
			}
			if err := json.Unmarshal(msg.Data, &sub); err == nil {
				hub.Subscribe(c.ID, sub.WorkspaceID, sub.ProbeID)
				// Send ack
				ack, _ := json.Marshal(map[string]interface{}{
					"event": "subscribe_ok",
					"data":  sub,
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
func (c *RawPanelConn) writePump() {
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
