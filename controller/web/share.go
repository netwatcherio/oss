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

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
)

// -------------------- Protected Endpoints (JWT auth) --------------------

// panelShareLinks registers share link management endpoints for authenticated users.
func panelShareLinks(api iris.Party, db *gorm.DB) {
	// Create share link for an agent
	api.Post("/workspaces/{id:uint64}/agents/{agentID:uint64}/share-links", func(ctx iris.Context) {
		workspaceID := uint(ctx.Params().GetUint64Default("id", 0))
		agentID := uint(ctx.Params().GetUint64Default("agentID", 0))

		userID, ok := ctx.Values().Get("userID").(uint)
		if !ok {
			ctx.StatusCode(http.StatusUnauthorized)
			_ = ctx.JSON(iris.Map{"error": "unauthorized"})
			return
		}

		// Verify user has access to workspace
		if !hasWorkspaceAccess(ctx, db, workspaceID, userID) {
			return
		}

		// Verify agent belongs to workspace
		_, err := agent.GetAgentByWorkspaceAndID(ctx.Request().Context(), db, workspaceID, agentID)
		if err != nil {
			if errors.Is(err, agent.ErrNotFound) {
				ctx.StatusCode(http.StatusNotFound)
				_ = ctx.JSON(iris.Map{"error": "agent not found"})
				return
			}
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		// Parse request body
		var body struct {
			ExpiresInSeconds int    `json:"expires_in_seconds"`
			Password         string `json:"password,omitempty"`
		}
		if err := ctx.ReadJSON(&body); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "invalid request body"})
			return
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
		output, err := share.Create(ctx.Request().Context(), db, share.CreateInput{
			WorkspaceID:     workspaceID,
			AgentID:         agentID,
			CreatedByUserID: userID,
			ExpiresIn:       expiresIn,
			Password:        body.Password,
		})
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		ctx.StatusCode(http.StatusCreated)
		_ = ctx.JSON(output)
	})

	// List share links for an agent
	api.Get("/workspaces/{id:uint64}/agents/{agentID:uint64}/share-links", func(ctx iris.Context) {
		workspaceID := uint(ctx.Params().GetUint64Default("id", 0))
		agentID := uint(ctx.Params().GetUint64Default("agentID", 0))

		userID, ok := ctx.Values().Get("userID").(uint)
		if !ok {
			ctx.StatusCode(http.StatusUnauthorized)
			_ = ctx.JSON(iris.Map{"error": "unauthorized"})
			return
		}

		// Verify user has access to workspace
		if !hasWorkspaceAccess(ctx, db, workspaceID, userID) {
			return
		}

		links, err := share.ListByAgent(ctx.Request().Context(), db, workspaceID, agentID)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		_ = ctx.JSON(iris.Map{"items": links, "total": len(links)})
	})

	// Delete (revoke) a share link
	api.Delete("/workspaces/{id:uint64}/agents/{agentID:uint64}/share-links/{linkID:uint64}", func(ctx iris.Context) {
		workspaceID := uint(ctx.Params().GetUint64Default("id", 0))
		agentID := uint(ctx.Params().GetUint64Default("agentID", 0))
		linkID := uint(ctx.Params().GetUint64Default("linkID", 0))

		userID, ok := ctx.Values().Get("userID").(uint)
		if !ok {
			ctx.StatusCode(http.StatusUnauthorized)
			_ = ctx.JSON(iris.Map{"error": "unauthorized"})
			return
		}

		// Verify user has access to workspace
		if !hasWorkspaceAccess(ctx, db, workspaceID, userID) {
			return
		}

		err := share.Delete(ctx.Request().Context(), db, workspaceID, agentID, linkID)
		if err != nil {
			if errors.Is(err, share.ErrShareLinkNotFound) {
				ctx.StatusCode(http.StatusNotFound)
				_ = ctx.JSON(iris.Map{"error": "share link not found"})
				return
			}
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		ctx.StatusCode(http.StatusNoContent)
	})
}

// hasWorkspaceAccess checks if the user has access to the workspace.
func hasWorkspaceAccess(ctx iris.Context, db *gorm.DB, workspaceID, userID uint) bool {
	store := workspace.NewStore(db)
	_, err := store.GetMemberByUserID(ctx.Request().Context(), workspaceID, userID)
	if err != nil {
		if errors.Is(err, workspace.ErrNotFound) {
			ctx.StatusCode(http.StatusForbidden)
			_ = ctx.JSON(iris.Map{"error": "access denied"})
			return false
		}
		ctx.StatusCode(http.StatusInternalServerError)
		_ = ctx.JSON(iris.Map{"error": err.Error()})
		return false
	}
	return true
}

// -------------------- Public Endpoints (no auth) --------------------

// RegisterShareRoutes registers public share link access endpoints.
func RegisterShareRoutes(app *iris.Application, db *gorm.DB, ch *sql.DB) {
	shareAPI := app.Party("/share")

	// Get shared agent info (validates token and optional password)
	shareAPI.Get("/{token:string}", func(ctx iris.Context) {
		token := ctx.Params().Get("token")
		password := ctx.URLParam("password")

		// Validate share link
		link, err := share.Validate(ctx.Request().Context(), db, share.ValidateInput{
			Token:    token,
			Password: password,
		})
		if err != nil {
			handleShareError(ctx, err)
			return
		}

		// Record access
		_ = share.RecordAccess(ctx.Request().Context(), db, link.ID)

		// Get agent info
		ag, err := agent.GetAgentByID(ctx.Request().Context(), db, link.AgentID)
		if err != nil {
			ctx.StatusCode(http.StatusNotFound)
			_ = ctx.JSON(iris.Map{"error": "agent not found"})
			return
		}

		// Determine public IP - prefer actual NETINFO public_address over override
		publicIP := ag.PublicIPOverride
		if ch != nil {
			netInfoData, err := probe.GetLatestNetInfoForAgent(ctx.Request().Context(), ch, uint64(link.AgentID), nil)
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
		owned, reverse, err := probe.ListByAgentWithReverse(ctx.Request().Context(), db, link.AgentID)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": "failed to fetch probes"})
			return
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
		_ = ctx.JSON(iris.Map{
			"agent": iris.Map{
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
	shareAPI.Get("/{token:string}/info", func(ctx iris.Context) {
		token := ctx.Params().Get("token")

		link, err := share.GetByToken(ctx.Request().Context(), db, token)
		if err != nil {
			if errors.Is(err, share.ErrShareLinkNotFound) {
				ctx.StatusCode(http.StatusNotFound)
				_ = ctx.JSON(iris.Map{"error": "share link not found"})
				return
			}
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		// Check if expired
		expired := time.Now().After(link.ExpiresAt)

		_ = ctx.JSON(iris.Map{
			"has_password":    link.HasPassword,
			"expired":         expired,
			"expires_at":      link.ExpiresAt,
			"allow_speedtest": link.AllowSpeedtest,
		})
	})

	// Get agent name for shared context (sanitized - only returns name)
	// Only works for agents that are part of probes visible to this share link
	shareAPI.Get("/{token:string}/agent/{agentID:uint64}", func(ctx iris.Context) {
		token := ctx.Params().Get("token")
		agentID := uint(ctx.Params().GetUint64Default("agentID", 0))
		password := ctx.URLParam("password")

		// Validate share link
		link, err := share.Validate(ctx.Request().Context(), db, share.ValidateInput{
			Token:    token,
			Password: password,
		})
		if err != nil {
			handleShareError(ctx, err)
			return
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
			db.WithContext(ctx.Request().Context()).
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
			db.WithContext(ctx.Request().Context()).
				Table("probe_targets").
				Joins("JOIN probes ON probes.id = probe_targets.probe_id").
				Where("probes.agent_id = ? AND probe_targets.agent_id = ?", agentID, link.AgentID).
				Count(&count)
			if count > 0 {
				isAccessible = true
			}
		}

		if !isAccessible {
			ctx.StatusCode(http.StatusNotFound)
			_ = ctx.JSON(iris.Map{"error": "agent not found"})
			return
		}

		// Get agent and return only safe fields
		ag, err := agent.GetAgentByID(ctx.Request().Context(), db, agentID)
		if err != nil {
			ctx.StatusCode(http.StatusNotFound)
			_ = ctx.JSON(iris.Map{"error": "agent not found"})
			return
		}

		// Return only name (and optionally location for context)
		_ = ctx.JSON(iris.Map{
			"id":       ag.ID,
			"name":     ag.Name,
			"location": ag.Location,
		})
	})

	// Get probe data for shared agent
	shareAPI.Get("/{token:string}/probe-data/{probeID:uint64}", func(ctx iris.Context) {
		token := ctx.Params().Get("token")
		probeID := uint(ctx.Params().GetUint64Default("probeID", 0))
		password := ctx.URLParam("password")

		// Validate share link
		link, err := share.Validate(ctx.Request().Context(), db, share.ValidateInput{
			Token:    token,
			Password: password,
		})
		if err != nil {
			handleShareError(ctx, err)
			return
		}

		// Verify probe belongs to the shared agent OR targets the shared agent
		// This allows viewing both owned probes AND reverse probes
		var p probe.Probe

		// First try: probe is owned by the shared agent
		err = db.WithContext(ctx.Request().Context()).
			Where("id = ? AND agent_id = ?", probeID, link.AgentID).
			First(&p).Error

		if err != nil {
			// Second try: probe targets the shared agent (reverse probe)
			// Check if any target in the probe has agent_id = shared agent
			err = db.WithContext(ctx.Request().Context()).
				Preload("Targets").
				Where("id = ?", probeID).
				First(&p).Error

			if err != nil {
				ctx.StatusCode(http.StatusNotFound)
				_ = ctx.JSON(iris.Map{"error": "probe not found"})
				return
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
				ctx.StatusCode(http.StatusNotFound)
				_ = ctx.JSON(iris.Map{"error": "probe not found"})
				return
			}
		}

		// Record access
		_ = share.RecordAccess(ctx.Request().Context(), db, link.ID)

		// Parse query params - EXACTLY like the normal panel endpoint in data.go
		from := ctx.URLParamDefault("from", "")
		to := ctx.URLParamDefault("to", "")
		limitStr := ctx.URLParamDefault("limit", "0")
		limit, _ := strconv.Atoi(limitStr)
		asc := ctx.URLParamDefault("asc", "") == "true"
		aggregateSecStr := ctx.URLParamDefault("aggregate", "0")
		aggregateSec, _ := strconv.Atoi(aggregateSecStr)
		probeType := ctx.URLParam("type") // "PING", "TRAFFICSIM", or "MTR"

		// Parse time range to time.Time
		var fromTime, toTime time.Time
		if from != "" {
			fromTime, _ = time.Parse(time.RFC3339, from)
		}
		if to != "" {
			toTime, _ = time.Parse(time.RFC3339, to)
		}

		// Use the SAME logic as the normal panel endpoint (data.go lines 156-195)
		var rows []probe.ProbeData
		var queryErr error

		if aggregateSec > 0 && (probeType == "PING" || probeType == "TRAFFICSIM" || probeType == "MTR") {
			// Use aggregated query for performance
			rows, queryErr = probe.GetProbeDataAggregated(ctx.Request().Context(), ch, uint64(probeID), probeType, fromTime, toTime, aggregateSec, limit)
		} else {
			// Standard non-aggregated query
			rows, queryErr = probe.GetProbeDataByProbe(ctx.Request().Context(), ch, uint64(probeID), fromTime, toTime, asc, limit)
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
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": "failed to query probe data"})
			return
		}

		// Return the SAME format as the normal panel - NewListResponse(rows)
		_ = ctx.JSON(NewListResponse(rows))
	})
}

// handleShareError handles common share link errors.
func handleShareError(ctx iris.Context, err error) {
	switch {
	case errors.Is(err, share.ErrShareLinkNotFound):
		ctx.StatusCode(http.StatusNotFound)
		_ = ctx.JSON(iris.Map{"error": "share link not found"})
	case errors.Is(err, share.ErrShareLinkExpired):
		ctx.StatusCode(http.StatusGone)
		_ = ctx.JSON(iris.Map{"error": "share link has expired"})
	case errors.Is(err, share.ErrPasswordRequired):
		ctx.StatusCode(http.StatusUnauthorized)
		_ = ctx.JSON(iris.Map{"error": "password required", "requires_password": true})
	case errors.Is(err, share.ErrInvalidPassword):
		ctx.StatusCode(http.StatusUnauthorized)
		_ = ctx.JSON(iris.Map{"error": "invalid password"})
	default:
		ctx.StatusCode(http.StatusInternalServerError)
		_ = ctx.JSON(iris.Map{"error": err.Error()})
	}
}
