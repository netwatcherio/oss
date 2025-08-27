package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kataras/iris/v12/websocket"
	log "github.com/sirupsen/logrus"
	"netwatcher-controller/internal/probe"
)

// addWebSocketServer wires a namespaced websocket server for agents.
func addWebSocketServer(r *Router) error {
	ws := websocket.New(websocket.DefaultGorillaUpgrader, getWebsocketEvents(r))

	// Authenticate connection with Bearer JWT and bind ws_conn to the session.
	ws.OnConnect = func(c *websocket.Conn) error {
		ctx := websocket.GetContext(c) // iris.Context

		authz := strings.TrimSpace(ctx.GetHeader("Authorization"))
		if authz == "" || !strings.HasPrefix(authz, "Bearer ") {
			ctx.StatusCode(http.StatusUnauthorized)
			return errors.New("unauthorized: missing bearer token")
		}
		raw := strings.TrimPrefix(authz, "Bearer ")

		// Optional: quick signature check so we can return 401 fast if the key is wrong.
		// (Service will validate again while parsing claims/lookup session.)
		_, err := jwt.Parse(raw, func(tok *jwt.Token) (interface{}, error) {
			return []byte(os.Getenv("KEY")), nil
		})
		if err != nil {
			ctx.StatusCode(http.StatusUnauthorized)
			return errors.New("unauthorized: invalid token")
		}

		// Resolve agent + session from token using auth service
		ag, sess, err := r.AuthSvc.GetAgentFromJWT(ctx, raw, r.DB)
		if err != nil {
			ctx.StatusCode(http.StatusUnauthorized)
			return errors.New("unauthorized: invalid agent session")
		}

		// Bind ws connection id to the session (best-effort)
		if err := r.AuthSvc.UpdateWSConn(ctx, sess.SessionID, c.ID()); err != nil {
			// Don't block connection; log and continue
			log.WithError(err).Warn("failed to update ws_conn on session")
		}

		// Best-effort heartbeat/last seen
		if r.AgentsRepo != nil {
			_ = r.AgentsRepo.PatchFields(ctx, ag.ID, map[string]any{
				"updated_at":   time.Now(),
				"last_seen_at": time.Now(),
			})
		}

		log.Infof("Agent %d connected via ws [%s]", ag.ID, c.ID())
		return nil
	}

	r.WebSocketServer = ws
	return nil
}

func getWebsocketEvents(r *Router) websocket.Namespaces {
	return websocket.Namespaces{
		"agent": websocket.Events{
			websocket.OnNamespaceConnected: func(nsConn *websocket.NSConn, msg websocket.Message) error {
				log.Infof("[%s] connected to namespace [%s]", nsConn, msg.Namespace)
				return nil
			},
			websocket.OnNamespaceDisconnect: func(nsConn *websocket.NSConn, msg websocket.Message) error {
				// Optionally: mark as offline or clear ws_conn if it matches
				log.Infof("[%s] disconnected from namespace [%s]", nsConn, msg.Namespace)
				return nil
			},

			// Agent asks for its assigned probes (including reverse/expanded).
			"probe_get": func(nsConn *websocket.NSConn, msg websocket.Message) error {
				// iris.Context -> implements stdlib Context via Deadline/Done? We'll wrap.
				stdCtx := context.Background()

				// Look up session by ws_conn
				sess, err := r.AuthSvc.GetSessionFromWSConn(stdCtx, nsConn.Conn.ID())
				if err != nil {
					return err
				}
				if !sess.IsAgent {
					return errors.New("session is not an agent session")
				}

				// Best-effort heartbeat/last seen
				if r.AgentsRepo != nil {
					_ = r.AgentsRepo.PatchFields(stdCtx, sess.ID, map[string]any{
						"updated_at":   time.Now(),
						"last_seen_at": time.Now(),
					})
				}

				// Fetch probes for agent
				var probes []probe.Probe
				p, err := r.ProbesSvc.ListByAgent(stdCtx, sess.ID, true)
				if err != nil {
					log.WithError(err).Error("ListForAgent failed")
					return err
				}
				probes = p

				b, err := json.Marshal(probes)
				if err != nil {
					return err
				}
				nsConn.Emit("probe_get", b)
				return nil
			},

			// Agent posts probe results (streamed to worker channel).
			"probe_post": func(nsConn *websocket.NSConn, msg websocket.Message) error {
				stdCtx := context.Background()

				// Validate session again via ws_conn
				sess, err := r.AuthSvc.GetSessionFromWSConn(stdCtx, nsConn.Conn.ID())
				if err != nil {
					return err
				}
				if !sess.IsAgent {
					return errors.New("session is not an agent session")
				}

				// Optional: ensure agent still exists
				/*if r.Agents != nil {
					if _, err := r.Agents.GetByID(stdCtx, sess.ID); err != nil {
						// log and still accept data (or reject)
						log.WithError(err).Warnf("agent %d not found for probe_post", sess.ID)
					}
				}

				var data agent.ProbeData
				if err := json.Unmarshal(msg.Body, &data); err != nil {
					log.WithError(err).Error("invalid probe_post payload")
					return err
				}*/

				// Stream to worker (non-blocking if you prefer; here we block)
				/*if r.ProbeDataChan != nil {
					r.ProbeDataChan <- data
				} else {
					log.Warn("ProbeDataChan is nil; dropping probe data")
				}*/

				return nil
			},
		},
	}
}

// small helper to detect the extended repo interface without hard dependency
func hasListWithReverse(repo probe.Repository) bool {
	type withReverse interface {
		ListForAgentWithReverse(ctx context.Context, agentID uint, limit, offset int) ([]probe.Probe, int64, error)
	}
	_, ok := interface{}(repo).(withReverse)
	return ok
}
