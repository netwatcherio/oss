// web/analysis_api.go
// REST API endpoints for AI health analysis
package web

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"netwatcher-controller/internal/probe"
)

func panelAnalysis(api fiber.Router, pg *gorm.DB, ch *sql.DB) {
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
