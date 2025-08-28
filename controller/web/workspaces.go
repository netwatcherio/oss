package web

import (
	"errors"
	"strconv"

	"github.com/kataras/iris/v12"
	"netwatcher-controller/internal/users"
	"netwatcher-controller/internal/workspace"
)

// Mount workspaces API using Iris parties (subpaths).
// Note: routes are mounted directly; we return no Route items.
func addRouteWorkspaces(r *Router) []*Route {
	ws := r.App.Party("/workspaces")

	// Protect the whole /workspaces subtree with the same JWT/session check you use elsewhere.
	// If you already add VerifySession() globally after public routes, you can remove this line.
	ws.Use(VerifySession())

	// Helpers
	parseUint := func(ctx iris.Context, name string) (uint, bool) {
		raw := ctx.Params().Get(name)
		id64, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return 0, false
		}
		return uint(id64), true
	}
	currentUser := func(ctx iris.Context) (*users.User, error) {
		tok := bearer(ctx)
		if tok == "" {
			return nil, errors.New("missing token")
		}
		u, _, err := r.AuthSvc.GetUserFromJWT(ctx, tok, r.DB)
		return u, err
	}

	// -------- /workspaces
	ws.Get("/", func(ctx iris.Context) {
		u, err := currentUser(ctx)
		if err != nil {
			ctx.StatusCode(iris.StatusUnauthorized)
			return
		}
		items, _, err := r.WorkspacesSvc.ListForUser(ctx, u.ID, 200, 0)
		if err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			return
		}
		_ = ctx.JSON(items)
	})

	ws.Post("/", func(ctx iris.Context) {
		ctx.ContentType("application/json")

		u, err := currentUser(ctx)
		if err != nil {
			ctx.StatusCode(iris.StatusUnauthorized)
			return
		}

		var wsBody workspace.Workspace
		if err := ctx.ReadJSON(&wsBody); err != nil {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}

		created, err := r.WorkspacesSvc.CreateWithOwner(ctx, u.ID, &wsBody)
		if err != nil {
			_ = ctx.JSON(err) // keep old behavior of returning error JSON
			return
		}
		_ = ctx.JSON(created)
	})

	// -------- /workspaces/{id}
	wsID := ws.Party("/{id:uint}")

	wsID.Get("/", func(ctx iris.Context) {
		u, err := currentUser(ctx)
		if err != nil {
			ctx.StatusCode(iris.StatusUnauthorized)
			return
		}
		id, ok := parseUint(ctx, "id")
		if !ok {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}
		if err := r.WorkspacesSvc.RequireMember(ctx, id, u.ID); err != nil {
			ctx.StatusCode(iris.StatusForbidden)
			return
		}
		wsObj, err := r.WorkspacesSvc.GetByID(ctx, id)
		if err != nil {
			_ = ctx.JSON(err)
			return
		}
		_ = ctx.JSON(wsObj)
	})

	wsID.Post("/update", func(ctx iris.Context) {
		ctx.ContentType("application/json")

		u, err := currentUser(ctx)
		if err != nil {
			ctx.StatusCode(iris.StatusUnauthorized)
			return
		}
		id, ok := parseUint(ctx, "id")
		if !ok {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}
		if err := r.WorkspacesSvc.RequireAdmin(ctx, id, u.ID); err != nil {
			ctx.StatusCode(iris.StatusForbidden)
			return
		}

		var body struct {
			Name        *string `json:"name"`
			Location    *string `json:"location"`
			Description *string `json:"description"`
		}
		if err := ctx.ReadJSON(&body); err != nil {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}

		_, err = r.WorkspacesSvc.Update(ctx, id, workspace.UpdateInput{
			Name:        body.Name,
			Location:    body.Location,
			Description: body.Description,
		})
		if err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			return
		}
		ctx.StatusCode(iris.StatusOK)
	})

	wsID.Post("/delete", func(ctx iris.Context) {
		u, err := currentUser(ctx)
		if err != nil {
			ctx.StatusCode(iris.StatusUnauthorized)
			return
		}
		id, ok := parseUint(ctx, "id")
		if !ok {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}
		if err := r.WorkspacesSvc.RequireOwner(ctx, id, u.ID); err != nil {
			ctx.StatusCode(iris.StatusForbidden)
			return
		}
		if err := r.WorkspacesSvc.Delete(ctx, id); err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			return
		}
		ctx.StatusCode(iris.StatusNoContent)
	})

	// -------- /workspaces/{id}/members
	members := wsID.Party("/members")

	members.Get("/", func(ctx iris.Context) {
		u, err := currentUser(ctx)
		if err != nil {
			ctx.StatusCode(iris.StatusUnauthorized)
			return
		}
		id, ok := parseUint(ctx, "id")
		if !ok {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}
		if err := r.WorkspacesSvc.RequireMember(ctx, id, u.ID); err != nil {
			ctx.StatusCode(iris.StatusForbidden)
			return
		}
		m, err := r.WorkspacesSvc.ListMembers(ctx, id)
		if err != nil {
			_ = ctx.JSON(err)
			return
		}
		_ = ctx.JSON(m)
	})

	members.Post("/", func(ctx iris.Context) {
		ctx.ContentType("application/json")

		u, err := currentUser(ctx)
		if err != nil {
			ctx.StatusCode(iris.StatusUnauthorized)
			return
		}
		id, ok := parseUint(ctx, "id")
		if !ok {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}
		if err := r.WorkspacesSvc.RequireAdmin(ctx, id, u.ID); err != nil {
			ctx.StatusCode(iris.StatusForbidden)
			return
		}

		var body struct {
			UserID *uint          `json:"userId,omitempty"`
			Email  string         `json:"email,omitempty"`
			Role   workspace.Role `json:"role"`
		}
		if err := ctx.ReadJSON(&body); err != nil {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}
		if body.Role == workspace.RoleOwner {
			ctx.StatusCode(iris.StatusBadRequest)
			_, _ = ctx.WriteString("only the current owner can add owners")
			return
		}

		if body.UserID != nil && *body.UserID != 0 {
			if _, err := r.WorkspacesSvc.AddMemberByUserID(ctx, id, *body.UserID, body.Role); err != nil {
				_ = ctx.JSON(err)
				return
			}
		} else {
			if _, err := r.WorkspacesSvc.InviteByEmail(ctx, id, body.Email, body.Role); err != nil {
				_ = ctx.JSON(err)
				return
			}
		}
		ctx.StatusCode(iris.StatusOK)
	})

	members.Post("/{memberId:uint}/update", func(ctx iris.Context) {
		ctx.ContentType("application/json")

		u, err := currentUser(ctx)
		if err != nil {
			ctx.StatusCode(iris.StatusUnauthorized)
			return
		}
		id, ok := parseUint(ctx, "id")
		if !ok {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}
		memberID, ok := parseUint(ctx, "memberId")
		if !ok {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}

		var body struct {
			Role workspace.Role `json:"role"`
		}
		if err := ctx.ReadJSON(&body); err != nil {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}

		if body.Role == workspace.RoleOwner {
			if err := r.WorkspacesSvc.RequireOwner(ctx, id, u.ID); err != nil {
				ctx.StatusCode(iris.StatusForbidden)
				return
			}
		} else {
			if err := r.WorkspacesSvc.RequireAdmin(ctx, id, u.ID); err != nil {
				ctx.StatusCode(iris.StatusForbidden)
				return
			}
		}

		if err := r.WorkspacesSvc.UpdateMemberRole(ctx, id, memberID, body.Role); err != nil {
			_ = ctx.JSON(err)
			return
		}
		ctx.StatusCode(iris.StatusOK)
	})

	members.Post("/{memberId:uint}/delete", func(ctx iris.Context) {
		u, err := currentUser(ctx)
		if err != nil {
			ctx.StatusCode(iris.StatusUnauthorized)
			return
		}
		id, ok := parseUint(ctx, "id")
		if !ok {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}
		memberID, ok := parseUint(ctx, "memberId")
		if !ok {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}

		if err := r.WorkspacesSvc.RequireAdmin(ctx, id, u.ID); err != nil {
			ctx.StatusCode(iris.StatusForbidden)
			return
		}
		if err := r.WorkspacesSvc.RemoveMember(ctx, id, memberID); err != nil {
			_ = ctx.JSON(err)
			return
		}
		ctx.StatusCode(iris.StatusOK)
	})

	// -------- /workspaces/{id}/groups
	/*groups := wsID.Party("/groups")

	groups.Get("/", func(ctx iris.Context) {
		u, err := currentUser(ctx)
		if err != nil {
			ctx.StatusCode(iris.StatusUnauthorized)
			return
		}
		id, ok := parseUint(ctx, "id")
		if !ok {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}
		if err := r.WorkspacesSvc.RequireMember(ctx, id, u.ID); err != nil {
			ctx.StatusCode(iris.StatusForbidden)
			return
		}

		var out []agent.Group
		if err := r.DB.WithContext(ctx).Where("workspace_id = ?", id).Find(&out).Error; err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			return
		}
		_ = ctx.JSON(out)
	})*/

	// we mounted routes directly, so nothing to add to r.Routes
	return nil
}
