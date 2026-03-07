// web/share.go
package web

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"netwatcher-controller/internal/agent"
	"netwatcher-controller/internal/probe"
	"netwatcher-controller/internal/share"
	"netwatcher-controller/internal/workspace"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// -------------------- Protected Endpoints (JWT auth) --------------------

// panelShareLinks registers share link management endpoints for authenticated users.
func panelShareLinks(api fiber.Router, db *gorm.DB) {
	// Create share link for an agent
	api.Post("/workspaces/:id/agents/:agentID/share-links", func(c *fiber.Ctx) error {
		workspaceID := uintParam(c, "id")
		agentID := uintParam(c, "agentID")

		userID := getUserID(c)
		if userID == 0 {
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}

		// Verify user has access to workspace
		if !fiberHasWorkspaceAccess(c, db, workspaceID, userID) {
			return nil // response already sent
		}

		// Verify agent belongs to workspace
		_, err := agent.GetAgentByWorkspaceAndID(c.UserContext(), db, workspaceID, agentID)
		if err != nil {
			if errors.Is(err, agent.ErrNotFound) {
				return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
			}
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		// Parse request body
		var body struct {
			ExpiresInSeconds int    `json:"expires_in_seconds"`
			Password         string `json:"password,omitempty"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}

		// Default to 24 hours if not specified
		expiresIn := time.Duration(body.ExpiresInSeconds) * time.Second
		if expiresIn <= 0 {
			expiresIn = 24 * time.Hour
		}
		// Cap at 30 days
		if expiresIn > 30*24*time.Hour {
			expiresIn = 30 * 24 * time.Hour
		}

		// Create share link
		output, err := share.Create(c.UserContext(), db, share.CreateInput{
			WorkspaceID:     workspaceID,
			AgentID:         agentID,
			CreatedByUserID: userID,
			ExpiresIn:       expiresIn,
			Password:        body.Password,
		})
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.Status(http.StatusCreated).JSON(output)
	})

	// List share links for an agent
	api.Get("/workspaces/:id/agents/:agentID/share-links", func(c *fiber.Ctx) error {
		workspaceID := uintParam(c, "id")
		agentID := uintParam(c, "agentID")

		userID := getUserID(c)
		if userID == 0 {
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}

		// Verify user has access to workspace
		if !fiberHasWorkspaceAccess(c, db, workspaceID, userID) {
			return nil // response already sent
		}

		links, err := share.ListByAgent(c.UserContext(), db, workspaceID, agentID)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{"items": links, "total": len(links)})
	})

	// Delete (revoke) a share link
	api.Delete("/workspaces/:id/agents/:agentID/share-links/:linkID", func(c *fiber.Ctx) error {
		workspaceID := uintParam(c, "id")
		agentID := uintParam(c, "agentID")
		linkID := uintParam(c, "linkID")

		userID := getUserID(c)
		if userID == 0 {
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}

		// Verify user has access to workspace
		if !fiberHasWorkspaceAccess(c, db, workspaceID, userID) {
			return nil // response already sent
		}

		err := share.Delete(c.UserContext(), db, workspaceID, agentID, linkID)
		if err != nil {
			if errors.Is(err, share.ErrShareLinkNotFound) {
				return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "share link not found"})
			}
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.SendStatus(http.StatusNoContent)
	})
}

// fiberHasWorkspaceAccess checks if the user has access to the workspace.
func fiberHasWorkspaceAccess(c *fiber.Ctx, db *gorm.DB, workspaceID, userID uint) bool {
	store := workspace.NewStore(db)
	_, err := store.GetMemberByUserID(c.UserContext(), workspaceID, userID)
	if err != nil {
		if errors.Is(err, workspace.ErrNotFound) {
			_ = c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
			return false
		}
		_ = c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		return false
	}
	return true
}

// -------------------- Public Endpoints (no auth) --------------------

// RegisterShareRoutes registers public share link access endpoints.
func RegisterShareRoutes(app *fiber.App, db *gorm.DB, ch *sql.DB) {
	shareAPI := app.Group("/share")

	// Get shared agent info (validates token and optional password)
	shareAPI.Get("/:token", func(c *fiber.Ctx) error {
		token := c.Params("token")
		password := c.Query("password")

		// Validate share link
		link, err := share.Validate(c.UserContext(), db, share.ValidateInput{
			Token:    token,
			Password: password,
		})
		if err != nil {
			return fiberHandleShareError(c, err)
		}

		// Record access
		_ = share.RecordAccess(c.UserContext(), db, link.ID)

		// Get agent info
		ag, err := agent.GetAgentByID(c.UserContext(), db, link.AgentID)
		if err != nil {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
		}

		// Determine public IP - prefer actual NETINFO public_address over override
		publicIP := ag.PublicIPOverride
		if ch != nil {
			netInfoData, err := probe.GetLatestNetInfoForAgent(c.UserContext(), ch, uint64(link.AgentID), nil)
			if err == nil && netInfoData != nil && netInfoData.Payload != nil {
				// Parse the netinfo payload to extract public_address
				var netPayload struct {
					PublicAddress string `json:"public_address"`
				}
				if json.Unmarshal(netInfoData.Payload, &netPayload) == nil && netPayload.PublicAddress != "" {
					publicIP = netPayload.PublicAddress
				}
			}
		}

		// Get owned probes AND reverse probes (from other agents targeting this one)
		owned, reverse, err := probe.ListByAgentWithReverse(c.UserContext(), db, link.AgentID)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch probes"})
		}

		// Filter to enabled probes only
		var probes []probe.Probe
		for _, p := range owned {
			if p.Enabled {
				probes = append(probes, p)
			}
		}
		for _, p := range reverse {
			if p.Enabled {
				probes = append(probes, p)
			}
		}

		// Return limited agent info (no secrets)
		return c.JSON(fiber.Map{
			"agent": fiber.Map{
				"id":           ag.ID,
				"name":         ag.Name,
				"description":  ag.Description,
				"location":     ag.Location,
				"version":      ag.Version,
				"public_ip":    publicIP,
				"initialized":  ag.Initialized,
				"updated_at":   ag.UpdatedAt,
				"last_seen_at": ag.LastSeenAt,
			},
			"probes":          probes,
			"reverse_count":   len(reverse), // Number of probes from other agents targeting this one
			"expires_at":      link.ExpiresAt,
			"allow_speedtest": link.AllowSpeedtest,
		})
	})

	// Check if share link requires password (no password needed for this check)
	shareAPI.Get("/:token/info", func(c *fiber.Ctx) error {
		token := c.Params("token")

		link, err := share.GetByToken(c.UserContext(), db, token)
		if err != nil {
			if errors.Is(err, share.ErrShareLinkNotFound) {
				return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "share link not found"})
			}
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		// Check if expired
		expired := time.Now().After(link.ExpiresAt)

		return c.JSON(fiber.Map{
			"has_password":    link.HasPassword,
			"expired":         expired,
			"expires_at":      link.ExpiresAt,
			"allow_speedtest": link.AllowSpeedtest,
		})
	})

	// Get agent name for shared context (sanitized - only returns name)
	// Only works for agents that are part of probes visible to this share link
	shareAPI.Get("/:token/agent/:agentID", func(c *fiber.Ctx) error {
		token := c.Params("token")
		agentID := uintParam(c, "agentID")
		password := c.Query("password")

		// Validate share link
		link, err := share.Validate(c.UserContext(), db, share.ValidateInput{
			Token:    token,
			Password: password,
		})
		if err != nil {
			return fiberHandleShareError(c, err)
		}

		// Check if this agent is accessible from the shared agent's probes
		// Either: the agent IS the shared agent, OR is targeted by/targets the shared agent
		isAccessible := false

		// Check 1: Is this the shared agent itself?
		if agentID == link.AgentID {
			isAccessible = true
		}

		// Check 2: Is this agent targeted by a probe owned by the shared agent?
		if !isAccessible {
			var count int64
			db.WithContext(c.UserContext()).
				Table("probe_targets").
				Joins("JOIN probes ON probes.id = probe_targets.probe_id").
				Where("probes.agent_id = ? AND probe_targets.agent_id = ?", link.AgentID, agentID).
				Count(&count)
			if count > 0 {
				isAccessible = true
			}
		}

		// Check 3: Does this agent have a probe that targets the shared agent?
		if !isAccessible {
			var count int64
			db.WithContext(c.UserContext()).
				Table("probe_targets").
				Joins("JOIN probes ON probes.id = probe_targets.probe_id").
				Where("probes.agent_id = ? AND probe_targets.agent_id = ?", agentID, link.AgentID).
				Count(&count)
			if count > 0 {
				isAccessible = true
			}
		}

		if !isAccessible {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
		}

		// Get agent and return only safe fields
		ag, err := agent.GetAgentByID(c.UserContext(), db, agentID)
		if err != nil {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
		}

		// Return only name (and optionally location for context)
		return c.JSON(fiber.Map{
			"id":       ag.ID,
			"name":     ag.Name,
			"location": ag.Location,
		})
	})

	// Get probe data for shared agent
	shareAPI.Get("/:token/probe-data/:probeID", func(c *fiber.Ctx) error {
		token := c.Params("token")
		probeID := uintParam(c, "probeID")
		password := c.Query("password")

		// Validate share link
		link, err := share.Validate(c.UserContext(), db, share.ValidateInput{
			Token:    token,
			Password: password,
		})
		if err != nil {
			return fiberHandleShareError(c, err)
		}

		// Verify probe belongs to the shared agent OR targets the shared agent
		// This allows viewing both owned probes AND reverse probes
		var p probe.Probe

		// First try: probe is owned by the shared agent
		err = db.WithContext(c.UserContext()).
			Where("id = ? AND agent_id = ?", probeID, link.AgentID).
			First(&p).Error

		if err != nil {
			// Second try: probe targets the shared agent (reverse probe)
			// Check if any target in the probe has agent_id = shared agent
			err = db.WithContext(c.UserContext()).
				Preload("Targets").
				Where("id = ?", probeID).
				First(&p).Error

			if err != nil {
				return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "probe not found"})
			}

			// Verify the probe targets the shared agent
			isReverseProbe := false
			for _, t := range p.Targets {
				if t.AgentID != nil && *t.AgentID == link.AgentID {
					isReverseProbe = true
					break
				}
			}
			if !isReverseProbe {
				return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "probe not found"})
			}
		}

		// Record access
		_ = share.RecordAccess(c.UserContext(), db, link.ID)

		// Parse query params - EXACTLY like the normal panel endpoint in data.go
		from := c.Query("from", "")
		to := c.Query("to", "")
		limitStr := c.Query("limit", "0")
		limit, _ := strconv.Atoi(limitStr)
		asc := c.Query("asc", "") == "true"
		aggregateSecStr := c.Query("aggregate", "0")
		aggregateSec, _ := strconv.Atoi(aggregateSecStr)
		probeType := c.Query("type") // "PING", "TRAFFICSIM", or "MTR"

		// Parse time range to time.Time
		var fromTime, toTime time.Time
		if from != "" {
			fromTime, _ = time.Parse(time.RFC3339, from)
		}
		if to != "" {
			toTime, _ = time.Parse(time.RFC3339, to)
		}

		// Use the SAME logic as the normal panel endpoint (data.go)
		var rows []probe.ProbeData
		var queryErr error

		if aggregateSec > 0 && (probeType == "PING" || probeType == "TRAFFICSIM" || probeType == "MTR") {
			// Use aggregated query for performance
			rows, queryErr = probe.GetProbeDataAggregated(c.UserContext(), ch, uint64(probeID), nil, probeType, fromTime, toTime, aggregateSec, limit)
		} else {
			// Standard non-aggregated query
			rows, queryErr = probe.GetProbeDataByProbe(c.UserContext(), ch, uint64(probeID), nil, fromTime, toTime, asc, limit)
			// Post-filter by type if specified
			if queryErr == nil && probeType != "" {
				filtered := make([]probe.ProbeData, 0, len(rows))
				for _, r := range rows {
					if string(r.Type) == probeType {
						filtered = append(filtered, r)
					}
				}
				rows = filtered
			}
		}

		if queryErr != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "failed to query probe data"})
		}

		// Return the SAME format as the normal panel - NewListResponse(rows)
		return c.JSON(NewListResponse(rows))
	})
}

// fiberHandleShareError handles common share link errors.
func fiberHandleShareError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, share.ErrShareLinkNotFound):
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "share link not found"})
	case errors.Is(err, share.ErrShareLinkExpired):
		return c.Status(http.StatusGone).JSON(fiber.Map{"error": "share link has expired"})
	case errors.Is(err, share.ErrPasswordRequired):
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "password required", "requires_password": true})
	case errors.Is(err, share.ErrInvalidPassword):
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "invalid password"})
	default:
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}
