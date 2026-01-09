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
	websocketServer := websocket.New(
		websocket.DefaultGorillaUpgrader,
		getWebsocketEvents(app, db, ch),
	)

	// Authenticate connection via PSK (agents) or JWT (panel clients)
	websocketServer.OnConnect = func(c *websocket.Conn) error {
		ctx := websocket.GetContext(c)

		wsIDStr := ctx.GetHeader("X-Workspace-ID")
		agIDStr := ctx.GetHeader("X-Agent-ID")
		psk := ctx.GetHeader("X-Agent-PSK")

		// Try agent authentication first (PSK-based)
		if wsIDStr != "" && agIDStr != "" && psk != "" {
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

			// Mark agent seen
			_ = agent.UpdateAgentSeen(ctx, db, a.ID, time.Now())

			// Stash IDs into the Iris context so namespace handlers can fetch them
			ctx.Values().Set("agent_id", a.ID)
			ctx.Values().Set("workspace_id", a.WorkspaceID)
			ctx.Values().Set("client_type", "agent")

			log.Infof("WS auth ok — agent %d (ws %d) connected as %s", a.ID, a.WorkspaceID, c.ID())
			return nil
		}

		// Try panel authentication (JWT-based)
		authHeader := ctx.GetHeader("Authorization")
		if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			u, sess, err := users.GetUserFromToken(context.Background(), db, token)
			if err != nil {
				ctx.StatusCode(http.StatusUnauthorized)
				return errors.New("unauthorized: invalid jwt token")
			}

			// Store user info in context
			ctx.Values().Set("user_id", u.ID)
			ctx.Values().Set("session_id", sess.SessionID)
			ctx.Values().Set("client_type", "panel")

			log.Infof("WS auth ok — panel user %d (session %d) connected as %s", u.ID, sess.SessionID, c.ID())
			return nil
		}

		// No valid auth provided
		ctx.StatusCode(http.StatusUnauthorized)
		return errors.New("unauthorized: missing authentication (X-Agent-PSK or Authorization header)")
	}

	app.Get("ws", websocket.Handler(websocketServer))
	return nil
}

func getWebsocketEvents(app *iris.Application, db *gorm.DB, ch *sql.DB) websocket.Namespaces {
	serverEvents := websocket.Namespaces{
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

				// Important: nsConn.Emit returns bool; do not treat as error
				nsConn.Emit("version", []byte("ok"))
				return nil
			},

			"probe_get": func(nsConn *websocket.NSConn, msg websocket.Message) error {
				ctx := websocket.GetContext(nsConn.Conn)
				aid, ok := ctx.Values().GetUint("agent_id")
				if (ok != nil) || (aid == 0) {
					return errors.New("unauthorized: no agent in context")
				}

				// Load and update agent
				a, err := agent.GetAgentByID(context.TODO(), db, aid)
				if err != nil {
					log.Error(err)
					return err
				}
				if err := agent.UpdateAgentSeen(context.TODO(), db, a.ID, time.Now()); err != nil {
					log.Error(err)
				}

				// Fetch probes for this agent
				// NOTE: Adjust your Probe struct if needed; this mirrors your previous logic
				ownedP, err := probe.ListForAgent(context.TODO(), db, ch, a.ID)
				if err != nil {
					log.Errorf("probe_get: %v", err)
				}

				payload, err := json.Marshal(ownedP)
				if err != nil {
					return err
				}

				// Important: nsConn.Emit returns bool; do not treat as error
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

				// Unmarshal as ProbeData (top-level fields + payload)
				log.Infof("[%s] posted message to namespace [%s]: %s", nsConn, msg.Namespace, msg.Body)

				var pp probe.ProbeData
				if err := json.Unmarshal(msg.Body, &pp); err != nil {
					log.Error(err)
					return err
				}

				// Force/augment meta from the authenticated context
				pp.AgentID = aid // reporting agent ID
				if pp.CreatedAt.IsZero() {
					pp.CreatedAt = time.Now()
				}
				pp.ReceivedAt = time.Now()

				err := agent.UpdateAgentSeen(context.TODO(), db, pp.AgentID, time.Now())
				if err != nil {
					return err
				}

				// Dispatch to the registered handler for pp.Kind (or AGENT-derived)
				if err := probe.Dispatch(context.TODO(), pp); err != nil {
					log.Errorf("probe_post dispatch: %v", err)
					return err
				}

				// Broadcast to subscribed panel clients
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

				// Optionally ACK
				nsConn.Emit("probe_post_ok", []byte(`{"ok":true}`))
				return nil
			},

			// ================== Speedtest Events ==================

			// speedtest_servers: Agent submits available speedtest.net servers
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

			// speedtest_queue_get: Agent requests pending speedtest items
			"speedtest_queue_get": func(nsConn *websocket.NSConn, msg websocket.Message) error {
				ctx := websocket.GetContext(nsConn.Conn)
				aid, ok := ctx.Values().GetUint("agent_id")
				if (ok != nil) || (aid == 0) {
					return errors.New("unauthorized: no agent in context")
				}

				// Expire old pending items first
				if expired, err := speedtest.ExpirePendingItems(context.TODO(), db); err != nil {
					log.Warnf("speedtest_queue_get: expire error: %v", err)
				} else if expired > 0 {
					log.Infof("speedtest_queue_get: expired %d items", expired)
				}

				// Check if agent is online (seen within last 2 minutes)
				// If LastSeenAt is zero or before threshold, the agent making this request is clearly online
				a, err := agent.GetAgentByID(context.TODO(), db, aid)
				if err != nil {
					log.Errorf("speedtest_queue_get: get agent: %v", err)
					return err
				}

				// Update LastSeenAt since agent is clearly online if making this request
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

			// speedtest_result: Agent submits completed speedtest result
			"speedtest_result": func(nsConn *websocket.NSConn, msg websocket.Message) error {
				ctx := websocket.GetContext(nsConn.Conn)
				aid, ok := ctx.Values().GetUint("agent_id")
				if (ok != nil) || (aid == 0) {
					return errors.New("unauthorized: no agent in context")
				}

				log.Infof("[%s] received speedtest_result from agent %d: %s", nsConn, aid, msg.Body)

				// Unmarshal the result which includes queue_id and the speedtest data
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
					// Mark queue item as completed
					if err := speedtest.MarkCompleted(context.TODO(), db, result.QueueID); err != nil {
						log.Errorf("speedtest_result mark completed: %v", err)
					}

					// Dispatch the speedtest data to ClickHouse via existing probe handler
					if len(result.Data) > 0 {
						// Get queue item to find the probe ID (or create one if needed)
						queueItem, err := speedtest.GetQueueItem(context.TODO(), db, result.QueueID)
						if err != nil {
							log.Errorf("speedtest_result get queue item: %v", err)
							return err
						}

						pp := probe.ProbeData{
							Type:       probe.TypeSpeedtest,
							AgentID:    aid,
							ProbeID:    result.QueueID, // Use queue ID as probe ID for tracing
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
					// Mark as failed
					if err := speedtest.MarkFailed(context.TODO(), db, result.QueueID, result.Error); err != nil {
						log.Errorf("speedtest_result mark failed: %v", err)
					}
				}

				nsConn.Emit("speedtest_result_ok", []byte(`{"ok":true}`))
				return nil
			},
		},

		// ================== Panel Namespace ==================
		// Panel clients connect with JWT auth and can subscribe to real-time probe data
		"panel": websocket.Events{
			websocket.OnNamespaceConnected: func(nsConn *websocket.NSConn, msg websocket.Message) error {
				ctx := websocket.GetContext(nsConn.Conn)
				uid, _ := ctx.Values().GetUint("user_id")
				clientType := ctx.Values().GetString("client_type")

				if clientType != "panel" {
					return errors.New("unauthorized: panel namespace requires JWT auth")
				}

				// Register connection with the panel hub
				GetPanelHub().RegisterConnection(nsConn.Conn.ID(), nsConn)

				log.Infof("[panel] user %d connected to panel namespace", uid)
				return nil
			},

			websocket.OnNamespaceDisconnect: func(nsConn *websocket.NSConn, msg websocket.Message) error {
				// Unregister connection from the panel hub (cleans up all subscriptions)
				GetPanelHub().UnregisterConnection(nsConn.Conn.ID())
				log.Infof("[panel] %s disconnected from panel namespace", nsConn.Conn.ID())
				return nil
			},

			// subscribe: Panel client subscribes to probe updates
			// Payload: { "workspace_id": uint, "probe_id": uint (optional, 0 = all) }
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

				// TODO: Verify user has access to workspace (for now, trust JWT)
				GetPanelHub().Subscribe(nsConn.Conn.ID(), sub.WorkspaceID, sub.ProbeID)

				nsConn.Emit("subscribe_ok", []byte(`{"ok":true}`))
				log.Infof("[panel] user %d subscribed to workspace %d probe %d", uid, sub.WorkspaceID, sub.ProbeID)
				return nil
			},

			// unsubscribe: Panel client unsubscribes from probe updates
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

	return serverEvents
}
