// web/agents.go
package web

import (
	"context"
	"database/sql"
	"net/http"
	"netwatcher-controller/internal/probe_data"
	"os"
	"strconv"
	"time"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
	"netwatcher-controller/internal/agent"
)

func panelAgents(api iris.Party, db *gorm.DB, ch *sql.DB) {
	ws := api.Party("/workspaces/{id:uint}")
	as := ws.Party("/agents")

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

	// POST /workspaces/{id}/agents  (returns agent + bootstrap PIN)
	as.Post("/", func(ctx iris.Context) {
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
		pinPepper := os.Getenv("PIN_PEPPER")
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
		}, pinPepper)
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
		a, err := probe_data.GetLatestNetInfoForAgent(context.TODO(), ch, uint64(aID), nil)
		if err != nil || a == nil {
			ctx.StatusCode(http.StatusNotFound)
			return
		}
		_ = ctx.JSON(a)
	})

	aid.Get("/sysinfo", func(ctx iris.Context) {
		//wsID := uintParam(ctx, "id")
		aID := uintParam(ctx, "agentID")
		a, err := probe_data.GetLatestSysInfoForAgent(context.TODO(), ch, uint64(aID), nil)
		if err != nil || a == nil {
			ctx.StatusCode(http.StatusNotFound)
			return
		}
		_ = ctx.JSON(a)
	})

	// PATCH /workspaces/{id}/agents/{agentID}
	aid.Patch("/", func(ctx iris.Context) {
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

	// DELETE /workspaces/{id}/agents/{agentID}
	aid.Delete("/", func(ctx iris.Context) {
		aID := uintParam(ctx, "agentID")
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

	// POST /workspaces/{id}/agents/{agentID}/issue-pin
	aid.Post("/issue-pin", func(ctx iris.Context) {
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
		pinPepper := os.Getenv("PIN_PEPPER")
		pin, err := agent.IssuePIN(ctx.Request().Context(), db, wsID, aID, ifZero(body.PinLength, 9), pinPepper, ttl)
		if err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(iris.Map{"pin": pin})
	})
}

func intParam(ctx iris.Context, name string, def, min, max int) int {
	if v, err := strconv.Atoi(ctx.URLParamDefault(name, "")); err == nil {
		if v < min {
			return min
		}
		if v > max {
			return max
		}
		return v
	}
	return def
}

func ifZero(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}
