package web

import (
	"net/http"

	"netwatcher-controller/internal/workspace"

	"github.com/gofiber/fiber/v2"
)

// Permission levels for code clarity
const (
	CanView   = workspace.RoleViewer // VIEWER - read-only access
	CanEdit   = workspace.RoleUser   // USER - create/edit agents and probes
	CanManage = workspace.RoleAdmin  // ADMIN - manage members, delete resources
	CanOwn    = workspace.RoleOwner  // OWNER - full control, workspace deletion
)

// RequireRole returns middleware that checks if the user has at least the specified role
// in the workspace identified by the "id" URL parameter.
func RequireRole(store *workspace.Store, minRole workspace.Role) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := currentUserID(c)
		if userID == 0 {
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "authentication required"})
		}

		wsID := uintParam(c, "id")
		if wsID == 0 {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "workspace id required"})
		}

		if !store.UserHasRole(c.UserContext(), wsID, userID, minRole) {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{
				"error":         "insufficient permissions",
				"required_role": string(minRole),
			})
		}

		return c.Next()
	}
}

// RequireWorkspaceAccess is a simpler middleware that only checks if the user
// has any access to the workspace (any role).
func RequireWorkspaceAccess(store *workspace.Store) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := currentUserID(c)
		if userID == 0 {
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "authentication required"})
		}

		wsID := uintParam(c, "id")
		if wsID == 0 {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "workspace id required"})
		}

		if !store.UserHasAccess(c.UserContext(), wsID, userID) {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
		}

		return c.Next()
	}
}

// RequireAgentRole checks permission for an agent within a workspace.
// It extracts workspaceId from "id" param and validates the user's role.
func RequireAgentRole(store *workspace.Store, minRole workspace.Role) fiber.Handler {
	return RequireRole(store, minRole)
}

// RequireProbeRole checks permission for a probe within a workspace.
// It extracts workspaceId from "id" param and validates the user's role.
func RequireProbeRole(store *workspace.Store, minRole workspace.Role) fiber.Handler {
	return RequireRole(store, minRole)
}

// RequireMemberManagement checks if user can manage members (ADMIN or higher)
// with additional validation that ADMIN cannot modify OWNER or other ADMINs.
func RequireMemberManagement(store *workspace.Store) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := currentUserID(c)
		if userID == 0 {
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "authentication required"})
		}

		wsID := uintParam(c, "id")
		if wsID == 0 {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "workspace id required"})
		}

		// Must be at least ADMIN
		if !store.UserHasRole(c.UserContext(), wsID, userID, workspace.RoleAdmin) {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{
				"error":         "insufficient permissions",
				"required_role": string(workspace.RoleAdmin),
			})
		}

		return c.Next()
	}
}
