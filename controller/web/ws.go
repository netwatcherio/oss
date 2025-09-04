package web

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/iris/v12/websocket"
	log "github.com/sirupsen/logrus"
)

// addWebSocketServer wires a namespaced websocket server for agents.
func addWebSocketServer(r *Router) error {
	ws := websocket.New(websocket.DefaultGorillaUpgrader, getWebsocketEvents(r))

	// Authenticate connection using Ed25519-signed headers (no JWT).
	ws.OnConnect = func(c *websocket.Conn) error {
		ctx := websocket.GetContext(c) // iris.Context

		// Required headers
		idStr := strings.TrimSpace(ctx.GetHeader("X-Agent-Id"))
		nonce := strings.TrimSpace(ctx.GetHeader("X-Agent-Nonce"))
		ts := strings.TrimSpace(ctx.GetHeader("X-Agent-Timestamp"))
		sigB64 := strings.TrimSpace(ctx.GetHeader("X-Agent-Signature"))

		if idStr == "" || nonce == "" || ts == "" || sigB64 == "" {
			ctx.StatusCode(http.StatusUnauthorized)
			return errors.New("unauthorized: missing signature headers")
		}
		agentIDu64, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil || agentIDu64 == 0 {
			ctx.StatusCode(http.StatusUnauthorized)
			return errors.New("unauthorized: invalid agent id")
		}
		agentID := uint(agentIDu64)

		// Load agent & public key
		ag, err := r.AgentsRepo.GetByID(ctx, agentID)
		if err != nil || len(ag.PublicKey) == 0 {
			ctx.StatusCode(http.StatusUnauthorized)
			return errors.New("unauthorized: agent not found or missing key")
		}

		// Timestamp skew check
		t, err := time.Parse(time.RFC1123, ts)
		if err != nil || absDuration(time.Since(t)) > 90*time.Second {
			ctx.StatusCode(http.StatusUnauthorized)
			return errors.New("unauthorized: bad timestamp")
		}

		// Canonical string (GET, fixed path, empty body hash)
		h := sha256.Sum256([]byte{})
		canon := strings.Join([]string{
			http.MethodGet,
			"/agent_ws",
			base64.RawStdEncoding.EncodeToString(h[:]),
			t.UTC().Format(time.RFC1123),
			nonce,
		}, "\n")

		// Verify signature
		sig, err := base64.RawStdEncoding.DecodeString(sigB64)
		if err != nil {
			ctx.StatusCode(http.StatusUnauthorized)
			return errors.New("unauthorized: bad signature encoding")
		}
		if !ed25519.Verify(ed25519.PublicKey(ag.PublicKey), []byte(canon), sig) {
			ctx.StatusCode(http.StatusUnauthorized)
			return errors.New("unauthorized: signature verify failed")
		}

		// Single-use nonce (replay prevention)
		if _, err := r.AgentsRepo.UseNonce(ctx, nonce); err != nil {
			ctx.StatusCode(http.StatusUnauthorized)
			return errors.New("unauthorized: nonce invalid/used/expired")
		}

		// Create ephemeral session bound to ws_conn (no JWT needed)
		ip := ctx.Values().GetString("client_ip")
		if _, err := r.AuthSvc.CreateEphemeralAgentSession(ctx, ag.ID, ip, c.ID(), 24*time.Hour); err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			return errors.New("failed to create ws session")
		}

		// Best-effort heartbeat/auth metadata
		_ = r.AgentsRepo.PatchFields(ctx, ag.ID, map[string]any{
			"updated_at":   time.Now(),
			"last_seen_at": time.Now(),
			"last_auth_at": time.Now(),
			"last_auth_ip": ip,
		})

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
				log.Infof("[%s] disconnected from namespace [%s]", nsConn, msg.Namespace)
				return nil
			},

			"probe_get": func(nsConn *websocket.NSConn, msg websocket.Message) error {
				stdCtx := context.Background()

				sess, err := r.AuthSvc.GetSessionFromWSConn(stdCtx, nsConn.Conn.ID())
				if err != nil {
					return err
				}
				if !sess.IsAgent {
					return errors.New("session is not an agent session")
				}

				_ = r.AgentsRepo.PatchFields(stdCtx, sess.ID, map[string]any{
					"updated_at":   time.Now(),
					"last_seen_at": time.Now(),
				})

				p, err := r.ProbesSvc.ListByAgent(stdCtx, sess.ID, true)
				if err != nil {
					log.WithError(err).Error("ListByAgent failed")
					return err
				}
				b, err := json.Marshal(p)
				if err != nil {
					return err
				}
				nsConn.Emit("probe_get", b)
				return nil
			},

			"probe_post": func(nsConn *websocket.NSConn, msg websocket.Message) error {
				stdCtx := context.Background()

				sess, err := r.AuthSvc.GetSessionFromWSConn(stdCtx, nsConn.Conn.ID())
				if err != nil {
					return err
				}
				if !sess.IsAgent {
					return errors.New("session is not an agent session")
				}

				// TODO: parse and forward results if needed
				return nil
			},
		},
	}
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
