// web/analysis_api.go
// REST API endpoints for AI health analysis
package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"netwatcher-controller/internal/geoip"
	"netwatcher-controller/internal/probe"
)

func panelAnalysis(api fiber.Router, pg *gorm.DB, ch *sql.DB, geoStore *geoip.Store) {
	// ------------------------------------------
	// GET /workspaces/:id/analysis
	// Workspace health overview with per-agent health vectors
	// Query: lookback=<minutes, default 60>
	// ------------------------------------------
	api.Get("/workspaces/:id/analysis", func(c *fiber.Ctx) error {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[analysis] PANIC: %v", r)
				_ = c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
			}
		}()

		wID := uintParam(c, "id")
		lookback := intOrDefault(c.Query("lookback"), 60)

		analysis, err := probe.ComputeWorkspaceAnalysis(c.UserContext(), ch, pg, wID, lookback)
		if err != nil {
			log.Printf("[analysis] workspace=%d error: %v", wID, err)
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		jsonBytes, err := json.Marshal(analysis)
		if err != nil {
			log.Printf("[analysis] JSON marshal error: %v", err)
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "json serialization failed"})
		}

		c.Set("Content-Type", "application/json")
		return c.Send(jsonBytes)
	})

	// ------------------------------------------
	// GET /workspaces/:id/analysis/probes/:probeId
	// Detailed probe analysis with bidirectional data
	// Query: lookback=<minutes, default 60>
	// ------------------------------------------
	api.Get("/workspaces/:id/analysis/probes/:probeId", func(c *fiber.Ctx) error {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[analysis] probe PANIC: %v", r)
				_ = c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
			}
		}()

		wID := uintParam(c, "id")
		probeID := uintParam(c, "probeId")
		lookback := intOrDefault(c.Query("lookback"), 60)

		analysis, err := probe.ComputeProbeAnalysis(c.UserContext(), ch, pg, wID, probeID, lookback)
		if err != nil {
			log.Printf("[analysis] workspace=%d probe=%d error: %v", wID, probeID, err)
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		jsonBytes, err := json.Marshal(analysis)
		if err != nil {
			log.Printf("[analysis] probe JSON marshal error: %v", err)
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "json serialization failed"})
		}

		c.Set("Content-Type", "application/json")
		return c.Send(jsonBytes)
	})

	// ------------------------------------------
	// GET /workspaces/:id/analysis/routes
	// Route/path analysis for cross-agent route comparison and divergence detection
	// Query: lookback=<hours, default 24>
	// ------------------------------------------
	api.Get("/workspaces/:id/analysis/routes", func(c *fiber.Ctx) error {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[analysis] routes PANIC: %v", r)
				_ = c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
			}
		}()

		wID := uintParam(c, "id")
		lookbackHours := intOrDefault(c.Query("lookback"), 24)

		// Bound the compute so a slow ClickHouse query never blows past
		// the Fiber WriteTimeout (30s) and gets the connection killed
		// mid-response. Returning 504 lets the client surface a real
		// error instead of a hung spinner.
		ctx, cancel := context.WithTimeout(c.UserContext(), 25*time.Second)
		defer cancel()

		// The probe package accepts a nil geoStore and skips ASN grouping.
		// We pass the real store when available so we get shared-ASN data.
		var geoResolver probe.GeoIPResolver
		if geoStore != nil {
			geoResolver = geoStoreAdapter{geoStore}
		}
		analysis, err := probe.ComputeWorkspaceRouteAnalysis(ctx, ch, pg, geoResolver, wID, lookbackHours)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				log.Printf("[analysis] routes workspace=%d timeout", wID)
				return c.Status(http.StatusGatewayTimeout).JSON(fiber.Map{"error": "route analysis timed out"})
			}
			log.Printf("[analysis] routes workspace=%d error: %v", wID, err)
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		jsonBytes, err := json.Marshal(analysis)
		if err != nil {
			log.Printf("[analysis] routes JSON marshal error: %v", err)
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "json serialization failed"})
		}

		c.Set("Content-Type", "application/json")
		return c.Send(jsonBytes)
	})

	// ------------------------------------------
	// GET /workspaces/:id/analysis/history
	// Historical analysis snapshots for trend analysis
	// Query: from=<RFC3339>, to=<RFC3339>, limit=<int, default 288>
	// ------------------------------------------
	api.Get("/workspaces/:id/analysis/history", func(c *fiber.Ctx) error {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[analysis] history PANIC: %v", r)
				_ = c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
			}
		}()

		wID := uintParam(c, "id")
		limit := intOrDefault(c.Query("limit"), 288)

		var from, to time.Time
		if v := c.Query("from"); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				from = t
			}
		}
		if v := c.Query("to"); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				to = t
			}
		}

		// Default to last 24 hours if no from specified
		if from.IsZero() {
			from = time.Now().UTC().Add(-24 * time.Hour)
		}

		snapshots, err := probe.GetAnalysisSnapshots(c.UserContext(), ch, wID, from, to, limit)
		if err != nil {
			log.Printf("[analysis] history workspace=%d error: %v", wID, err)
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		jsonBytes, err := json.Marshal(fiber.Map{
			"workspace_id": wID,
			"snapshots":    snapshots,
			"count":        len(snapshots),
		})
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "json serialization failed"})
		}

		c.Set("Content-Type", "application/json")
		return c.Send(jsonBytes)
	})
}

// geoStoreAdapter wraps *geoip.Store to satisfy probe.GeoIPResolver. We can't
// import the probe package's interface from inside geoip, and the probe
// package can't import geoip (cycle risk), so the conversion lives at the
// wiring layer (web).
type geoStoreAdapter struct{ s *geoip.Store }

func (g geoStoreAdapter) HasASN() bool { return g.s.HasASN() }
func (g geoStoreAdapter) LookupASN(ipStr string) (uint, string, bool) {
	info, err := g.s.LookupASN(ipStr)
	if err != nil || info == nil {
		return 0, "", false
	}
	return info.Number, info.Organization, true
}
