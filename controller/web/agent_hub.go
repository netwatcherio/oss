package web

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/kataras/iris/v12/websocket"
	log "github.com/sirupsen/logrus"
)

// AgentConnectionInfo tracks metadata about an agent's WebSocket connection
type AgentConnectionInfo struct {
	AgentID     uint              `json:"agent_id"`
	WorkspaceID uint              `json:"workspace_id"`
	ConnID      string            `json:"conn_id"`
	ClientIP    string            `json:"client_ip"`
	ConnectedAt time.Time         `json:"connected_at"`
	conn        *websocket.NSConn // internal, not exposed in JSON
}

// AgentHub manages WebSocket connections for agents.
// Tracks connected agents by ID for targeted deactivation broadcasts.
type AgentHub struct {
	mu sync.RWMutex

	// Connections indexed by agent ID (one connection per agent)
	connections map[uint]*AgentConnectionInfo
}

// AgentDeactivateMessage is sent to agents when they are deleted
type AgentDeactivateMessage struct {
	Reason string `json:"reason"`
}

// Global agent hub instance
var agentHub = NewAgentHub()

// GetAgentHub returns the global agent hub instance
func GetAgentHub() *AgentHub {
	return agentHub
}

// NewAgentHub creates a new AgentHub instance
func NewAgentHub() *AgentHub {
	return &AgentHub{
		connections: make(map[uint]*AgentConnectionInfo),
	}
}

// RegisterAgentWithInfo registers an agent connection with metadata
func (h *AgentHub) RegisterAgentWithInfo(info AgentConnectionInfo) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// If there's an existing connection for this agent, log and close it
	if old, exists := h.connections[info.AgentID]; exists {
		log.Warnf("[AgentHub] Agent %d already connected (old: conn_id=%s ip=%s since=%s), replacing with new connection (conn_id=%s ip=%s)",
			info.AgentID,
			old.ConnID, old.ClientIP, old.ConnectedAt.Format(time.RFC3339),
			info.ConnID, info.ClientIP)

		// Force disconnect the old connection in background
		go func(oldConn *websocket.NSConn) {
			time.Sleep(500 * time.Millisecond)
			if err := oldConn.Disconnect(context.TODO()); err != nil {
				log.Debugf("[AgentHub] Error disconnecting old connection for agent %d: %v", info.AgentID, err)
			}
		}(old.conn)
	}

	h.connections[info.AgentID] = &info
	log.Infof("[AgentHub] Agent %d registered (total: %d) conn_id=%s ip=%s ws=%d",
		info.AgentID, len(h.connections), info.ConnID, info.ClientIP, info.WorkspaceID)
}

// RegisterAgent registers an agent connection (legacy compatibility)
func (h *AgentHub) RegisterAgent(agentID uint, conn *websocket.NSConn) {
	h.RegisterAgentWithInfo(AgentConnectionInfo{
		AgentID:     agentID,
		conn:        conn,
		ConnectedAt: time.Now(),
	})
}

// UnregisterAgent removes an agent connection
// Only removes if the agent is currently registered (prevents race conditions
// where an old connection's disconnect event fires after a new connection was registered)
func (h *AgentHub) UnregisterAgent(agentID uint, conn *websocket.NSConn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Only remove if the connection matches what's currently registered
	if current, exists := h.connections[agentID]; exists {
		if current.conn == conn {
			delete(h.connections, agentID)
			log.Infof("[AgentHub] Agent %d unregistered (total: %d) conn_id=%s", agentID, len(h.connections), current.ConnID)
		} else {
			log.Debugf("[AgentHub] Agent %d disconnect ignored - connection was replaced (old=%s, current=%s)",
				agentID, current.ConnID, current.ConnID)
		}
	}
}

// DeactivateAgent sends a deactivation message to a connected agent
func (h *AgentHub) DeactivateAgent(agentID uint, reason string) bool {
	h.mu.RLock()
	info, exists := h.connections[agentID]
	h.mu.RUnlock()

	if !exists || info.conn == nil {
		log.Debugf("[AgentHub] Agent %d not connected, cannot send deactivate", agentID)
		return false
	}

	msg := AgentDeactivateMessage{Reason: reason}
	payload, err := json.Marshal(msg)
	if err != nil {
		log.Errorf("[AgentHub] Failed to marshal deactivate message: %v", err)
		return false
	}

	if !info.conn.Emit("deactivate", payload) {
		log.Warnf("[AgentHub] Failed to emit deactivate to agent %d", agentID)
		return false
	}

	log.Infof("[AgentHub] Sent deactivate to agent %d (reason: %s)", agentID, reason)
	return true
}

// IsAgentConnected checks if an agent is currently connected
func (h *AgentHub) IsAgentConnected(agentID uint) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, exists := h.connections[agentID]
	return exists
}

// GetConnectedAgentCount returns the number of connected agents
func (h *AgentHub) GetConnectedAgentCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.connections)
}

// GetConnectedAgentIDs returns a list of all connected agent IDs
func (h *AgentHub) GetConnectedAgentIDs() []uint {
	h.mu.RLock()
	defer h.mu.RUnlock()
	ids := make([]uint, 0, len(h.connections))
	for id := range h.connections {
		ids = append(ids, id)
	}
	return ids
}

// GetActiveConnections returns connection info for all connected agents (for admin debugging)
func (h *AgentHub) GetActiveConnections() []AgentConnectionInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]AgentConnectionInfo, 0, len(h.connections))
	for _, info := range h.connections {
		// Return a copy without the internal connection pointer
		result = append(result, AgentConnectionInfo{
			AgentID:     info.AgentID,
			WorkspaceID: info.WorkspaceID,
			ConnID:      info.ConnID,
			ClientIP:    info.ClientIP,
			ConnectedAt: info.ConnectedAt,
		})
	}
	return result
}

// GetConnectionInfo returns connection info for a specific agent (for debugging)
func (h *AgentHub) GetConnectionInfo(agentID uint) *AgentConnectionInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if info, exists := h.connections[agentID]; exists {
		return &AgentConnectionInfo{
			AgentID:     info.AgentID,
			WorkspaceID: info.WorkspaceID,
			ConnID:      info.ConnID,
			ClientIP:    info.ClientIP,
			ConnectedAt: info.ConnectedAt,
		}
	}
	return nil
}
