// web/probes.go
package web

import (
	"net/http"

	"netwatcher-controller/internal/probe"
	"netwatcher-controller/internal/workspace"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
)

func panelProbes(api iris.Party, db *gorm.DB) {
	base := api.Party("/workspaces/{id:uint}/agents/{agentID:uint}/probes")
	wsStore := workspace.NewStore(db)

	// Apply workspace access check to all probe routes
	base.Use(RequireWorkspaceAccess(wsStore))

	// GET /workspaces/{id}/agents/{agentID}/probes - requires CanView (any member)
	base.Get("/", func(ctx iris.Context) {
		aID := uintParam(ctx, "agentID")
		list, err := probe.ListByAgent(ctx.Request().Context(), db, aID)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(NewListResponse(list))
	})

	// POST /workspaces/{id}/agents/{agentID}/probes - requires CanEdit (USER+)
	base.Post("/", RequireRole(wsStore, CanEdit), func(ctx iris.Context) {
		var input probe.CreateInput

		if err := ctx.ReadJSON(&input); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			return
		}
		p, err := probe.Create(ctx.Request().Context(), db, input)
		if err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		ctx.StatusCode(http.StatusCreated)
		_ = ctx.JSON(p)
	})

	// /workspaces/{id}/agents/{agentID}/probes/{probeID}
	pid := base.Party("/{probeID:uint}")

	// GET /workspaces/{id}/agents/{agentID}/probes/{probeID} - requires CanView (any member)
	pid.Get("/", func(ctx iris.Context) {
		id := uintParam(ctx, "probeID")
		p, err := probe.GetByID(ctx.Request().Context(), db, id)
		if err != nil || p == nil {
			ctx.StatusCode(http.StatusNotFound)
			return
		}
		_ = ctx.JSON(p)
	})

	// PATCH /workspaces/{id}/agents/{agentID}/probes/{probeID} - requires CanEdit (USER+)
	pid.Patch("/", RequireRole(wsStore, CanEdit), func(ctx iris.Context) {
		id := uintParam(ctx, "probeID")
		var body struct {
			Enabled             *bool           `json:"enabled"`
			IntervalSec         *int            `json:"intervalSec"`
			TimeoutSec          *int            `json:"timeoutSec"`
			Labels              *map[string]any `json:"labels"`
			Metadata            *map[string]any `json:"metadata"`
			ReplaceTargets      []string        `json:"replaceTargets"`
			ReplaceAgentTargets []uint          `json:"replaceAgentTargets"`
		}
		if err := ctx.ReadJSON(&body); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			return
		}
		in := probe.UpdateInput{
			ID:                  id,
			Enabled:             body.Enabled,
			IntervalSec:         body.IntervalSec,
			TimeoutSec:          body.TimeoutSec,
			Labels:              jsonPtrFromMap(body.Labels),
			Metadata:            jsonPtrFromMap(body.Metadata),
			ReplaceTargets:      body.ReplaceTargets,
			ReplaceAgentTargets: body.ReplaceAgentTargets,
		}
		p, err := probe.Update(ctx.Request().Context(), db, in)
		if err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(p)
	})

	// DELETE /workspaces/{id}/agents/{agentID}/probes/{probeID} - requires CanManage (ADMIN+)
	pid.Delete("/", RequireRole(wsStore, CanManage), func(ctx iris.Context) {
		id := uintParam(ctx, "probeID")
		if err := probe.Delete(ctx.Request().Context(), db, id); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(iris.Map{"ok": true})
	})
}
