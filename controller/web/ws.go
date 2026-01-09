package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"netwatcher-controller/internal/agent"
	probe "netwatcher-controller/internal/probe"
	"netwatcher-controller/internal/speedtest"
	"netwatcher-controller/internal/users"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"

	_ "github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/websocket"
	log "github.com/sirupsen/logrus"
)

// Expected headers for WS:
//   X-Workspace-ID: <uint>
//   X-Agent-ID:     <uint>
//   X-Agent-PSK:    <string>
//
// If valid, we attach agent/workspace IDs to the Iris context for use in events.

func addWebSocketServer(app *iris.Application, db *gorm.DB, ch *sql.DB) error {
	// Agent WebSocket server - uses PSK header auth
	agentWsServer := websocket.New(
		websocket.DefaultGorillaUpgrader,
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
			ctx.StatusCode(http.StatusUnauthorized)
			return errors.New("unauthorized: invalid psk")
		}

		_ = agent.UpdateAgentSeen(ctx, db, a.ID, time.Now())
		ctx.Values().Set("agent_id", a.ID)
		ctx.Values().Set("workspace_id", a.WorkspaceID)
		ctx.Values().Set("client_type", "agent")

		log.Infof("WS auth ok — agent %d (ws %d) connected as %s", a.ID, a.WorkspaceID, c.ID())
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
				log.Infof("[%s] connected to namespace [%s] (agent=%d ws=%d)", nsConn, msg.Namespace, aid, wsid)
				return nil
			},
			websocket.OnNamespaceDisconnect: func(nsConn *websocket.NSConn, msg websocket.Message) error {
				log.Infof("[%s] disconnected from namespace [%s]", nsConn, msg.Namespace)
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

				log.Infof("[%s] posted message to namespace [%s]: %s", nsConn, msg.Namespace, msg.Body)

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

				err := agent.UpdateAgentSeen(context.TODO(), db, pp.AgentID, time.Now())
				if err != nil {
					return err
				}

				if err := probe.Dispatch(context.TODO(), pp); err != nil {
					log.Errorf("probe_post dispatch: %v", err)
					return err
				}

				GetPanelHub().Broadcast(ProbeDataBroadcast{
					WorkspaceID: wsid,
					ProbeID:     pp.ProbeID,
					AgentID:     pp.AgentID,
					Type:        string(pp.Type),
					Payload:     pp.Payload,
					CreatedAt:   pp.CreatedAt.Format(time.RFC3339),
					Target:      pp.Target,
					Triggered:   pp.Triggered,
				})

				// Also broadcast to raw WebSocket clients
				GetRawPanelHub().BroadcastRaw(ProbeDataBroadcast{
					WorkspaceID: wsid,
					ProbeID:     pp.ProbeID,
					AgentID:     pp.AgentID,
					Type:        string(pp.Type),
					Payload:     pp.Payload,
					CreatedAt:   pp.CreatedAt.Format(time.RFC3339),
					Target:      pp.Target,
					Triggered:   pp.Triggered,
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
					if err := speedtest.MarkCompleted(context.TODO(), db, result.QueueID); err != nil {
						log.Errorf("speedtest_result mark completed: %v", err)
					}

					if len(result.Data) > 0 {
						queueItem, err := speedtest.GetQueueItem(context.TODO(), db, result.QueueID)
						if err != nil {
							log.Errorf("speedtest_result get queue item: %v", err)
							return err
						}

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
					if err := speedtest.MarkFailed(context.TODO(), db, result.QueueID, result.Error); err != nil {
						log.Errorf("speedtest_result mark failed: %v", err)
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
