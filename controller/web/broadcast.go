package web

import (
	"encoding/json"
	"sync"

	"github.com/kataras/iris/v12/websocket"
	log "github.com/sirupsen/logrus"
)

// PanelHub manages WebSocket subscriptions for panel clients.
// Panel clients can subscribe to probe updates and receive real-time data.
type PanelHub struct {
	mu sync.RWMutex

	// Connections indexed by connection ID
	connections map[string]*websocket.NSConn

	// Subscriptions: workspaceID -> probeID -> set of connection IDs
	// probeID of 0 means "all probes in workspace"
	subscriptions map[uint]map[uint]map[string]struct{}

	// Reverse lookup: connection ID -> list of (workspaceID, probeID) pairs
	connSubscriptions map[string][]subscription
}

type subscription struct {
	WorkspaceID uint `json:"workspace_id"`
	ProbeID     uint `json:"probe_id"` // 0 = all probes in workspace
}

// ProbeDataBroadcast represents the data sent to panel clients
type ProbeDataBroadcast struct {
	WorkspaceID  uint            `json:"workspace_id"`
	ProbeID      uint            `json:"probe_id"`
	AgentID      uint            `json:"agent_id"`       // Reporting agent (who sent this data)
	ProbeAgentID uint            `json:"probe_agent_id"` // Probe owner agent (for direction detection)
	TargetAgent  uint            `json:"target_agent"`   // Target agent ID (for direction detection)
	Type         string          `json:"type"`
	Payload      json.RawMessage `json:"payload"`
	CreatedAt    string          `json:"created_at"`
	Target       string          `json:"target,omitempty"`
	Triggered    bool            `json:"triggered,omitempty"`
}

// Global panel hub instance
var panelHub = NewPanelHub()

// GetPanelHub returns the global panel hub instance
func GetPanelHub() *PanelHub {
	return panelHub
}

// NewPanelHub creates a new PanelHub instance
func NewPanelHub() *PanelHub {
	return &PanelHub{
		connections:       make(map[string]*websocket.NSConn),
		subscriptions:     make(map[uint]map[uint]map[string]struct{}),
		connSubscriptions: make(map[string][]subscription),
	}
}

// RegisterConnection registers a panel client connection
func (h *PanelHub) RegisterConnection(connID string, conn *websocket.NSConn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.connections[connID] = conn
	h.connSubscriptions[connID] = []subscription{}

	log.Infof("[PanelHub] Connection registered: %s", connID)
}

// UnregisterConnection removes a panel client and all its subscriptions
func (h *PanelHub) UnregisterConnection(connID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Remove all subscriptions for this connection
	if subs, ok := h.connSubscriptions[connID]; ok {
		for _, sub := range subs {
			if wsMap, ok := h.subscriptions[sub.WorkspaceID]; ok {
				if probeMap, ok := wsMap[sub.ProbeID]; ok {
					delete(probeMap, connID)
					if len(probeMap) == 0 {
						delete(wsMap, sub.ProbeID)
					}
				}
				if len(wsMap) == 0 {
					delete(h.subscriptions, sub.WorkspaceID)
				}
			}
		}
	}

	delete(h.connSubscriptions, connID)
	delete(h.connections, connID)

	log.Infof("[PanelHub] Connection unregistered: %s", connID)
}

// Subscribe adds a subscription for probe updates
// probeID of 0 subscribes to all probes in the workspace
func (h *PanelHub) Subscribe(connID string, workspaceID, probeID uint) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Initialize nested maps if needed
	if _, ok := h.subscriptions[workspaceID]; !ok {
		h.subscriptions[workspaceID] = make(map[uint]map[string]struct{})
	}
	if _, ok := h.subscriptions[workspaceID][probeID]; !ok {
		h.subscriptions[workspaceID][probeID] = make(map[string]struct{})
	}

	// Add subscription
	h.subscriptions[workspaceID][probeID][connID] = struct{}{}

	// Track for cleanup
	h.connSubscriptions[connID] = append(h.connSubscriptions[connID], subscription{
		WorkspaceID: workspaceID,
		ProbeID:     probeID,
	})

	log.Infof("[PanelHub] Subscription added: conn=%s workspace=%d probe=%d", connID, workspaceID, probeID)
}

// Unsubscribe removes a subscription
func (h *PanelHub) Unsubscribe(connID string, workspaceID, probeID uint) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if wsMap, ok := h.subscriptions[workspaceID]; ok {
		if probeMap, ok := wsMap[probeID]; ok {
			delete(probeMap, connID)
			if len(probeMap) == 0 {
				delete(wsMap, probeID)
			}
		}
		if len(wsMap) == 0 {
			delete(h.subscriptions, workspaceID)
		}
	}

	// Update connSubscriptions (remove matching entry)
	if subs, ok := h.connSubscriptions[connID]; ok {
		filtered := subs[:0]
		for _, s := range subs {
			if s.WorkspaceID != workspaceID || s.ProbeID != probeID {
				filtered = append(filtered, s)
			}
		}
		h.connSubscriptions[connID] = filtered
	}

	log.Infof("[PanelHub] Subscription removed: conn=%s workspace=%d probe=%d", connID, workspaceID, probeID)
}

// Broadcast sends probe data to all subscribed panel clients
func (h *PanelHub) Broadcast(data ProbeDataBroadcast) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	payload, err := json.Marshal(data)
	if err != nil {
		log.Errorf("[PanelHub] Failed to marshal broadcast data: %v", err)
		return
	}

	// Collect unique connection IDs that should receive this broadcast
	recipients := make(map[string]struct{})

	wsMap, ok := h.subscriptions[data.WorkspaceID]
	if !ok {
		return // No subscriptions for this workspace
	}

	// Check for specific probe subscriptions
	if probeMap, ok := wsMap[data.ProbeID]; ok {
		for connID := range probeMap {
			recipients[connID] = struct{}{}
		}
	}

	// Check for workspace-wide subscriptions (probeID = 0)
	if probeMap, ok := wsMap[0]; ok {
		for connID := range probeMap {
			recipients[connID] = struct{}{}
		}
	}

	// Send to all recipients
	for connID := range recipients {
		if conn, ok := h.connections[connID]; ok {
			if !conn.Emit("probe_data", payload) {
				log.Warnf("[PanelHub] Failed to emit to connection: %s", connID)
			}
		}
	}

	if len(recipients) > 0 {
		log.Debugf("[PanelHub] Broadcast sent to %d clients for probe %d", len(recipients), data.ProbeID)
	}
}

// GetSubscriberCount returns the number of active subscriptions (for debugging)
func (h *PanelHub) GetSubscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	count := 0
	for _, wsMap := range h.subscriptions {
		for _, probeMap := range wsMap {
			count += len(probeMap)
		}
	}
	return count
}
