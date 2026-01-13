// web/alerts.go
package web

import (
	"net/http"

	"netwatcher-controller/internal/alert"
	"netwatcher-controller/internal/workspace"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
)

func panelAlerts(api iris.Party, db *gorm.DB) {
	wsStore := workspace.NewStore(db)

	// Global alert endpoints (across all user's workspaces)
	alerts := api.Party("/alerts")

	// GET /alerts - List all alerts for user's workspaces
	alerts.Get("/", func(ctx iris.Context) {
		userID := getUserID(ctx)
		if userID == 0 {
			ctx.StatusCode(http.StatusUnauthorized)
			return
		}

		// Get user's workspace IDs
		workspaceIDs, err := getUserWorkspaceIDs(ctx.Request().Context(), db, userID)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		if len(workspaceIDs) == 0 {
			_ = ctx.JSON(NewListResponse([]alert.Alert{}))
			return
		}

		// Get status filter
		var statusFilter *alert.Status
		if s := ctx.URLParam("status"); s != "" {
			status := alert.Status(s)
			statusFilter = &status
		}

		limit := ctx.URLParamIntDefault("limit", 100)

		var allAlerts []alert.Alert
		for _, wsID := range workspaceIDs {
			alerts, err := alert.ListAlerts(ctx.Request().Context(), db, &wsID, statusFilter, limit)
			if err != nil {
				continue
			}
			allAlerts = append(allAlerts, alerts...)
		}

		_ = ctx.JSON(NewListResponse(allAlerts))
	})

	// GET /alerts/count - Get count of active alerts
	alerts.Get("/count", func(ctx iris.Context) {
		userID := getUserID(ctx)
		if userID == 0 {
			ctx.StatusCode(http.StatusUnauthorized)
			return
		}

		workspaceIDs, err := getUserWorkspaceIDs(ctx.Request().Context(), db, userID)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		count, err := alert.CountActiveAlerts(ctx.Request().Context(), db, workspaceIDs)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		_ = ctx.JSON(iris.Map{"count": count})
	})

	// GET /alerts/:id - Get single alert
	alerts.Get("/{id:uint}", func(ctx iris.Context) {
		id := uintParam(ctx, "id")
		a, err := alert.GetAlertByID(ctx.Request().Context(), db, id)
		if err != nil {
			ctx.StatusCode(http.StatusNotFound)
			return
		}
		_ = ctx.JSON(a)
	})

	// PATCH /alerts/:id/acknowledge - Acknowledge alert
	alerts.Patch("/{id:uint}/acknowledge", func(ctx iris.Context) {
		id := uintParam(ctx, "id")
		userID := getUserID(ctx)

		if err := alert.AcknowledgeAlert(ctx.Request().Context(), db, id, userID); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(iris.Map{"ok": true})
	})

	// PATCH /alerts/:id/resolve - Resolve alert
	alerts.Patch("/{id:uint}/resolve", func(ctx iris.Context) {
		id := uintParam(ctx, "id")

		if err := alert.ResolveAlert(ctx.Request().Context(), db, id); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(iris.Map{"ok": true})
	})

	// -------------------- Alert Rules (per workspace) --------------------
	rules := api.Party("/workspaces/{id:uint}/alert-rules")
	rules.Use(RequireWorkspaceAccess(wsStore))

	// GET /workspaces/:id/alert-rules - List rules for workspace
	rules.Get("/", func(ctx iris.Context) {
		wsID := uintParam(ctx, "id")
		list, err := alert.ListRulesByWorkspace(ctx.Request().Context(), db, wsID)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(NewListResponse(list))
	})

	// POST /workspaces/:id/alert-rules - Create rule (requires CanEdit)
	rules.Post("/", RequireRole(wsStore, CanEdit), func(ctx iris.Context) {
		wsID := uintParam(ctx, "id")
		var input alert.CreateRuleInput
		if err := ctx.ReadJSON(&input); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			return
		}
		input.WorkspaceID = wsID

		rule, err := alert.CreateRule(ctx.Request().Context(), db, input)
		if err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		ctx.StatusCode(http.StatusCreated)
		_ = ctx.JSON(rule)
	})

	// GET /workspaces/:id/alert-rules/:ruleID - Get single rule
	rules.Get("/{ruleID:uint}", func(ctx iris.Context) {
		ruleID := uintParam(ctx, "ruleID")
		rule, err := alert.GetRuleByID(ctx.Request().Context(), db, ruleID)
		if err != nil {
			ctx.StatusCode(http.StatusNotFound)
			return
		}
		_ = ctx.JSON(rule)
	})

	// PATCH /workspaces/:id/alert-rules/:ruleID - Update rule (requires CanEdit)
	rules.Patch("/{ruleID:uint}", RequireRole(wsStore, CanEdit), func(ctx iris.Context) {
		ruleID := uintParam(ctx, "ruleID")
		var input alert.UpdateRuleInput
		if err := ctx.ReadJSON(&input); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			return
		}
		input.ID = ruleID

		rule, err := alert.UpdateRule(ctx.Request().Context(), db, input)
		if err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(rule)
	})

	// DELETE /workspaces/:id/alert-rules/:ruleID - Delete rule (requires CanManage)
	rules.Delete("/{ruleID:uint}", RequireRole(wsStore, CanManage), func(ctx iris.Context) {
		ruleID := uintParam(ctx, "ruleID")
		if err := alert.DeleteRule(ctx.Request().Context(), db, ruleID); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(iris.Map{"ok": true})
	})
}
