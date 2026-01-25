package web

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/kataras/iris/v12/websocket"
	log "github.com/sirupsen/logrus"
)

// AgentHub manages WebSocket connections for agents.
// Tracks connected agents by ID for targeted deactivation broadcasts.
type AgentHub struct {
	mu sync.RWMutex

	// Connections indexed by agent ID (one connection per agent)
	connections map[uint]*websocket.NSConn
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
		connections: make(map[uint]*websocket.NSConn),
	}
}

// RegisterAgent registers an agent connection
func (h *AgentHub) RegisterAgent(agentID uint, conn *websocket.NSConn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// If there's an existing connection for this agent, close it first
	// This handles the case where an agent reconnects after an update/restart
	// before the old connection has timed out
	if old, exists := h.connections[agentID]; exists {
		log.Warnf("[AgentHub] Agent %d already connected, closing old connection before replacing", agentID)
		// Force disconnect the old connection to ensure clean state
		// Use a delay to allow the new connection to stabilize first
		go func(oldConn *websocket.NSConn) {
			// Brief delay to let the new connection fully register before
			// disconnecting the old one (prevents race condition where
			// old disconnect event interferes with new connection)
			time.Sleep(500 * time.Millisecond)
			if err := oldConn.Disconnect(context.TODO()); err != nil {
				log.Debugf("[AgentHub] Error disconnecting old connection for agent %d: %v", agentID, err)
			}
		}(old)

	}

	h.connections[agentID] = conn
	log.Infof("[AgentHub] Agent %d registered (total: %d)", agentID, len(h.connections))
}

// UnregisterAgent removes an agent connection
// Only removes if the agent is currently registered (prevents race conditions
// where an old connection's disconnect event fires after a new connection was registered)
func (h *AgentHub) UnregisterAgent(agentID uint, conn *websocket.NSConn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Only remove if the connection matches what's currently registered
	// This prevents race conditions where an old connection disconnects after
	// a new one was registered
	if current, exists := h.connections[agentID]; exists {
		if current == conn {
			delete(h.connections, agentID)
			log.Infof("[AgentHub] Agent %d unregistered (total: %d)", agentID, len(h.connections))
		} else {
			log.Debugf("[AgentHub] Agent %d disconnect ignored - connection was replaced", agentID)
		}
	}
}

// DeactivateAgent sends a deactivation message to a connected agent
// This is called when an agent is deleted from the panel
func (h *AgentHub) DeactivateAgent(agentID uint, reason string) bool {
	h.mu.RLock()
	conn, exists := h.connections[agentID]
	h.mu.RUnlock()

	if !exists {
		log.Debugf("[AgentHub] Agent %d not connected, cannot send deactivate", agentID)
		return false
	}

	msg := AgentDeactivateMessage{Reason: reason}
	payload, err := json.Marshal(msg)
	if err != nil {
		log.Errorf("[AgentHub] Failed to marshal deactivate message: %v", err)
		return false
	}

	if !conn.Emit("deactivate", payload) {
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
