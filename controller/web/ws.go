package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"netwatcher-controller/internal/agent"
	"netwatcher-controller/internal/lookup"
	probe "netwatcher-controller/internal/probe"
	"netwatcher-controller/internal/speedtest"
	"netwatcher-controller/internal/users"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"

	gorillaWs "github.com/gorilla/websocket"
	_ "github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/websocket"
	log "github.com/sirupsen/logrus"
)

// WebSocket connection settings for improved stability
const (
	// Time allowed to write a message to the peer
	writeWait = 60 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 90 * time.Second

	// Send pings to peer with this period (must be less than pongWait)
	pingPeriod = 30 * time.Second

	// Maximum message size allowed from peer
	maxMessageSize = 512 * 1024 // 512KB
)

// agentUpgrader is a custom upgrader with proper buffer sizes for agent connections
var agentUpgrader = gorillaWs.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for agent connections
	},
	HandshakeTimeout: 10 * time.Second,
}

// Expected headers for WS:
//   X-Workspace-ID: <uint>
//   X-Agent-ID:     <uint>
//   X-Agent-PSK:    <string>
//
// If valid, we attach agent/workspace IDs to the Iris context for use in events.

func addWebSocketServer(app *iris.Application, db *gorm.DB, ch *sql.DB) error {
	// Agent WebSocket server - uses PSK header auth with custom upgrader for stability
	agentWsServer := websocket.New(
		websocket.GorillaUpgrader(agentUpgrader),
		getAgentWebsocketEvents(app, db, ch),
	)
	agentWsServer.OnConnect = func(c *websocket.Conn) error {
		ctx := websocket.GetContext(c)

		wsIDStr := ctx.GetHeader("X-Workspace-ID")
		agIDStr := ctx.GetHeader("X-Agent-ID")
		psk := ctx.GetHeader("X-Agent-PSK")

		if wsIDStr == "" || agIDStr == "" || psk == "" {
			ctx.StatusCode(http.StatusUnauthorized)
			return errors.New("unauthorized: missing X-Workspace-ID, X-Agent-ID, or X-Agent-PSK headers")
		}

		wsID64, err := strconv.ParseUint(strings.TrimSpace(wsIDStr), 10, 64)
		if err != nil {
			ctx.StatusCode(http.StatusUnauthorized)
			return errors.New("unauthorized: invalid workspace id")
		}
		agID64, err := strconv.ParseUint(strings.TrimSpace(agIDStr), 10, 64)
		if err != nil {
			ctx.StatusCode(http.StatusUnauthorized)
			return errors.New("unauthorized: invalid agent id")
		}

		a, err := agent.AuthenticateWithPSK(ctx, db, uint(wsID64), uint(agID64), psk)
		if err != nil {
			// Check if agent was deleted - return proper error to signal deactivation
			if errors.Is(err, agent.ErrAgentDeleted) {
				log.Infof("WS: Agent %d/%d attempted connect after deletion", wsID64, agID64)
				ctx.StatusCode(http.StatusGone) // 410 Gone
				return errors.New("agent_deleted: agent has been removed from workspace")
			}
			ctx.StatusCode(http.StatusUnauthorized)
			return errors.New("unauthorized: invalid psk")
		}

		_ = agent.UpdateAgentSeen(ctx, db, a.ID, time.Now())
		ctx.Values().Set("agent_id", a.ID)
		ctx.Values().Set("workspace_id", a.WorkspaceID)
		ctx.Values().Set("client_type", "agent")
		ctx.Values().Set("conn_id", c.ID()) // Track connection ID for session debugging

		log.Infof("WS auth ok — agent %d (ws %d) conn_id=%s", a.ID, a.WorkspaceID, c.ID())
		return nil
	}

	// Panel WebSocket server - uses JWT query param auth (browsers can't send headers)
	panelWsServer := websocket.New(
		websocket.DefaultGorillaUpgrader,
		getPanelWebsocketEvents(app, db, ch),
	)
	panelWsServer.OnConnect = func(c *websocket.Conn) error {
		ctx := websocket.GetContext(c)

		// Get JWT from query param (browsers can't send Authorization header with WebSocket)
		token := ctx.URLParam("token")
		if token == "" {
			// Fallback to Authorization header (for non-browser clients)
			authHeader := ctx.GetHeader("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				token = strings.TrimPrefix(authHeader, "Bearer ")
			}
		}

		if token == "" {
			ctx.StatusCode(http.StatusUnauthorized)
			return errors.New("unauthorized: missing token query param or Authorization header")
		}

		u, sess, err := users.GetUserFromToken(context.Background(), db, token)
		if err != nil {
			ctx.StatusCode(http.StatusUnauthorized)
			return errors.New("unauthorized: invalid jwt token")
		}

		ctx.Values().Set("user_id", u.ID)
		ctx.Values().Set("session_id", sess.SessionID)
		ctx.Values().Set("client_type", "panel")

		log.Infof("WS auth ok — panel user %d (session %d) connected as %s", u.ID, sess.SessionID, c.ID())
		return nil
	}

	// Mount separate endpoints
	app.Get("/ws/agent", websocket.Handler(agentWsServer))
	app.Get("/ws/panel", websocket.Handler(panelWsServer))

	// Keep legacy /ws for backwards compatibility (routes to agent)
	app.Get("/ws", websocket.Handler(agentWsServer))

	return nil
}

// getAgentWebsocketEvents returns the namespace events for agent connections
func getAgentWebsocketEvents(app *iris.Application, db *gorm.DB, ch *sql.DB) websocket.Namespaces {
	return websocket.Namespaces{
		"agent": websocket.Events{
			websocket.OnNamespaceConnected: func(nsConn *websocket.NSConn, msg websocket.Message) error {
				ctx := websocket.GetContext(nsConn.Conn)
				aid, _ := ctx.Values().GetUint("agent_id")
				wsid, _ := ctx.Values().GetUint("workspace_id")
				connID := ctx.Values().GetString("conn_id")
				clientIP := lookup.GetClientIP(ctx)
				log.Infof("[NS_CONNECT] agent=%d ws=%d conn_id=%s ip=%s ns=%s", aid, wsid, connID, clientIP, msg.Namespace)

				// Register with AgentHub with full metadata for debugging
				GetAgentHub().RegisterAgentWithInfo(AgentConnectionInfo{
					AgentID:     aid,
					WorkspaceID: wsid,
					ConnID:      connID,
					ClientIP:    clientIP,
					ConnectedAt: time.Now(),
					conn:        nsConn,
				})

				return nil
			},
			websocket.OnNamespaceDisconnect: func(nsConn *websocket.NSConn, msg websocket.Message) error {
				ctx := websocket.GetContext(nsConn.Conn)
				aid, _ := ctx.Values().GetUint("agent_id")
				connID := ctx.Values().GetString("conn_id")

				// Unregister from AgentHub - pass connection to prevent race condition
				// where old disconnect removes newly registered connection
				GetAgentHub().UnregisterAgent(aid, nsConn)

				log.Infof("[NS_DISCONNECT] agent=%d conn_id=%s ns=%s", aid, connID, msg.Namespace)
				return nil
			},

			// Deactivate handler - sent when agent is deleted from panel
			// Agent receives this and should clean up and exit
			"deactivate": func(nsConn *websocket.NSConn, msg websocket.Message) error {
				// This is sent TO the agent, not FROM the agent
				// No action needed on controller side for incoming deactivate messages
				log.Debugf("[%s] received deactivate ack", nsConn)
				return nil
			},

			// Ping/heartbeat handler - agents send this periodically to keep connection alive
			// Also serves as online status tracking by updating last_seen_at
			"ping": func(nsConn *websocket.NSConn, msg websocket.Message) error {
				ctx := websocket.GetContext(nsConn.Conn)
				aid, ok := ctx.Values().GetUint("agent_id")
				if ok != nil || aid == 0 {
					return errors.New("unauthorized: no agent in context")
				}

				// Update last_seen_at for status tracking
				if err := agent.UpdateAgentSeen(context.TODO(), db, aid, time.Now()); err != nil {
					log.WithError(err).Warnf("[ping] failed to update last_seen for agent %d", aid)
				}

				// Respond with pong
				nsConn.Emit("pong", []byte(`{"ok":true}`))
				return nil
			},

			"version": func(nsConn *websocket.NSConn, msg websocket.Message) error {
				var versionData = struct {
					Version string `json:"version"`
				}{}

				ctx := websocket.GetContext(nsConn.Conn)
				aid, ok := ctx.Values().GetUint("agent_id")
				if (ok != nil) || (aid == 0) {
					return errors.New("unauthorized: no agent in context")
				}

				log.Infof("[%s] received version update message [%s]: %s", nsConn, msg.Namespace, msg.Body)

				// Load and update agent
				a, err := agent.GetAgentByID(context.TODO(), db, aid)
				if err != nil {
					log.Error(err)
					return err
				}
				if err := json.Unmarshal(msg.Body, &versionData); err != nil {
					log.Error(err)
					return err
				}

				if err := agent.UpdateAgentVersion(context.TODO(), db, a.ID, versionData.Version); err != nil {
					log.Error(err)
				}

				nsConn.Emit("version", []byte("ok"))
				return nil
			},

			"probe_get": func(nsConn *websocket.NSConn, msg websocket.Message) error {
				ctx := websocket.GetContext(nsConn.Conn)
				aid, ok := ctx.Values().GetUint("agent_id")
				if (ok != nil) || (aid == 0) {
					return errors.New("unauthorized: no agent in context")
				}

				a, err := agent.GetAgentByID(context.TODO(), db, aid)
				if err != nil {
					log.Error(err)
					return err
				}
				if err := agent.UpdateAgentSeen(context.TODO(), db, a.ID, time.Now()); err != nil {
					log.Error(err)
				}

				ownedP, err := probe.ListForAgent(context.TODO(), db, ch, a.ID)
				if err != nil {
					log.Errorf("probe_get: %v", err)
				}

				// Log summary of probes being sent (not full JSON)
				typeCounts := make(map[string]int)
				var probeIDs []uint
				for _, p := range ownedP {
					typeCounts[string(p.Type)]++
					probeIDs = append(probeIDs, p.ID)
				}
				log.Infof("[probe_get] agent %d: sending %d probes %v (IDs: %v)", a.ID, len(ownedP), typeCounts, probeIDs)

				payload, err := json.Marshal(ownedP)
				if err != nil {
					return err
				}

				nsConn.Emit("probe_get", payload)
				return nil
			},

			"probe_post": func(nsConn *websocket.NSConn, msg websocket.Message) error {
				ctx := websocket.GetContext(nsConn.Conn)
				aid, ok := ctx.Values().GetUint("agent_id")
				if (ok != nil) || (aid == 0) {
					return errors.New("unauthorized: no agent in context")
				}
				wsid, _ := ctx.Values().GetUint("workspace_id")
				connID := ctx.Values().GetString("conn_id")

				var pp probe.ProbeData
				if err := json.Unmarshal(msg.Body, &pp); err != nil {
					log.Error(err)
					return err
				}

				pp.AgentID = aid
				if pp.CreatedAt.IsZero() {
					pp.CreatedAt = time.Now()
				}
				pp.ReceivedAt = time.Now()

				// Log summarized probe data with connection ID for session debugging
				targetInfo := pp.Target
				if pp.TargetAgent > 0 {
					targetInfo = fmt.Sprintf("agent:%d", pp.TargetAgent)
				}
				log.Infof("[probe_post] agent=%d probe=%d type=%s target=%s",
					aid, pp.ProbeID, pp.Type, targetInfo)

				// Resolve TargetAgent and ProbeAgentID from probe configuration if not already set
				// This handles bidirectional probe direction detection
				if pp.ProbeID != 0 {
					p, err := probe.GetByID(context.TODO(), db, pp.ProbeID)
					if err == nil && p != nil {
						// Always set ProbeAgentID to the probe owner for direction identification
						pp.ProbeAgentID = p.AgentID

						// SESSION INTEGRITY CHECK: Detect if probe is being submitted by wrong agent
						// For NETINFO probes, the submitting agent should match the probe owner
						// (NETINFO is self-reporting, not cross-agent like PING/MTR)
						if pp.Type == probe.TypeNetInfo && p.AgentID != aid {
							log.Errorf("[SESSION_INTEGRITY] NETINFO probe %d owned by agent %d but submitted by agent %d (conn_id=%s, ws=%d)",
								pp.ProbeID, p.AgentID, aid, connID, wsid)
						}

						// Set TargetAgent if not already set and probe has targets
						if pp.TargetAgent == 0 && len(p.Targets) > 0 {
							// Determine direction based on reporting agent
							if aid == p.AgentID && p.Targets[0].AgentID != nil {
								// Forward direction: probe owner reporting, target is Target.AgentID
								pp.TargetAgent = *p.Targets[0].AgentID
							} else if p.Targets[0].AgentID != nil && aid == *p.Targets[0].AgentID {
								// Reverse direction: target agent reporting, target is probe owner
								pp.TargetAgent = p.AgentID
							}
						}
					}
				}

				err := agent.UpdateAgentSeen(context.TODO(), db, pp.AgentID, time.Now())
				if err != nil {
					return err
				}

				if err := probe.Dispatch(context.TODO(), pp); err != nil {
					log.Errorf("probe_post dispatch: %v", err)
					return err
				}

				GetPanelHub().Broadcast(ProbeDataBroadcast{
					WorkspaceID:  wsid,
					ProbeID:      pp.ProbeID,
					AgentID:      pp.AgentID,
					ProbeAgentID: pp.ProbeAgentID,
					TargetAgent:  pp.TargetAgent,
					Type:         string(pp.Type),
					Payload:      pp.Payload,
					CreatedAt:    pp.CreatedAt.Format(time.RFC3339),
					Target:       pp.Target,
					Triggered:    pp.Triggered,
				})

				// Also broadcast to raw WebSocket clients
				GetRawPanelHub().BroadcastRaw(ProbeDataBroadcast{
					WorkspaceID:  wsid,
					ProbeID:      pp.ProbeID,
					AgentID:      pp.AgentID,
					ProbeAgentID: pp.ProbeAgentID,
					TargetAgent:  pp.TargetAgent,
					Type:         string(pp.Type),
					Payload:      pp.Payload,
					CreatedAt:    pp.CreatedAt.Format(time.RFC3339),
					Target:       pp.Target,
					Triggered:    pp.Triggered,
				})

				// Also broadcast to share-link WebSocket clients (keyed by agent ID)
				GetRawShareHub().BroadcastToShare(pp.AgentID, ProbeDataBroadcast{
					WorkspaceID:  wsid,
					ProbeID:      pp.ProbeID,
					AgentID:      pp.AgentID,
					ProbeAgentID: pp.ProbeAgentID,
					TargetAgent:  pp.TargetAgent,
					Type:         string(pp.Type),
					Payload:      pp.Payload,
					CreatedAt:    pp.CreatedAt.Format(time.RFC3339),
					Target:       pp.Target,
					Triggered:    pp.Triggered,
				})

				nsConn.Emit("probe_post_ok", []byte(`{"ok":true}`))
				return nil
			},

			"speedtest_servers": func(nsConn *websocket.NSConn, msg websocket.Message) error {
				ctx := websocket.GetContext(nsConn.Conn)
				aid, ok := ctx.Values().GetUint("agent_id")
				if (ok != nil) || (aid == 0) {
					return errors.New("unauthorized: no agent in context")
				}

				log.Infof("[%s] received speedtest_servers from agent %d", nsConn, aid)

				var servers []speedtest.ServerInput
				if err := json.Unmarshal(msg.Body, &servers); err != nil {
					log.Errorf("speedtest_servers unmarshal: %v", err)
					return err
				}

				if err := speedtest.UpsertServersForAgent(context.TODO(), db, aid, servers); err != nil {
					log.Errorf("speedtest_servers upsert: %v", err)
					return err
				}

				log.Infof("[%s] stored %d speedtest servers for agent %d", nsConn, len(servers), aid)
				nsConn.Emit("speedtest_servers_ok", []byte(`{"ok":true}`))
				return nil
			},

			"speedtest_queue_get": func(nsConn *websocket.NSConn, msg websocket.Message) error {
				ctx := websocket.GetContext(nsConn.Conn)
				aid, ok := ctx.Values().GetUint("agent_id")
				if (ok != nil) || (aid == 0) {
					return errors.New("unauthorized: no agent in context")
				}

				if expired, err := speedtest.ExpirePendingItems(context.TODO(), db); err != nil {
					log.Warnf("speedtest_queue_get: expire error: %v", err)
				} else if expired > 0 {
					log.Infof("speedtest_queue_get: expired %d items", expired)
				}

				a, err := agent.GetAgentByID(context.TODO(), db, aid)
				if err != nil {
					log.Errorf("speedtest_queue_get: get agent: %v", err)
					return err
				}

				if err := agent.UpdateAgentSeen(context.TODO(), db, a.ID, time.Now()); err != nil {
					log.Warnf("speedtest_queue_get: update seen: %v", err)
				}

				items, err := speedtest.ListPendingForAgent(context.TODO(), db, aid)
				if err != nil {
					log.Errorf("speedtest_queue_get: %v", err)
					return err
				}

				payload, err := json.Marshal(items)
				if err != nil {
					return err
				}

				nsConn.Emit("speedtest_queue", payload)
				return nil
			},

			"speedtest_result": func(nsConn *websocket.NSConn, msg websocket.Message) error {
				ctx := websocket.GetContext(nsConn.Conn)
				aid, ok := ctx.Values().GetUint("agent_id")
				if (ok != nil) || (aid == 0) {
					return errors.New("unauthorized: no agent in context")
				}

				log.Infof("[%s] received speedtest_result from agent %d: %s", nsConn, aid, msg.Body)

				var result struct {
					QueueID uint            `json:"queue_id"`
					Success bool            `json:"success"`
					Error   string          `json:"error,omitempty"`
					Data    json.RawMessage `json:"data,omitempty"`
				}
				if err := json.Unmarshal(msg.Body, &result); err != nil {
					log.Errorf("speedtest_result unmarshal: %v", err)
					return err
				}

				if result.Success {
					// Get queue item first - we need workspace info for broadcasting
					queueItem, err := speedtest.GetQueueItem(context.TODO(), db, result.QueueID)
					if err != nil {
						log.Errorf("speedtest_result get queue item %d: %v", result.QueueID, err)
						// Still try to mark completed even if we can't get the queue item
					}

					if err := speedtest.MarkCompleted(context.TODO(), db, result.QueueID); err != nil {
						log.Errorf("speedtest_result mark completed for queue %d: %v", result.QueueID, err)
					} else {
						log.Infof("speedtest_result: marked queue item %d as completed", result.QueueID)
					}

					// Broadcast update to panel clients
					if queueItem != nil {
						GetRawPanelHub().BroadcastSpeedtestUpdate(SpeedtestUpdate{
							QueueID:     result.QueueID,
							WorkspaceID: queueItem.WorkspaceID,
							AgentID:     aid,
							Status:      "completed",
							ServerID:    queueItem.ServerID,
							ServerName:  queueItem.ServerName,
						})
					}

					if len(result.Data) > 0 && queueItem != nil {
						// Look up the actual SPEEDTEST probe for this agent
						probes, err := probe.ListByAgent(context.TODO(), db, aid)
						if err != nil {
							log.Errorf("speedtest_result list probes: %v", err)
							return err
						}

						// Find the SPEEDTEST probe
						var speedtestProbeID uint
						for _, p := range probes {
							if p.Type == probe.TypeSpeedtest {
								speedtestProbeID = p.ID
								break
							}
						}

						if speedtestProbeID == 0 {
							log.Warnf("speedtest_result: no SPEEDTEST probe found for agent %d, using queue ID", aid)
							speedtestProbeID = result.QueueID // Fallback to queue ID if probe not found
						}

						pp := probe.ProbeData{
							Type:       probe.TypeSpeedtest,
							AgentID:    aid,
							ProbeID:    speedtestProbeID,
							Payload:    result.Data,
							CreatedAt:  time.Now(),
							ReceivedAt: time.Now(),
							Target:     queueItem.ServerID,
						}

						if err := probe.Dispatch(context.TODO(), pp); err != nil {
							log.Errorf("speedtest_result dispatch: %v", err)
						}
					}
				} else {
					// Failed test - mark as failed and broadcast
					queueItem, _ := speedtest.GetQueueItem(context.TODO(), db, result.QueueID)

					if err := speedtest.MarkFailed(context.TODO(), db, result.QueueID, result.Error); err != nil {
						log.Errorf("speedtest_result mark failed for queue %d: %v", result.QueueID, err)
					} else {
						log.Infof("speedtest_result: marked queue item %d as failed: %s", result.QueueID, result.Error)
					}

					// Broadcast failure to panel clients
					if queueItem != nil {
						GetRawPanelHub().BroadcastSpeedtestUpdate(SpeedtestUpdate{
							QueueID:     result.QueueID,
							WorkspaceID: queueItem.WorkspaceID,
							AgentID:     aid,
							Status:      "failed",
							Error:       result.Error,
							ServerID:    queueItem.ServerID,
							ServerName:  queueItem.ServerName,
						})
					}
				}

				nsConn.Emit("speedtest_result_ok", []byte(`{"ok":true}`))
				return nil
			},
		},
	}
}

// getPanelWebsocketEvents returns the namespace events for panel connections
func getPanelWebsocketEvents(app *iris.Application, db *gorm.DB, ch *sql.DB) websocket.Namespaces {
	return websocket.Namespaces{
		"panel": websocket.Events{
			websocket.OnNamespaceConnected: func(nsConn *websocket.NSConn, msg websocket.Message) error {
				ctx := websocket.GetContext(nsConn.Conn)
				uid, _ := ctx.Values().GetUint("user_id")
				clientType := ctx.Values().GetString("client_type")

				if clientType != "panel" {
					return errors.New("unauthorized: panel namespace requires JWT auth")
				}

				GetPanelHub().RegisterConnection(nsConn.Conn.ID(), nsConn)

				log.Infof("[panel] user %d connected to panel namespace", uid)
				return nil
			},

			websocket.OnNamespaceDisconnect: func(nsConn *websocket.NSConn, msg websocket.Message) error {
				GetPanelHub().UnregisterConnection(nsConn.Conn.ID())
				log.Infof("[panel] %s disconnected from panel namespace", nsConn.Conn.ID())
				return nil
			},

			"subscribe": func(nsConn *websocket.NSConn, msg websocket.Message) error {
				ctx := websocket.GetContext(nsConn.Conn)
				uid, ok := ctx.Values().GetUint("user_id")
				if ok != nil || uid == 0 {
					return errors.New("unauthorized: no user in context")
				}

				var sub struct {
					WorkspaceID uint `json:"workspace_id"`
					ProbeID     uint `json:"probe_id"`
				}
				if err := json.Unmarshal(msg.Body, &sub); err != nil {
					log.Errorf("[panel] subscribe unmarshal error: %v", err)
					return err
				}

				GetPanelHub().Subscribe(nsConn.Conn.ID(), sub.WorkspaceID, sub.ProbeID)

				nsConn.Emit("subscribe_ok", []byte(`{"ok":true}`))
				log.Infof("[panel] user %d subscribed to workspace %d probe %d", uid, sub.WorkspaceID, sub.ProbeID)
				return nil
			},

			"unsubscribe": func(nsConn *websocket.NSConn, msg websocket.Message) error {
				var sub struct {
					WorkspaceID uint `json:"workspace_id"`
					ProbeID     uint `json:"probe_id"`
				}
				if err := json.Unmarshal(msg.Body, &sub); err != nil {
					log.Errorf("[panel] unsubscribe unmarshal error: %v", err)
					return err
				}

				GetPanelHub().Unsubscribe(nsConn.Conn.ID(), sub.WorkspaceID, sub.ProbeID)

				nsConn.Emit("unsubscribe_ok", []byte(`{"ok":true}`))
				return nil
			},
		},
	}
}
