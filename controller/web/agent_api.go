// web/agent_api.go - Agent-specific API endpoints (PSK authenticated)
package web

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"netwatcher-controller/internal/agent"
	"netwatcher-controller/internal/geoip"
	"netwatcher-controller/internal/lookup"

	"github.com/kataras/iris/v12"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// agentAPIMiddleware validates agent PSK from headers.
// Expects X-Workspace-ID, X-Agent-ID, and X-Agent-PSK headers.
func agentAPIMiddleware(db *gorm.DB) iris.Handler {
	return func(ctx iris.Context) {
		workspaceIDStr := ctx.GetHeader("X-Workspace-ID")
		agentIDStr := ctx.GetHeader("X-Agent-ID")
		psk := ctx.GetHeader("X-Agent-PSK")

		if workspaceIDStr == "" || agentIDStr == "" || psk == "" {
			ctx.StatusCode(http.StatusUnauthorized)
			_ = ctx.JSON(iris.Map{"error": "missing auth headers"})
			return
		}

		workspaceID, err := strconv.ParseUint(workspaceIDStr, 10, 64)
		if err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "invalid workspace_id"})
			return
		}

		agentID, err := strconv.ParseUint(agentIDStr, 10, 64)
		if err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "invalid agent_id"})
			return
		}

		// Authenticate with PSK
		a, err := agent.AuthenticateWithPSK(ctx.Request().Context(), db, uint(workspaceID), uint(agentID), psk)
		if err != nil {
			ctx.StatusCode(http.StatusUnauthorized)
			_ = ctx.JSON(iris.Map{"error": "invalid_psk"})
			return
		}

		// Store agent in context for downstream handlers
		ctx.Values().Set("agent", a)
		ctx.Values().Set("agent_id", a.ID)
		ctx.Values().Set("workspace_id", a.WorkspaceID)

		// Update last seen
		if err := agent.UpdateAgentSeen(ctx.Request().Context(), db, a.ID, time.Now()); err != nil {
			log.WithError(err).Warn("failed to update agent last_seen")
		}

		ctx.Next()
	}
}

// RegisterAgentAPI registers agent-specific API endpoints.
// These endpoints use PSK-based authentication (header: X-Agent-PSK).
func RegisterAgentAPI(api iris.Party, db *gorm.DB, ch *sql.DB, geoStore *geoip.Store) {
	// All routes under /agent/api require PSK auth
	agentAPI := api.Party("/agent/api", agentAPIMiddleware(db))

	// GET /agent/api/whoami - Returns the agent's public IP as seen by the controller
	// This allows agents to discover their public IP without external services.
	agentAPI.Get("/whoami", func(ctx iris.Context) {
		clientIP := lookup.GetClientIP(ctx)

		// Quick response with just IP (minimal latency)
		if ctx.URLParamExists("quick") {
			_ = ctx.JSON(lookup.QuickLookup(ctx))
			return
		}

		// Full response with GeoIP enrichment
		result, err := lookup.UnifiedLookup(ctx.Request().Context(), ch, geoStore, clientIP)
		if err != nil {
			log.WithError(err).Warn("whoami lookup failed")
			// Still return the IP even if enrichment fails
			_ = ctx.JSON(iris.Map{
				"ip":        clientIP,
				"timestamp": time.Now(),
				"error":     err.Error(),
			})
			return
		}

		_ = ctx.JSON(result)
	})

	// GET /agent/api/lookup/ip/{ip} - Lookup GeoIP/PTR for any IP
	// Useful for agents that want to enrich hop data locally
	agentAPI.Get("/lookup/ip/{ip:string}", func(ctx iris.Context) {
		ip := ctx.Params().Get("ip")
		if ip == "" {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "ip parameter required"})
			return
		}

		result, err := lookup.UnifiedLookup(ctx.Request().Context(), ch, geoStore, ip)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		_ = ctx.JSON(result)
	})
}
