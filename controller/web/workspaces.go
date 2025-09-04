// web/workspaces.go
package web

import (
	"net/http"
	"strconv"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
	"netwatcher-controller/internal/workspace"
)

func panelWorkspaces(api iris.Party, db *gorm.DB) {
	wsParty := api.Party("/workspaces")
	store := workspace.NewStore(db)

	// GET /workspaces
	wsParty.Get("/", func(ctx iris.Context) {
		uid := currentUserID(ctx)
		out, err := store.ListWorkspaces(ctx.Request().Context(), workspace.ListWorkspacesFilter{
			OwnerID: uid,
			Query:   stringsTrim(ctx.URLParamDefault("q", "")),
			Limit:   intParam(ctx, "limit", 50, 1, 200),
			Offset:  intParam(ctx, "offset", 0, 0, 1_000_000),
		})
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(out)
	})

	// POST /workspaces
	wsParty.Post("/", func(ctx iris.Context) {
		uid := currentUserID(ctx)
		var body struct {
			Name        string         `json:"name"`
			DisplayName string         `json:"displayName"`
			Settings    map[string]any `json:"settings"`
		}
		if err := ctx.ReadJSON(&body); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			return
		}
		in := workspace.CreateWorkspaceInput{
			Name:        body.Name,
			OwnerID:     uid,
			DisplayName: body.DisplayName,
			Settings:    jsonFromMap(body.Settings),
		}
		ws, err := store.CreateWorkspace(ctx.Request().Context(), in)
		if err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		ctx.StatusCode(http.StatusCreated)
		_ = ctx.JSON(ws)
	})

	// /workspaces/{id}
	wsID := wsParty.Party("/{id:uint}")

	// GET /workspaces/{id}
	wsID.Get("/", func(ctx iris.Context) {
		id := uintParam(ctx, "id")
		ws, err := store.GetWorkspace(ctx.Request().Context(), id)
		if err != nil || ws == nil || ws.OwnerID != currentUserID(ctx) {
			ctx.StatusCode(http.StatusNotFound)
			return
		}
		_ = ctx.JSON(ws)
	})

	// PATCH /workspaces/{id}
	wsID.Patch("/", func(ctx iris.Context) {
		id := uintParam(ctx, "id")
		var body struct {
			DisplayName *string         `json:"displayName"`
			Settings    *map[string]any `json:"settings"`
		}
		if err := ctx.ReadJSON(&body); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			return
		}
		in := workspace.UpdateWorkspaceInput{
			DisplayName: body.DisplayName,
			Settings:    jsonPtrFromMap(body.Settings),
		}
		ws, err := store.UpdateWorkspace(ctx.Request().Context(), id, in)
		if err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(ws)
	})

	// DELETE /workspaces/{id}
	wsID.Delete("/", func(ctx iris.Context) {
		id := uintParam(ctx, "id")
		if err := store.DeleteWorkspace(ctx.Request().Context(), id); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(iris.Map{"ok": true})
	})
}

func uintParam(ctx iris.Context, name string) uint {
	v, _ := strconv.Atoi(ctx.Params().Get(name))
	return uint(v)
}
