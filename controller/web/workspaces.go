// web/workspaces.go
package web

import (
	"net/http"
	"strconv"
	"strings"

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
			_ = ctx.JSON(iris.Map{"error": "invalid json"})
			return
		}
		in := workspace.CreateWorkspaceInput{
			Name:        body.Name,
			OwnerID:     uid,
			Description: body.DisplayName,
			Settings:    jsonFromMap(body.Settings),
		}
		ws, err := store.CreateWorkspace(ctx.Request().Context(), in)
		if err != nil {
			status := http.StatusBadRequest
			if err == workspace.ErrAlreadyExists {
				status = http.StatusConflict
			}
			ctx.StatusCode(status)
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
			_ = ctx.JSON(iris.Map{"error": "not found"})
			return
		}
		_ = ctx.JSON(ws)
	})

	// PATCH /workspaces/{id}
	wsID.Patch("/", func(ctx iris.Context) {
		id := uintParam(ctx, "id")
		var body struct {
			Name        *string         `json:"name"`
			Description *string         `json:"description"`
			Settings    *map[string]any `json:"settings"`
		}
		if err := ctx.ReadJSON(&body); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "invalid json"})
			return
		}
		in := workspace.UpdateWorkspaceInput{
			Description: body.Description,
			Name:        body.Name,
			Settings:    jsonPtrFromMap(body.Settings),
		}
		ws, err := store.UpdateWorkspace(ctx.Request().Context(), id, in)
		if err != nil {
			status := http.StatusBadRequest
			if err == workspace.ErrNotFound {
				status = http.StatusNotFound
			}
			ctx.StatusCode(status)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(ws)
	})

	// DELETE /workspaces/{id}
	wsID.Delete("/", func(ctx iris.Context) {
		id := uintParam(ctx, "id")
		err := store.DeleteWorkspace(ctx.Request().Context(), id)
		if err != nil {
			status := http.StatusBadRequest
			if err == workspace.ErrNotFound {
				status = http.StatusNotFound
			}
			ctx.StatusCode(status)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(iris.Map{"ok": true})
	})

	// ----- Members -----

	// GET /workspaces/{id}/members
	wsID.Get("/members", func(ctx iris.Context) {
		id := uintParam(ctx, "id")
		ms, err := store.ListMembers(ctx.Request().Context(), id)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(ms)
	})

	// POST /workspaces/{id}/members
	wsID.Post("/members", func(ctx iris.Context) {
		wsIDv := uintParam(ctx, "id")
		var body struct {
			UserID uint           `json:"userId"`
			Email  string         `json:"email"`
			Role   workspace.Role `json:"role"`
			Meta   map[string]any `json:"meta"`
		}
		if err := ctx.ReadJSON(&body); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "invalid json"})
			return
		}
		m, err := store.AddMember(ctx.Request().Context(), workspace.AddMemberInput{
			WorkspaceID: wsIDv,
			UserID:      body.UserID,
			Email:       body.Email,
			Role:        body.Role,
			Meta:        jsonFromMap(body.Meta),
		})
		if err != nil {
			status := http.StatusBadRequest
			switch err {
			case workspace.ErrEmailRequired, workspace.ErrInvalidInput, workspace.ErrInvalidRole:
				status = http.StatusBadRequest
			case workspace.ErrAlreadyExists:
				status = http.StatusConflict
			case workspace.ErrNotFound:
				status = http.StatusNotFound
			case workspace.ErrForbidden:
				status = http.StatusForbidden
			}
			ctx.StatusCode(status)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		ctx.StatusCode(http.StatusCreated)
		_ = ctx.JSON(m)
	})

	// PATCH /workspaces/{id}/members/{memberId}
	wsID.Patch("/members/{memberId:uint}", func(ctx iris.Context) {
		memberID := uintParamName(ctx, "memberId")
		var body struct {
			Role workspace.Role `json:"role"`
		}
		if err := ctx.ReadJSON(&body); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "invalid json"})
			return
		}
		m, err := store.UpdateMemberRole(ctx.Request().Context(), memberID, body.Role)
		if err != nil {
			status := http.StatusBadRequest
			if err == workspace.ErrNotFound {
				status = http.StatusNotFound
			} else if err == workspace.ErrInvalidRole {
				status = http.StatusBadRequest
			}
			ctx.StatusCode(status)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(m)
	})

	// DELETE /workspaces/{id}/members/{memberId}
	wsID.Delete("/members/{memberId:uint}", func(ctx iris.Context) {
		memberID := uintParamName(ctx, "memberId")
		if err := store.RemoveMember(ctx.Request().Context(), memberID); err != nil {
			status := http.StatusBadRequest
			if err == workspace.ErrNotFound {
				status = http.StatusNotFound
			}
			ctx.StatusCode(status)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(iris.Map{"ok": true})
	})

	// POST /workspaces/{id}/accept-invite
	wsID.Post("/accept-invite", func(ctx iris.Context) {
		wsIDv := uintParam(ctx, "id")
		var body struct {
			Email string `json:"email"`
		}
		if err := ctx.ReadJSON(&body); err != nil || strings.TrimSpace(body.Email) == "" {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "email required"})
			return
		}
		userID := currentUserID(ctx)
		m, err := store.AcceptInvite(ctx.Request().Context(), wsIDv, body.Email, userID)
		if err != nil {
			status := http.StatusBadRequest
			switch err {
			case workspace.ErrInvalidInput:
				status = http.StatusBadRequest
			case workspace.ErrNotFound:
				status = http.StatusNotFound
			case workspace.ErrAlreadyExists:
				status = http.StatusConflict
			}
			ctx.StatusCode(status)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(m)
	})

	// POST /workspaces/{id}/transfer-ownership
	wsID.Post("/transfer-ownership", func(ctx iris.Context) {
		wsIDv := uintParam(ctx, "id")
		var body struct {
			NewOwnerUserID uint `json:"newOwnerUserId"`
		}
		if err := ctx.ReadJSON(&body); err != nil || body.NewOwnerUserID == 0 {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "newOwnerUserId required"})
			return
		}
		if err := store.TransferOwnership(ctx.Request().Context(), wsIDv, body.NewOwnerUserID); err != nil {
			status := http.StatusBadRequest
			if err == workspace.ErrInvalidInput {
				status = http.StatusBadRequest
			}
			ctx.StatusCode(status)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(iris.Map{"ok": true})
	})
}

func uintParam(ctx iris.Context, name string) uint {
	v, _ := strconv.Atoi(ctx.Params().Get(name))
	if v < 0 {
		return 0
	}
	return uint(v)
}

func uintParamName(ctx iris.Context, name string) uint {
	v, _ := strconv.Atoi(ctx.Params().Get(name))
	if v < 0 {
		return 0
	}
	return uint(v)
}
