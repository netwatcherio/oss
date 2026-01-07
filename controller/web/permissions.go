package web

import (
	"net/http"

	"netwatcher-controller/internal/workspace"

	"github.com/kataras/iris/v12"
)

// Permission levels for code clarity
const (
	CanView   = workspace.RoleReadOnly  // VIEWER - read-only access
	CanEdit   = workspace.RoleReadWrite // USER - create/edit agents and probes
	CanManage = workspace.RoleAdmin     // ADMIN - manage members, delete resources
	CanOwn    = workspace.RoleOwner     // OWNER - full control, workspace deletion
)

// RequireRole returns middleware that checks if the user has at least the specified role
// in the workspace identified by the "id" URL parameter.
func RequireRole(store *workspace.Store, minRole workspace.Role) iris.Handler {
	return func(ctx iris.Context) {
		userID := currentUserID(ctx)
		if userID == 0 {
			ctx.StatusCode(http.StatusUnauthorized)
			_ = ctx.JSON(iris.Map{"error": "authentication required"})
			return
		}

		wsID := uintParam(ctx, "id")
		if wsID == 0 {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "workspace id required"})
			return
		}

		if !store.UserHasRole(ctx.Request().Context(), wsID, userID, minRole) {
			ctx.StatusCode(http.StatusForbidden)
			_ = ctx.JSON(iris.Map{
				"error":         "insufficient permissions",
				"required_role": string(minRole),
			})
			return
		}

		ctx.Next()
	}
}

// RequireWorkspaceAccess is a simpler middleware that only checks if the user
// has any access to the workspace (any role).
func RequireWorkspaceAccess(store *workspace.Store) iris.Handler {
	return func(ctx iris.Context) {
		userID := currentUserID(ctx)
		if userID == 0 {
			ctx.StatusCode(http.StatusUnauthorized)
			_ = ctx.JSON(iris.Map{"error": "authentication required"})
			return
		}

		wsID := uintParam(ctx, "id")
		if wsID == 0 {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "workspace id required"})
			return
		}

		if !store.UserHasAccess(ctx.Request().Context(), wsID, userID) {
			ctx.StatusCode(http.StatusForbidden)
			_ = ctx.JSON(iris.Map{"error": "access denied"})
			return
		}

		ctx.Next()
	}
}

// RequireAgentRole checks permission for an agent within a workspace.
// It extracts workspaceId from "id" param and validates the user's role.
func RequireAgentRole(store *workspace.Store, minRole workspace.Role) iris.Handler {
	return RequireRole(store, minRole)
}

// RequireProbeRole checks permission for a probe within a workspace.
// It extracts workspaceId from "id" param and validates the user's role.
func RequireProbeRole(store *workspace.Store, minRole workspace.Role) iris.Handler {
	return RequireRole(store, minRole)
}

// RequireMemberManagement checks if user can manage members (ADMIN or higher)
// with additional validation that ADMIN cannot modify OWNER or other ADMINs.
func RequireMemberManagement(store *workspace.Store) iris.Handler {
	return func(ctx iris.Context) {
		userID := currentUserID(ctx)
		if userID == 0 {
			ctx.StatusCode(http.StatusUnauthorized)
			_ = ctx.JSON(iris.Map{"error": "authentication required"})
			return
		}

		wsID := uintParam(ctx, "id")
		if wsID == 0 {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "workspace id required"})
			return
		}

		// Must be at least ADMIN
		if !store.UserHasRole(ctx.Request().Context(), wsID, userID, workspace.RoleAdmin) {
			ctx.StatusCode(http.StatusForbidden)
			_ = ctx.JSON(iris.Map{
				"error":         "insufficient permissions",
				"required_role": string(workspace.RoleAdmin),
			})
			return
		}

		ctx.Next()
	}
}
