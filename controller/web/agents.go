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

	"github.com/kataras/iris/v12"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func panelAgents(api iris.Party, db *gorm.DB, ch *sql.DB, limitsConfig *limits.Config) {
	ws := api.Party("/workspaces/{id:uint}")
	wsStore := workspace.NewStore(db)

	// Apply workspace access check to all agent routes
	as := ws.Party("/agents")
	as.Use(RequireWorkspaceAccess(wsStore))

	// GET /workspaces/{id}/agents
	as.Get("/", func(ctx iris.Context) {
		wsID := uintParam(ctx, "id")
		limit := intParam(ctx, "limit", 50, 1, 200)
		offset := intParam(ctx, "offset", 0, 0, 1_000_000)
		list, total, err := agent.ListAgentsByWorkspace(ctx.Request().Context(), db, wsID, limit, offset)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(iris.Map{"data": list, "total": total, "limit": limit, "offset": offset})
	})

	// POST /workspaces/{id}/agents - requires CanEdit (USER+)
	as.Post("/", RequireRole(wsStore, CanEdit), func(ctx iris.Context) {
		wsID := uintParam(ctx, "id")
		var body struct {
			Name             string         `json:"name"`
			Description      string         `json:"description"`
			Location         string         `json:"location"`
			PublicIPOverride string         `json:"public_ip_override"`
			Version          string         `json:"version"`
			PinLength        int            `json:"pinLength"`
			PinTTLSeconds    int            `json:"pinTTLSeconds"`
			Labels           map[string]any `json:"labels"`
			Metadata         map[string]any `json:"metadata"`
		}
		if err := ctx.ReadJSON(&body); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			return
		}
		var ttl *time.Duration
		if body.PinTTLSeconds > 0 {
			d := time.Duration(body.PinTTLSeconds) * time.Second
			ttl = &d
		}

		// Check workspace agent limit
		if err := limits.CanAddAgent(ctx.Request().Context(), db, limitsConfig, wsID); err != nil {
			if errors.Is(err, limits.ErrAgentLimitReached) {
				ctx.StatusCode(http.StatusForbidden)
				_ = ctx.JSON(iris.Map{"error": err.Error()})
				return
			}
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		out, err := agent.CreateAgent(ctx.Request().Context(), db, agent.CreateInput{
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
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		ctx.StatusCode(http.StatusCreated)
		_ = ctx.JSON(out)
	})

	// /workspaces/{id}/agents/{agentID}
	aid := as.Party("/{agentID:uint}")

	// GET /workspaces/{id}/agents/{agentID}
	aid.Get("/", func(ctx iris.Context) {
		wsID := uintParam(ctx, "id")
		aID := uintParam(ctx, "agentID")
		a, err := agent.GetAgentByWorkspaceAndID(ctx.Request().Context(), db, wsID, aID)
		if err != nil || a == nil {
			ctx.StatusCode(http.StatusNotFound)
			return
		}
		_ = ctx.JSON(a)
	})

	aid.Get("/netinfo", func(ctx iris.Context) {
		//wsID := uintParam(ctx, "id")
		aID := uintParam(ctx, "agentID")
		a, err := probe.GetLatestNetInfoForAgent(context.TODO(), ch, uint64(aID), nil)
		if err != nil || a == nil {
			ctx.StatusCode(http.StatusNotFound)
			return
		}
		_ = ctx.JSON(a)
	})

	aid.Get("/sysinfo", func(ctx iris.Context) {
		//wsID := uintParam(ctx, "id")
		aID := uintParam(ctx, "agentID")
		a, err := probe.GetLatestSysInfoForAgent(context.TODO(), ch, uint64(aID), nil)
		if err != nil || a == nil {
			ctx.StatusCode(http.StatusNotFound)
			return
		}
		_ = ctx.JSON(a)
	})

	// PATCH /workspaces/{id}/agents/{agentID} - requires CanEdit (USER+)
	aid.Patch("/", RequireRole(wsStore, CanEdit), func(ctx iris.Context) {
		aID := uintParam(ctx, "agentID")
		var body struct {
			Name             *string         `json:"name"`
			Description      *string         `json:"description"`
			Location         *string         `json:"location"`
			PublicIPOverride *string         `json:"public_ip_override"`
			Version          *string         `json:"version"`
			Labels           *map[string]any `json:"labels"`
			Metadata         *map[string]any `json:"metadata"`
		}
		if err := ctx.ReadJSON(&body); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			return
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

		if err := agent.PatchAgentFields(ctx.Request().Context(), db, aID, patch); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		a, _ := agent.GetAgentByID(ctx.Request().Context(), db, aID)
		_ = ctx.JSON(a)
	})

	// DELETE /workspaces/{id}/agents/{agentID} - requires CanManage (ADMIN+)
	aid.Delete("/", RequireRole(wsStore, CanManage), func(ctx iris.Context) {
		aID := uintParam(ctx, "agentID")

		// Send deactivation message to connected agent BEFORE deletion
		// This ensures the agent receives the message while still authenticated
		if GetAgentHub().DeactivateAgent(aID, "deleted") {
			log.Infof("Sent deactivation to connected agent %d before deletion", aID)
		}

		if err := agent.DeleteAgent(ctx.Request().Context(), db, aID); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(iris.Map{"ok": true})
	})

	// POST /workspaces/{id}/agents/{agentID}/heartbeat
	aid.Post("/heartbeat", func(ctx iris.Context) {
		aID := uintParam(ctx, "agentID")
		now := time.Now()
		if err := agent.UpdateAgentSeen(ctx.Request().Context(), db, aID, now); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(iris.Map{"ok": true, "ts": now})
	})

	// POST /workspaces/{id}/agents/{agentID}/issue-pin - requires CanEdit (USER+)
	aid.Post("/issue-pin", RequireRole(wsStore, CanEdit), func(ctx iris.Context) {
		wsID := uintParam(ctx, "id")
		aID := uintParam(ctx, "agentID")
		var body struct {
			PinLength  int `json:"pinLength"`
			TTLSeconds int `json:"ttlSeconds"`
		}
		_ = ctx.ReadJSON(&body)
		var ttl *time.Duration
		if body.TTLSeconds > 0 {
			d := time.Duration(body.TTLSeconds) * time.Second
			ttl = &d
		}
		pin, err := agent.IssuePIN(ctx.Request().Context(), db, wsID, aID, ifZero(body.PinLength, 9), ttl)
		if err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(iris.Map{"pin": pin})
	})

	// POST /workspaces/{id}/agents/{agentID}/regenerate - requires CanManage (ADMIN+)
	// Invalidates existing PSK (disconnecting any connected agent), marks agent as uninitialized,
	// and issues a new PIN for reinstallation on a different machine.
	aid.Post("/regenerate", RequireRole(wsStore, CanManage), func(ctx iris.Context) {
		wsID := uintParam(ctx, "id")
		aID := uintParam(ctx, "agentID")
		var body struct {
			PinLength  int `json:"pinLength"`
			TTLSeconds int `json:"ttlSeconds"`
		}
		_ = ctx.ReadJSON(&body)

		// 1) Send deactivation signal to connected agent BEFORE invalidating PSK
		// This ensures the agent receives the message while still authenticated
		// and triggers its cleanup/uninstall flow
		if GetAgentHub().DeactivateAgent(aID, "regenerated") {
			log.Infof("Sent deactivation to connected agent %d before regeneration", aID)
			// Brief pause to allow the deactivate message to be delivered
			time.Sleep(500 * time.Millisecond)
		}

		// 2) Clear the PSK hash - this invalidates any existing sessions
		// If the agent missed the deactivate signal, it will fail auth on next reconnect
		if err := agent.PatchAgentFields(ctx.Request().Context(), db, aID, map[string]any{
			"psk_hash":    "",    // Clear PSK - invalidates existing sessions
			"initialized": false, // Mark as not initialized - requires bootstrap
		}); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		// 2) Issue a new PIN for reinstallation
		var ttl *time.Duration
		if body.TTLSeconds > 0 {
			d := time.Duration(body.TTLSeconds) * time.Second
			ttl = &d
		}
		pin, err := agent.IssuePIN(ctx.Request().Context(), db, wsID, aID, ifZero(body.PinLength, 9), ttl)
		if err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		// 3) Get updated agent info
		a, _ := agent.GetAgentByID(ctx.Request().Context(), db, aID)

		_ = ctx.JSON(iris.Map{
			"pin":   pin,
			"agent": a,
		})
	})
}
