// web/agents.go
package web

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"netwatcher-controller/internal/limits"
	"netwatcher-controller/internal/probe"
	"netwatcher-controller/internal/workspace"
	"time"

	"netwatcher-controller/internal/agent"

	"github.com/gofiber/fiber/v2"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func panelAgents(api fiber.Router, db *gorm.DB, ch *sql.DB, limitsConfig *limits.Config) {
	ws := api.Group("/workspaces/:id")
	wsStore := workspace.NewStore(db)

	// Apply workspace access check to all agent routes
	as := ws.Group("/agents")
	as.Use(RequireWorkspaceAccess(wsStore))

	// GET /workspaces/{id}/agents
	as.Get("/", func(c *fiber.Ctx) error {
		wsID := uintParam(c, "id")
		limit := intParam(c, "limit", 50, 1, 200)
		offset := intParam(c, "offset", 0, 0, 1_000_000)
		list, total, err := agent.ListAgentsByWorkspace(c.UserContext(), db, wsID, limit, offset)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": list, "total": total, "limit": limit, "offset": offset})
	})

	// POST /workspaces/{id}/agents - requires CanEdit (USER+)
	as.Post("/", RequireRole(wsStore, CanEdit), func(c *fiber.Ctx) error {
		wsID := uintParam(c, "id")
		var body struct {
			Name              string         `json:"name"`
			Description       string         `json:"description"`
			Location          string         `json:"location"`
			PublicIPOverride  string         `json:"public_ip_override"`
			Version           string         `json:"version"`
			PinLength         int            `json:"pinLength"`
			PinTTLSeconds     int            `json:"pinTTLSeconds"`
			Labels            map[string]any `json:"labels"`
			Metadata          map[string]any `json:"metadata"`
			TrafficSimEnabled *bool          `json:"trafficsim_enabled"`
			TrafficSimHost    string         `json:"trafficsim_host"`
			TrafficSimPort    int            `json:"trafficsim_port"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.SendStatus(http.StatusBadRequest)
		}
		var ttl *time.Duration
		if body.PinTTLSeconds > 0 {
			d := time.Duration(body.PinTTLSeconds) * time.Second
			ttl = &d
		}

		// Check workspace agent limit
		if err := limits.CanAddAgent(c.UserContext(), db, limitsConfig, wsID); err != nil {
			if errors.Is(err, limits.ErrAgentLimitReached) {
				return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
			}
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		out, err := agent.CreateAgent(c.UserContext(), db, agent.CreateInput{
			WorkspaceID:      wsID,
			Name:             body.Name,
			Description:      body.Description,
			PinLength:        body.PinLength,
			Location:         body.Location,
			PublicIPOverride: body.PublicIPOverride,
			Version:          body.Version,
			Labels:           jsonFromMap(body.Labels),
			Metadata:         jsonFromMap(body.Metadata),
			PINTTL:           ttl,
		})
		if err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(http.StatusCreated).JSON(out)
	})

	// /workspaces/{id}/agents/{agentID}
	aid := as.Group("/:agentID")

	// GET /workspaces/{id}/agents/{agentID}
	aid.Get("/", func(c *fiber.Ctx) error {
		wsID := uintParam(c, "id")
		aID := uintParam(c, "agentID")
		a, err := agent.GetAgentByWorkspaceAndID(c.UserContext(), db, wsID, aID)
		if err != nil || a == nil {
			return c.SendStatus(http.StatusNotFound)
		}
		return c.JSON(a)
	})

	aid.Get("/netinfo", func(c *fiber.Ctx) error {
		aID := uintParam(c, "agentID")
		a, err := probe.GetLatestNetInfoForAgent(context.TODO(), ch, uint64(aID), nil)
		if err != nil || a == nil {
			return c.SendStatus(http.StatusNotFound)
		}
		return c.JSON(a)
	})

	aid.Get("/sysinfo", func(c *fiber.Ctx) error {
		aID := uintParam(c, "agentID")
		a, err := probe.GetLatestSysInfoForAgent(context.TODO(), ch, uint64(aID), nil)
		if err != nil || a == nil {
			return c.SendStatus(http.StatusNotFound)
		}
		return c.JSON(a)
	})

	// PATCH /workspaces/{id}/agents/{agentID} - requires CanEdit (USER+)
	aid.Patch("/", RequireRole(wsStore, CanEdit), func(c *fiber.Ctx) error {
		aID := uintParam(c, "agentID")
		var body struct {
			Name              *string         `json:"name"`
			Description       *string         `json:"description"`
			Location          *string         `json:"location"`
			PublicIPOverride  *string         `json:"public_ip_override"`
			Version           *string         `json:"version"`
			Labels            *map[string]any `json:"labels"`
			Metadata          *map[string]any `json:"metadata"`
			TrafficSimEnabled *bool           `json:"trafficsim_enabled"`
			TrafficSimHost    *string         `json:"trafficsim_host"`
			TrafficSimPort    *int            `json:"trafficsim_port"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.SendStatus(http.StatusBadRequest)
		}
		patch := map[string]any{}
		if body.Name != nil {
			patch["name"] = *body.Name
		}
		if body.Description != nil {
			patch["description"] = *body.Description
		}
		if body.Location != nil {
			patch["location"] = *body.Location
		}
		if body.PublicIPOverride != nil {
			patch["public_ip_override"] = *body.PublicIPOverride
		}
		if body.Version != nil {
			patch["version"] = *body.Version
		}
		if body.Labels != nil {
			patch["labels"] = jsonFromMap(*body.Labels)
		}
		if body.Metadata != nil {
			patch["metadata"] = jsonFromMap(*body.Metadata)
		}
		if body.TrafficSimEnabled != nil {
			patch["trafficsim_enabled"] = *body.TrafficSimEnabled
		}
		if body.TrafficSimHost != nil {
			patch["trafficsim_host"] = *body.TrafficSimHost
		}
		if body.TrafficSimPort != nil {
			patch["trafficsim_port"] = *body.TrafficSimPort
		}

		if err := agent.PatchAgentFields(c.UserContext(), db, aID, patch); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		a, _ := agent.GetAgentByID(c.UserContext(), db, aID)
		return c.JSON(a)
	})

	// DELETE /workspaces/{id}/agents/{agentID} - requires CanManage (ADMIN+)
	aid.Delete("/", RequireRole(wsStore, CanManage), func(c *fiber.Ctx) error {
		aID := uintParam(c, "agentID")

		// Send deactivation message to connected agent BEFORE deletion
		// This ensures the agent receives the message while still authenticated
		if GetAgentHub().DeactivateAgent(aID, "deleted") {
			log.Infof("Sent deactivation to connected agent %d before deletion", aID)
		}

		if err := agent.DeleteAgent(c.UserContext(), db, aID); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// POST /workspaces/{id}/agents/{agentID}/heartbeat
	aid.Post("/heartbeat", func(c *fiber.Ctx) error {
		aID := uintParam(c, "agentID")
		now := time.Now()
		if err := agent.UpdateAgentSeen(c.UserContext(), db, aID, now); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true, "ts": now})
	})

	// POST /workspaces/{id}/agents/{agentID}/issue-pin - requires CanEdit (USER+)
	aid.Post("/issue-pin", RequireRole(wsStore, CanEdit), func(c *fiber.Ctx) error {
		wsID := uintParam(c, "id")
		aID := uintParam(c, "agentID")
		var body struct {
			PinLength  int `json:"pinLength"`
			TTLSeconds int `json:"ttlSeconds"`
		}
		_ = c.BodyParser(&body)
		var ttl *time.Duration
		if body.TTLSeconds > 0 {
			d := time.Duration(body.TTLSeconds) * time.Second
			ttl = &d
		}
		pin, err := agent.IssuePIN(c.UserContext(), db, wsID, aID, ifZero(body.PinLength, 9), ttl)
		if err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"pin": pin})
	})

	// POST /workspaces/{id}/agents/{agentID}/regenerate - requires CanManage (ADMIN+)
	// Invalidates existing PSK (disconnecting any connected agent), marks agent as uninitialized,
	// and issues a new PIN for reinstallation on a different machine.
	aid.Post("/regenerate", RequireRole(wsStore, CanManage), func(c *fiber.Ctx) error {
		wsID := uintParam(c, "id")
		aID := uintParam(c, "agentID")
		var body struct {
			PinLength  int `json:"pinLength"`
			TTLSeconds int `json:"ttlSeconds"`
		}
		_ = c.BodyParser(&body)

		// 1) Send deactivation signal to connected agent BEFORE invalidating PSK
		if GetAgentHub().DeactivateAgent(aID, "regenerated") {
			log.Infof("Sent deactivation to connected agent %d before regeneration", aID)
			// Brief pause to allow the deactivate message to be delivered
			time.Sleep(500 * time.Millisecond)
		}

		// 2) Clear the PSK hash - this invalidates any existing sessions
		if err := agent.PatchAgentFields(c.UserContext(), db, aID, map[string]any{
			"psk_hash":    "",    // Clear PSK - invalidates existing sessions
			"initialized": false, // Mark as not initialized - requires bootstrap
		}); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}

		// 2) Issue a new PIN for reinstallation
		var ttl *time.Duration
		if body.TTLSeconds > 0 {
			d := time.Duration(body.TTLSeconds) * time.Second
			ttl = &d
		}
		pin, err := agent.IssuePIN(c.UserContext(), db, wsID, aID, ifZero(body.PinLength, 9), ttl)
		if err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}

		// 3) Get updated agent info
		a, _ := agent.GetAgentByID(c.UserContext(), db, aID)

		return c.JSON(fiber.Map{
			"pin":   pin,
			"agent": a,
		})
	})

	// GET /workspaces/{id}/agents/{agentID}/pending-pin - requires CanEdit (USER+)
	aid.Get("/pending-pin", RequireRole(wsStore, CanEdit), func(c *fiber.Ctx) error {
		wsID := uintParam(c, "id")
		aID := uintParam(c, "agentID")

		// Check if agent is already initialized
		a, err := agent.GetAgentByWorkspaceAndID(c.UserContext(), db, wsID, aID)
		if err != nil || a == nil {
			return c.SendStatus(http.StatusNotFound)
		}
		if a.Initialized {
			// Agent already bootstrapped - no PIN to show
			return c.JSON(fiber.Map{"pin": "", "initialized": true})
		}

		pin, err := agent.GetPendingPIN(c.UserContext(), db, wsID, aID)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"pin": pin, "initialized": false})
	})
}
