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

	"github.com/gofiber/fiber/v2"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// agentAPIMiddleware validates agent PSK from headers.
// Expects X-Workspace-ID, X-Agent-ID, and X-Agent-PSK headers.
func agentAPIMiddleware(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		workspaceIDStr := c.Get("X-Workspace-ID")
		agentIDStr := c.Get("X-Agent-ID")
		psk := c.Get("X-Agent-PSK")

		if workspaceIDStr == "" || agentIDStr == "" || psk == "" {
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "missing auth headers"})
		}

		workspaceID, err := strconv.ParseUint(workspaceIDStr, 10, 64)
		if err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid workspace_id"})
		}

		agentID, err := strconv.ParseUint(agentIDStr, 10, 64)
		if err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid agent_id"})
		}

		// Authenticate with PSK
		a, err := agent.AuthenticateWithPSK(c.UserContext(), db, uint(workspaceID), uint(agentID), psk)
		if err != nil {
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "invalid_psk"})
		}

		// Store agent in context for downstream handlers
		c.Locals("agent", a)
		c.Locals("agent_id", a.ID)
		c.Locals("workspace_id", a.WorkspaceID)

		// Update last seen
		if err := agent.UpdateAgentSeen(c.UserContext(), db, a.ID, time.Now()); err != nil {
			log.WithError(err).Warn("failed to update agent last_seen")
		}

		return c.Next()
	}
}

// fiberClientIP extracts the real client IP from a Fiber context using the shared lookup logic.
func fiberClientIP(c *fiber.Ctx) string {
	return lookup.GetClientIPFromHeaders(c.IP(), c.Get("X-Forwarded-For"), c.Get("X-Real-IP"))
}

// RegisterAgentAPI registers agent-specific API endpoints.
// These endpoints use PSK-based authentication (header: X-Agent-PSK).
func RegisterAgentAPI(api fiber.Router, db *gorm.DB, ch *sql.DB, geoStore *geoip.Store) {
	// All routes under /agent/api require PSK auth
	agentAPI := api.Group("/agent/api", agentAPIMiddleware(db))

	// GET /agent/api/whoami - Returns the agent's public IP as seen by the controller
	// This allows agents to discover their public IP without external services.
	agentAPI.Get("/whoami", func(c *fiber.Ctx) error {
		// CRITICAL: Prevent caching by reverse proxies (Caddy, Nginx, Cloudflare)
		// This endpoint must always return the unique IP of the current requestor.
		// Caching here causes "IP Confusion" where Agent B gets Agent A's cached IP.
		c.Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		c.Set("Pragma", "no-cache")

		// Debug logging for header diagnosis
		remoteAddr := c.IP()
		xForwardedFor := c.Get("X-Forwarded-For")
		xRealIP := c.Get("X-Real-IP")
		cfConnectingIP := c.Get("CF-Connecting-IP")

		log.WithFields(log.Fields{
			"remote_addr":      remoteAddr,
			"x_forwarded_for":  xForwardedFor,
			"x_real_ip":        xRealIP,
			"cf_connecting_ip": cfConnectingIP,
		}).Debug("whoami request headers")

		clientIP := fiberClientIP(c)
		log.WithFields(log.Fields{
			"resolved_ip":    clientIP,
			"geo_store_nil":  geoStore == nil,
			"clickhouse_nil": ch == nil,
		}).Info("whoami request received")

		// Quick response with just IP (minimal latency)
		// Check if quick=true
		quickParam := c.Query("quick")
		if quickParam == "true" || quickParam == "1" {
			return c.JSON(lookup.QuickLookupByIP(clientIP))
		}

		// Full response with GeoIP enrichment
		result, err := lookup.UnifiedLookup(c.UserContext(), ch, geoStore, clientIP)
		if err != nil {
			log.WithError(err).Warn("whoami lookup failed")
			// Still return the IP even if enrichment fails
			return c.JSON(fiber.Map{
				"ip":        clientIP,
				"timestamp": time.Now(),
				"error":     err.Error(),
			})
		}

		// Debug: log what we're returning
		hasGeoIP := result.GeoIP != nil
		var hasCity, hasASN bool
		if hasGeoIP {
			hasCity = result.GeoIP.City != nil
			hasASN = result.GeoIP.ASN != nil
		}
		log.WithFields(log.Fields{
			"ip":          clientIP,
			"has_geoip":   hasGeoIP,
			"has_city":    hasCity,
			"has_asn":     hasASN,
			"reverse_dns": result.ReverseDNS,
		}).Info("whoami response")

		return c.JSON(result)
	})

	// GET /agent/api/lookup/ip/:ip - Lookup GeoIP/PTR for any IP
	// Useful for agents that want to enrich hop data locally
	agentAPI.Get("/lookup/ip/:ip", func(c *fiber.Ctx) error {
		ip := c.Params("ip")
		if ip == "" {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "ip parameter required"})
		}

		result, err := lookup.UnifiedLookup(c.UserContext(), ch, geoStore, ip)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(result)
	})
}
