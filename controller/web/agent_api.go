package web

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"netwatcher-controller/internal/agent"
)

// ---------- Request/Response DTOs ----------

type agentChallengeReq struct {
	WorkspaceID uint   `json:"workspaceId"`
	ID          uint   `json:"id"`
	PIN         string `json:"pin"`
}
type agentChallengeResp struct {
	AgentID   uint      `json:"agentId"`
	Nonce     string    `json:"nonce"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type agentRegisterKeyReq struct {
	WorkspaceID     uint   `json:"workspaceId"`
	ID              uint   `json:"id"`
	PIN             string `json:"pin"`
	PublicKeyBase64 string `json:"publicKeyBase64Raw"` // base64url-raw of 32B Ed25519 public key
	SignatureBase64 string `json:"signatureBase64Raw"` // base64url-raw of 64B signature over Nonce
	Nonce           string `json:"nonce"`
	AgentVersion    string `json:"version,omitempty"` // optional
}

func addRouteAgentAPI(r *Router) []*Route {
	var routes []*Route

	// ----- Bootstrap: challenge -----
	routes = append(routes, &Route{
		Name: "Agent Challenge",
		Path: "/agent/challenge",
		Type: RouteType_POST,
		Func: func(ctx iris.Context) error {
			ctx.ContentType("application/json")
			var in agentChallengeReq
			if err := ctx.ReadJSON(&in); err != nil {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}
			out, err := r.AgentsSvc.CreateChallenge(ctx, agent.ChallengeInput{
				WorkspaceID: in.WorkspaceID,
				AgentID:     in.ID,
				PIN:         in.PIN,
			})
			if err != nil {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}
			return ctx.JSON(agentChallengeResp{
				AgentID:   out.AgentID,
				Nonce:     out.Nonce,
				ExpiresAt: out.ExpiresAt,
			})
		},
	})

	// ----- Bootstrap: register public key -----
	routes = append(routes, &Route{
		Name: "Agent Register Key",
		Path: "/agent/register-key",
		Type: RouteType_POST,
		Func: func(ctx iris.Context) error {
			ctx.ContentType("application/json")
			var in agentRegisterKeyReq
			if err := ctx.ReadJSON(&in); err != nil {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}
			pub, err := base64.RawStdEncoding.DecodeString(in.PublicKeyBase64)
			if err != nil || len(pub) != ed25519.PublicKeySize {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}
			sig, err := base64.RawStdEncoding.DecodeString(in.SignatureBase64)
			if err != nil || len(sig) != ed25519.SignatureSize {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}

			ag, err := r.AgentsSvc.RegisterKey(ctx, agent.RegisterKeyInput{
				WorkspaceID: in.WorkspaceID,
				AgentID:     in.ID,
				PIN:         in.PIN,
				PublicKey:   pub,
				Nonce:       in.Nonce,
				Signature:   sig,
			})
			if err != nil {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}

			// Best-effort heartbeat & version update
			ip := ctx.Values().GetString("client_ip")
			_ = touchAgentHeartbeat(ctx, r.DB, r.AgentsRepo, ag.ID, ip)
			if in.AgentVersion != "" {
				_ = r.AgentsRepo.PatchFields(ctx, ag.ID, map[string]any{
					"version":    in.AgentVersion,
					"updated_at": time.Now(),
				})
			}

			return ctx.JSON(ag)
		},
	})

	// ----- Signed endpoint: get probes (no JWT) -----
	routes = append(routes, &Route{
		Name: "Agent API - Get Probes (KeyAuth)",
		Path: "/agent/probes",
		Type: RouteType_GET,
		// Verify Ed25519 signature on every request
		Middlewares: []iris.Handler{
			KeyAuth{Repo: r.AgentsRepo, Skew: 90 * time.Second}.Middleware,
		},
		Func: func(ctx iris.Context) error {
			ctx.ContentType("application/json")

			ag, ok := ctx.Values().Get("agent").(*agent.Agent)
			if !ok || ag == nil {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}

			// Get persisted + reverse/virtual probes
			prs, err := r.ProbesSvc.ListByAgent(ctx, ag.ID, true /* includeReverse */)
			if err != nil {
				ctx.StatusCode(http.StatusInternalServerError)
				return nil
			}

			// Optionally: keep agent “last seen” fresh on fetch
			if err := touchAgentHeartbeat(ctx, r.DB, r.AgentsRepo, ag.ID, ctx.Values().GetString("client_ip")); err != nil {
				log.WithError(err).Warn("failed to refresh heartbeat on probe fetch")
			}

			// Respond as-is; agents know how to run based on Type/Targets
			return ctx.JSON(prs)
		},
	})

	return routes
}

// ----- KeyAuth middleware (Ed25519 signed requests) -----

type KeyAuth struct {
	Repo agent.Repository
	Skew time.Duration // allowed clock skew
}

func (ka KeyAuth) Middleware(ctx iris.Context) {
	// Required headers
	idStr := ctx.GetHeader("X-Agent-Id")
	nonce := ctx.GetHeader("X-Agent-Nonce")
	ts := ctx.GetHeader("X-Agent-Timestamp")
	sigB64 := ctx.GetHeader("X-Agent-Signature")

	if idStr == "" || nonce == "" || ts == "" || sigB64 == "" {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}
	agentID, err := parseUint(idStr)
	if err != nil {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}

	// Load agent & public key
	ag, err := ka.Repo.GetByID(ctx, uint(agentID))
	if err != nil || len(ag.PublicKey) == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}

	// Timestamp skew check
	t, err := time.Parse(time.RFC1123, ts)
	if err != nil || absDuration(time.Since(t)) > ka.Skew {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}

	// Read body (restore afterwards)
	var bodyBytes []byte
	if ctx.Request().Body != nil {
		bodyBytes, _ = io.ReadAll(ctx.Request().Body)
		ctx.Request().Body = io.NopCloser(strings.NewReader(string(bodyBytes)))
	}
	h := sha256.Sum256(bodyBytes)

	// Canonical string
	canon := strings.Join([]string{
		ctx.Method(),
		ctx.Path(),
		base64.RawStdEncoding.EncodeToString(h[:]),
		t.UTC().Format(time.RFC1123),
		nonce,
	}, "\n")

	// Verify signature
	sig, err := base64.RawStdEncoding.DecodeString(sigB64)
	if err != nil {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}
	if !ed25519.Verify(ed25519.PublicKey(ag.PublicKey), []byte(canon), sig) {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}

	// Single-use nonce (replay prevent). If not found/expired/used -> 401.
	if _, err := ka.Repo.UseNonce(ctx, nonce); err != nil {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}

	// Touch auth metadata (best-effort)
	_ = ka.Repo.PatchFields(ctx, ag.ID, map[string]any{
		"last_auth_at": time.Now(),
		"last_auth_ip": ctx.Values().GetString("client_ip"),
	})

	// Pass agent to the handler
	ctx.Values().Set("agent", ag)
	ctx.Next()
}

// ----- helpers -----

func parseUint(s string) (uint64, error) { return strconv.ParseUint(s, 10, 64) }

func touchAgentHeartbeat(ctx iris.Context, db *gorm.DB, repo agent.Repository, agentID uint, ip string) error {
	// Update UpdatedAt/LastSeenAt + best-effort auth IP (if provided)
	fields := map[string]any{
		"last_seen_at": time.Now(),
		"updated_at":   time.Now(),
	}
	if ip != "" {
		fields["last_auth_ip"] = ip
	}
	return repo.PatchFields(ctx, agentID, fields)
}

func ensureAgentInitialized(ctx iris.Context, db *gorm.DB, repo agent.Repository, agentID uint) error {
	ag, err := repo.GetByID(ctx, agentID)
	if err != nil {
		return err
	}
	if ag.Initialized {
		return nil
	}
	return repo.PatchFields(ctx, agentID, map[string]any{"initialized": true})
}
