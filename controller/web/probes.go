// web/probes.go
package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"netwatcher-controller/internal/limits"
	"netwatcher-controller/internal/probe"
	"netwatcher-controller/internal/workspace"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func panelProbes(api fiber.Router, db *gorm.DB, limitsConfig *limits.Config) {
	base := api.Group("/workspaces/:id/agents/:agentID/probes")
	wsStore := workspace.NewStore(db)

	// Apply workspace access check to all probe routes
	base.Use(RequireWorkspaceAccess(wsStore))

	// GET /workspaces/:id/agents/:agentID/probes - requires CanView (any member)
	base.Get("/", func(c *fiber.Ctx) error {
		aID := uintParam(c, "agentID")
		list, err := probe.ListByAgent(c.UserContext(), db, aID)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(NewListResponse(list))
	})

	// POST /workspaces/:id/agents/:agentID/probes - requires CanEdit (USER+)
	base.Post("/", RequireRole(wsStore, CanEdit), func(c *fiber.Ctx) error {
		aID := uintParam(c, "agentID")
		var input probe.CreateInput

		if err := c.BodyParser(&input); err != nil {
			return c.SendStatus(http.StatusBadRequest)
		}

		// Check agent probe limit
		if err := limits.CanAddProbe(c.UserContext(), db, limitsConfig, aID); err != nil {
			if errors.Is(err, limits.ErrProbeLimitReached) {
				return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
			}
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		p, err := probe.Create(c.UserContext(), db, input)
		if err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(http.StatusCreated).JSON(p)
	})

	// /workspaces/:id/agents/:agentID/probes/:probeID
	pid := base.Group("/:probeID")

	// GET /workspaces/:id/agents/:agentID/probes/:probeID - requires CanView (any member)
	pid.Get("/", func(c *fiber.Ctx) error {
		id := uintParam(c, "probeID")
		p, err := probe.GetByID(c.UserContext(), db, id)
		if err != nil || p == nil {
			return c.SendStatus(http.StatusNotFound)
		}
		return c.JSON(p)
	})

	// PATCH /workspaces/:id/agents/:agentID/probes/:probeID - requires CanEdit (USER+)
	pid.Patch("/", RequireRole(wsStore, CanEdit), func(c *fiber.Ctx) error {
		id := uintParam(c, "probeID")
		var body struct {
			Enabled             *bool           `json:"enabled"`
			IntervalSec         *int            `json:"interval_sec"`
			TimeoutSec          *int            `json:"timeout_sec"`
			Count               *int            `json:"count"`
			DurationSec         *int            `json:"duration_sec"`
			BindInterface       *string         `json:"bind_interface"`
			Labels              *map[string]any `json:"labels"`
			Metadata            *map[string]any `json:"metadata"`
			ReplaceTargets      []string        `json:"replaceTargets"`
			ReplaceAgentTargets []uint          `json:"replaceAgentTargets"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.SendStatus(http.StatusBadRequest)
		}
		in := probe.UpdateInput{
			ID:                  id,
			Enabled:             body.Enabled,
			IntervalSec:         body.IntervalSec,
			TimeoutSec:          body.TimeoutSec,
			Count:               body.Count,
			DurationSec:         body.DurationSec,
			BindInterface:       body.BindInterface,
			Labels:              jsonPtrFromMap(body.Labels),
			Metadata:            jsonPtrFromMap(body.Metadata),
			ReplaceTargets:      body.ReplaceTargets,
			ReplaceAgentTargets: body.ReplaceAgentTargets,
		}
		p, err := probe.Update(c.UserContext(), db, in)
		if err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(p)
	})

	// DELETE /workspaces/:id/agents/:agentID/probes/:probeID - requires CanEdit (USER+)
	pid.Delete("/", RequireRole(wsStore, CanEdit), func(c *fiber.Ctx) error {
		id := uintParam(c, "probeID")
		if err := probe.Delete(c.UserContext(), db, id); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// -------------------- Workspace-Level Probe Operations --------------------
	// These endpoints operate across agents within a workspace

	wsProbes := api.Group("/workspaces/:id/probes")
	wsProbes.Use(RequireWorkspaceAccess(wsStore))

	// GET /workspaces/:id/probes/matching?source={agentID}&dest={agentID,agentID,...}&types={TYPE,TYPE,...}
	// Find probes from source agent that target the specified destination agents
	wsProbes.Get("/matching", func(c *fiber.Ctx) error {
		sourceIDInt, _ := strconv.Atoi(c.Query("source"))
		sourceID := uint(sourceIDInt)
		destIDsStr := c.Query("dest")
		typesStr := c.Query("types")

		if sourceID == 0 || destIDsStr == "" {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "source and dest query params required"})
		}

		// Parse dest agent IDs
		var destIDs []uint
		for _, s := range strings.Split(destIDsStr, ",") {
			if id, err := strconv.ParseUint(strings.TrimSpace(s), 10, 32); err == nil {
				destIDs = append(destIDs, uint(id))
			}
		}

		if len(destIDs) == 0 {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "dest must contain valid agent IDs"})
		}

		// Parse probe types (optional)
		var probeTypes []probe.Type
		if typesStr != "" {
			for _, s := range strings.Split(typesStr, ",") {
				probeTypes = append(probeTypes, probe.Type(strings.TrimSpace(s)))
			}
		}

		matches, err := probe.FindMatchingProbes(c.UserContext(), db, sourceID, destIDs, probeTypes)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(NewListResponse(matches))
	})

	// POST /workspaces/:id/probes/copy - Copy probes between agents
	// Requires CanEdit (USER+) permission
	wsProbes.Post("/copy", RequireRole(wsStore, CanEdit), func(c *fiber.Ctx) error {
		wsID := uintParam(c, "id")

		var input probe.CopyInput
		if err := c.BodyParser(&input); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid input: " + err.Error()})
		}

		// Set workspace ID from route
		input.WorkspaceID = wsID

		// Validation
		if input.SourceAgentID == 0 {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "source_agent_id required"})
		}
		if len(input.DestAgentIDs) == 0 {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "dest_agent_ids required"})
		}

		result, err := probe.CopyProbes(c.UserContext(), db, input)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(result)
	})
}
