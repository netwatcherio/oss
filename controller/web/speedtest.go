// web/speedtest.go
package web

import (
	"database/sql"
	"net/http"
	"netwatcher-controller/internal/speedtest"
	"netwatcher-controller/internal/workspace"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
)

// panelSpeedtest registers speedtest queue and server endpoints under agents.
// Routes: /workspaces/{id}/agents/{agentID}/speedtest-*
func panelSpeedtest(api iris.Party, db *gorm.DB, ch *sql.DB) {
	ws := api.Party("/workspaces/{id:uint}")
	wsStore := workspace.NewStore(db)

	// Speedtest routes under individual agents
	st := ws.Party("/agents/{agentID:uint}")
	st.Use(RequireWorkspaceAccess(wsStore))

	// -------------------- Queue Endpoints --------------------

	// GET /workspaces/{id}/agents/{agentID}/speedtest-queue
	// List queue items for an agent
	st.Get("/speedtest-queue", func(ctx iris.Context) {
		aID := uintParam(ctx, "agentID")
		var status *speedtest.QueueStatus
		if s := ctx.URLParam("status"); s != "" {
			st := speedtest.QueueStatus(s)
			status = &st
		}

		items, err := speedtest.ListForAgent(ctx.Request().Context(), db, aID, status)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(iris.Map{"data": items, "total": len(items)})
	})

	// POST /workspaces/{id}/agents/{agentID}/speedtest-queue
	// Add a new speedtest to the queue - requires CanEdit (USER+)
	st.Post("/speedtest-queue", RequireRole(wsStore, CanEdit), func(ctx iris.Context) {
		wsID := uintParam(ctx, "id")
		aID := uintParam(ctx, "agentID")
		userID := ctx.Values().GetUintDefault("user_id", 0)

		var body struct {
			ServerID   string `json:"server_id"`
			ServerName string `json:"server_name"`
		}
		if err := ctx.ReadJSON(&body); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "invalid request body"})
			return
		}

		item, err := speedtest.CreateQueueItem(ctx.Request().Context(), db, speedtest.CreateQueueInput{
			WorkspaceID: wsID,
			AgentID:     aID,
			ServerID:    body.ServerID,
			ServerName:  body.ServerName,
			RequestedBy: userID,
		})
		if err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		ctx.StatusCode(http.StatusCreated)
		_ = ctx.JSON(item)
	})

	// GET /workspaces/{id}/agents/{agentID}/speedtest-queue/{queueID}
	// Get a specific queue item
	st.Get("/speedtest-queue/{queueID:uint}", func(ctx iris.Context) {
		qID := uintParam(ctx, "queueID")
		item, err := speedtest.GetQueueItem(ctx.Request().Context(), db, qID)
		if err != nil {
			ctx.StatusCode(http.StatusNotFound)
			_ = ctx.JSON(iris.Map{"error": "queue item not found"})
			return
		}
		_ = ctx.JSON(item)
	})

	// DELETE /workspaces/{id}/agents/{agentID}/speedtest-queue/{queueID}
	// Cancel a pending queue item - requires CanEdit (USER+)
	st.Delete("/speedtest-queue/{queueID:uint}", RequireRole(wsStore, CanEdit), func(ctx iris.Context) {
		qID := uintParam(ctx, "queueID")
		if err := speedtest.CancelQueueItem(ctx.Request().Context(), db, qID); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(iris.Map{"ok": true})
	})

	// -------------------- Server Cache Endpoints --------------------

	// GET /workspaces/{id}/agents/{agentID}/speedtest-servers
	// Get cached speedtest servers for an agent
	st.Get("/speedtest-servers", func(ctx iris.Context) {
		aID := uintParam(ctx, "agentID")
		servers, err := speedtest.ListServersForAgent(ctx.Request().Context(), db, aID)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(iris.Map{"data": servers, "total": len(servers)})
	})
}
