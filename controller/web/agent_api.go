package web

import (
	"net/http"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"netwatcher-controller/internal/agent"
	"netwatcher-controller/internal/auth"
)

// Small response types so agents get exactly what they need.
type agentLoginReq struct {
	PIN          string `json:"pin"`
	ID           uint   `json:"id"`      // GORM PK
	AgentVersion string `json:"version"` // optional
}
type agentLoginResp struct {
	Token string       `json:"token"`
	Data  *agent.Agent `json:"data"`
}

func addRouteAgentAPI(r *Router) []*Route {
	var routes []*Route

	// POST /agent/login
	routes = append(routes, &Route{
		Name: "Agent API Login",
		Path: "/agent/login",
		JWT:  false,
		Type: RouteType_POST,
		Func: func(ctx iris.Context) error {
			ctx.ContentType("application/json")

			var in agentLoginReq
			if err := ctx.ReadJSON(&in); err != nil {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}

			ip := ctx.Values().GetString("client_ip")

			// Go through auth service (creates session, issues JWT, best-effort version bump)
			token, ag, err := r.AuthSvc.AgentLogin(ctx, auth.AgentLogin{
				PIN:          in.PIN,
				ID:           in.ID,
				AgentVersion: in.AgentVersion,
				IP:           ip,
			})
			if err != nil {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}

			// Heartbeat/initialize best-effort (don’t fail login if these fail)
			if err := touchAgentHeartbeat(ctx, r.DB, r.AgentsRepo, ag.ID, ip); err != nil {
				log.WithError(err).Warn("failed to update agent heartbeat")
			}
			if err := ensureAgentInitialized(ctx, r.DB, r.AgentsRepo, ag.ID); err != nil {
				log.WithError(err).Warn("failed to initialize agent")
			}

			resp := agentLoginResp{Token: token, Data: ag}
			err = ctx.JSON(resp)
			return err
		},
	})

	// GET /agent/probes
	// Returns the *complete* list of probes for this agent, including reverse/meta expansions,
	// with targets resolved enough for execution (host/ip[:port]) when possible.
	routes = append(routes, &Route{
		Name: "Agent API - Get Probes",
		Path: "/agent/probes",
		JWT:  true,
		Type: RouteType_GET,
		Func: func(ctx iris.Context) error {
			ctx.ContentType("application/json")

			token := bearer(ctx)
			if token == "" {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}

			ag, sess, err := r.AuthSvc.GetAgentFromJWT(ctx, token, r.DB)
			if err != nil {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}
			_ = sess // available if you want to refresh expiry, etc.

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

			// Respond as-is; agents know how to run based on Type/Server/Targets
			err = ctx.JSON(prs)
			return err
		},
	})

	return routes
}

// ----- helpers -----

func bearer(ctx iris.Context) string {
	authz := ctx.GetHeader("Authorization")
	if authz == "" {
		return ""
	}
	if !strings.HasPrefix(strings.ToLower(authz), "bearer ") {
		return ""
	}
	return strings.TrimSpace(authz[7:])
}

func touchAgentHeartbeat(ctx iris.Context, db *gorm.DB, repo agent.Repository, agentID uint, ip string) error {
	// Update UpdatedAt/LastSeenAt/IP (repo.PatchFields keeps it simple)
	return repo.PatchFields(ctx, agentID, map[string]any{
		"last_seen_at": time.Now(),
		"updated_at":   time.Now(),
		"last_ip":      ip,
	})
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
