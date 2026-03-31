// web/alerts.go
package web

import (
	"database/sql"
	"net/http"
	"strconv"

	"netwatcher-controller/internal/alert"
	"netwatcher-controller/internal/workspace"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func panelAlerts(api fiber.Router, db *gorm.DB, ch *sql.DB) {
	wsStore := workspace.NewStore(db)

	// Global alert endpoints (across all user's workspaces)
	alerts := api.Group("/alerts")

	// GET /alerts - List all alerts for user's workspaces
	alerts.Get("/", func(c *fiber.Ctx) error {
		userID := getUserID(c)
		if userID == 0 {
			return c.SendStatus(http.StatusUnauthorized)
		}

		// Get user's workspace IDs
		workspaceIDs, err := getUserWorkspaceIDs(c.UserContext(), db, userID)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		if len(workspaceIDs) == 0 {
			return c.JSON(NewListResponse([]alert.Alert{}))
		}

		// Get status filter
		var statusFilter *alert.Status
		if s := c.Query("status"); s != "" {
			status := alert.Status(s)
			statusFilter = &status
		}

		limit := 100
		if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
			limit = l
		}

		var allAlerts []alert.Alert
		for _, wsID := range workspaceIDs {
			a, err := alert.ListAlerts(c.UserContext(), db, &wsID, statusFilter, limit)
			if err != nil {
				continue
			}
			allAlerts = append(allAlerts, a...)
		}

		return c.JSON(NewListResponse(allAlerts))
	})

	// GET /alerts/count - Get count of active alerts
	alerts.Get("/count", func(c *fiber.Ctx) error {
		userID := getUserID(c)
		if userID == 0 {
			return c.SendStatus(http.StatusUnauthorized)
		}

		workspaceIDs, err := getUserWorkspaceIDs(c.UserContext(), db, userID)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		count, err := alert.CountActiveAlerts(c.UserContext(), db, workspaceIDs)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{"count": count})
	})

	// GET /alerts/:id - Get single alert
	alerts.Get("/:id", func(c *fiber.Ctx) error {
		id := uintParam(c, "id")
		a, err := alert.GetAlertByID(c.UserContext(), db, id)
		if err != nil {
			return c.SendStatus(http.StatusNotFound)
		}
		return c.JSON(a)
	})

	// PATCH /alerts/:id/acknowledge - Acknowledge alert
	alerts.Patch("/:id/acknowledge", func(c *fiber.Ctx) error {
		id := uintParam(c, "id")
		userID := getUserID(c)

		if err := alert.AcknowledgeAlert(c.UserContext(), db, id, userID); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// PATCH /alerts/:id/resolve - Resolve alert
	alerts.Patch("/:id/resolve", func(c *fiber.Ctx) error {
		id := uintParam(c, "id")

		if err := alert.ResolveAlert(c.UserContext(), db, id); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// -------------------- Alert Rules (per workspace) --------------------
	rules := api.Group("/workspaces/:id/alert-rules")
	rules.Use(RequireWorkspaceAccess(wsStore))

	// GET /workspaces/:id/alert-rules - List rules for workspace
	rules.Get("/", func(c *fiber.Ctx) error {
		wsID := uintParam(c, "id")
		list, err := alert.ListRulesByWorkspace(c.UserContext(), db, wsID)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(NewListResponse(list))
	})

	// POST /workspaces/:id/alert-rules - Create rule (requires CanEdit)
	rules.Post("/", RequireRole(wsStore, CanEdit), func(c *fiber.Ctx) error {
		wsID := uintParam(c, "id")
		var input alert.CreateRuleInput
		if err := c.BodyParser(&input); err != nil {
			return c.SendStatus(http.StatusBadRequest)
		}
		input.WorkspaceID = wsID

		rule, err := alert.CreateRule(c.UserContext(), db, input)
		if err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}

		return c.Status(http.StatusCreated).JSON(rule)
	})

	// GET /workspaces/:id/alert-rules/:ruleID - Get single rule
	rules.Get("/:ruleID", func(c *fiber.Ctx) error {
		ruleID := uintParam(c, "ruleID")
		rule, err := alert.GetRuleByID(c.UserContext(), db, ruleID)
		if err != nil {
			return c.SendStatus(http.StatusNotFound)
		}
		return c.JSON(rule)
	})

	// PATCH /workspaces/:id/alert-rules/:ruleID - Update rule (requires CanEdit)
	rules.Patch("/:ruleID", RequireRole(wsStore, CanEdit), func(c *fiber.Ctx) error {
		ruleID := uintParam(c, "ruleID")
		var input alert.UpdateRuleInput
		if err := c.BodyParser(&input); err != nil {
			return c.SendStatus(http.StatusBadRequest)
		}
		input.ID = ruleID

		rule, err := alert.UpdateRule(c.UserContext(), db, input)
		if err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(rule)
	})

	// DELETE /workspaces/:id/alert-rules/:ruleID - Delete rule (requires CanManage)
	rules.Delete("/:ruleID", RequireRole(wsStore, CanManage), func(c *fiber.Ctx) error {
		ruleID := uintParam(c, "ruleID")
		if err := alert.DeleteRule(c.UserContext(), db, ruleID); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// GET /workspaces/:id/probes/:probeID/baseline - Get baseline stats for a probe
	api.Get("/workspaces/:id/probes/:probeID/baseline", func(c *fiber.Ctx) error {
		probeID := uintParam(c, "probeID")
		metric := alert.Metric(c.Query("metric", "latency"))
		windowDays := 7
		if wd, err := strconv.Atoi(c.Query("window", "7")); err == nil && wd > 0 {
			windowDays = wd
		}

		if ch == nil {
			return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{"error": "ClickHouse not available"})
		}

		stats, err := alert.GetProbeBaseline(c.UserContext(), ch, probeID, metric, windowDays)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		if stats.Count == 0 {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "No data available for baseline"})
		}
		return c.JSON(stats)
	})
}
