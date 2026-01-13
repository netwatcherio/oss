package web

import (
	"context"
	"strconv"
	"strings"

	"netwatcher-controller/internal/admin"
	"netwatcher-controller/internal/agent"
	"netwatcher-controller/internal/users"
	"netwatcher-controller/internal/workspace"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
)

// RegisterAdminRoutes mounts admin API endpoints under /admin
// All routes require SITE_ADMIN role via AdminMiddleware
func RegisterAdminRoutes(api iris.Party, db *gorm.DB) {
	adminAPI := api.Party("/admin")
	adminAPI.Use(AdminMiddleware(db))

	// Stats
	adminAPI.Get("/stats", adminStatsHandler(db))
	adminAPI.Get("/workspace-stats", adminWorkspaceStatsHandler(db))

	// Users
	adminAPI.Get("/users", adminListUsersHandler(db))
	adminAPI.Get("/users/{id:uint}", adminGetUserHandler(db))
	adminAPI.Put("/users/{id:uint}", adminUpdateUserHandler(db))
	adminAPI.Delete("/users/{id:uint}", adminDeleteUserHandler(db))
	adminAPI.Put("/users/{id:uint}/role", adminSetUserRoleHandler(db))

	// Workspaces
	adminAPI.Get("/workspaces", adminListWorkspacesHandler(db))
	adminAPI.Get("/workspaces/{id:uint}", adminGetWorkspaceHandler(db))
	adminAPI.Put("/workspaces/{id:uint}", adminUpdateWorkspaceHandler(db))
	adminAPI.Delete("/workspaces/{id:uint}", adminDeleteWorkspaceHandler(db))

	// Workspace members
	adminAPI.Get("/workspaces/{id:uint}/members", adminListMembersHandler(db))
	adminAPI.Post("/workspaces/{id:uint}/members", adminAddMemberHandler(db))
	adminAPI.Put("/workspaces/{wID:uint}/members/{mID:uint}", adminUpdateMemberHandler(db))
	adminAPI.Delete("/workspaces/{wID:uint}/members/{mID:uint}", adminRemoveMemberHandler(db))

	// Agents
	adminAPI.Get("/agents", adminListAgentsHandler(db))
	adminAPI.Get("/agents/stats", adminAgentStatsHandler(db))
}

// ==================== Stats ====================

func adminStatsHandler(db *gorm.DB) iris.Handler {
	return func(ctx iris.Context) {
		stats, err := admin.GetStats(context.Background(), db)
		if err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(stats)
	}
}

func adminWorkspaceStatsHandler(db *gorm.DB) iris.Handler {
	return func(ctx iris.Context) {
		stats, err := admin.GetWorkspaceStats(context.Background(), db)
		if err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(iris.Map{"data": stats})
	}
}

// ==================== Users ====================

func adminListUsersHandler(db *gorm.DB) iris.Handler {
	return func(ctx iris.Context) {
		limit, _ := strconv.Atoi(ctx.URLParamDefault("limit", "50"))
		offset, _ := strconv.Atoi(ctx.URLParamDefault("offset", "0"))
		query := ctx.URLParam("q")

		usersList, total, err := users.List(context.Background(), db, limit, offset, query)
		if err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		_ = ctx.JSON(iris.Map{
			"data":   usersList,
			"total":  total,
			"limit":  limit,
			"offset": offset,
		})
	}
}

func adminGetUserHandler(db *gorm.DB) iris.Handler {
	return func(ctx iris.Context) {
		id, _ := ctx.Params().GetUint("id")
		user, err := users.Get(context.Background(), db, id)
		if err != nil {
			if err == users.ErrNotFound {
				ctx.StatusCode(iris.StatusNotFound)
				_ = ctx.JSON(iris.Map{"error": "user not found"})
				return
			}
			ctx.StatusCode(iris.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(user)
	}
}

type adminUpdateUserInput struct {
	Email    *string `json:"email"`
	Name     *string `json:"name"`
	Verified *bool   `json:"verified"`
}

func adminUpdateUserHandler(db *gorm.DB) iris.Handler {
	return func(ctx iris.Context) {
		id, _ := ctx.Params().GetUint("id")

		var input adminUpdateUserInput
		if err := ctx.ReadJSON(&input); err != nil {
			ctx.StatusCode(iris.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "invalid request body"})
			return
		}

		// Update profile fields
		if input.Email != nil || input.Name != nil {
			profileInput := users.UpdateProfileInput{
				Email: input.Email,
				Name:  input.Name,
			}
			if err := users.UpdateProfile(context.Background(), db, id, profileInput); err != nil {
				if err == users.ErrNotFound {
					ctx.StatusCode(iris.StatusNotFound)
					_ = ctx.JSON(iris.Map{"error": "user not found"})
					return
				}
				ctx.StatusCode(iris.StatusInternalServerError)
				_ = ctx.JSON(iris.Map{"error": err.Error()})
				return
			}
		}

		// Update verified status
		if input.Verified != nil && *input.Verified {
			_ = users.MarkVerified(context.Background(), db, id)
		}

		user, err := users.Get(context.Background(), db, id)
		if err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(user)
	}
}

func adminDeleteUserHandler(db *gorm.DB) iris.Handler {
	return func(ctx iris.Context) {
		id, _ := ctx.Params().GetUint("id")

		// Prevent self-deletion
		currentUser := ctx.Values().Get("user").(*users.User)
		if currentUser.ID == id {
			ctx.StatusCode(iris.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "cannot delete yourself"})
			return
		}

		if err := users.Delete(context.Background(), db, id); err != nil {
			if err == users.ErrNotFound {
				ctx.StatusCode(iris.StatusNotFound)
				_ = ctx.JSON(iris.Map{"error": "user not found"})
				return
			}
			ctx.StatusCode(iris.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		_ = ctx.JSON(iris.Map{"ok": true})
	}
}

type adminSetRoleInput struct {
	Role string `json:"role"`
}

func adminSetUserRoleHandler(db *gorm.DB) iris.Handler {
	return func(ctx iris.Context) {
		id, _ := ctx.Params().GetUint("id")

		var input adminSetRoleInput
		if err := ctx.ReadJSON(&input); err != nil {
			ctx.StatusCode(iris.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "invalid request body"})
			return
		}

		role := strings.TrimSpace(input.Role)
		if role != "USER" && role != admin.SiteAdminRole {
			ctx.StatusCode(iris.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "role must be USER or SITE_ADMIN"})
			return
		}

		// Prevent self-demotion
		currentUser := ctx.Values().Get("user").(*users.User)
		if currentUser.ID == id && role != admin.SiteAdminRole {
			ctx.StatusCode(iris.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "cannot demote yourself"})
			return
		}

		if err := users.SetRole(context.Background(), db, id, role); err != nil {
			if err == users.ErrNotFound {
				ctx.StatusCode(iris.StatusNotFound)
				_ = ctx.JSON(iris.Map{"error": "user not found"})
				return
			}
			ctx.StatusCode(iris.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		user, _ := users.Get(context.Background(), db, id)
		_ = ctx.JSON(user)
	}
}

// ==================== Workspaces ====================

func adminListWorkspacesHandler(db *gorm.DB) iris.Handler {
	return func(ctx iris.Context) {
		store := workspace.NewStore(db)
		limit, _ := strconv.Atoi(ctx.URLParamDefault("limit", "50"))
		offset, _ := strconv.Atoi(ctx.URLParamDefault("offset", "0"))
		query := ctx.URLParam("q")

		workspaces, err := store.ListWorkspaces(context.Background(), workspace.ListWorkspacesFilter{
			Query:  query,
			Limit:  limit,
			Offset: offset,
		})
		if err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		_ = ctx.JSON(iris.Map{
			"data":   workspaces,
			"limit":  limit,
			"offset": offset,
		})
	}
}

func adminGetWorkspaceHandler(db *gorm.DB) iris.Handler {
	return func(ctx iris.Context) {
		store := workspace.NewStore(db)
		id, _ := ctx.Params().GetUint("id")

		ws, err := store.GetWorkspace(context.Background(), id)
		if err != nil {
			if err == workspace.ErrNotFound {
				ctx.StatusCode(iris.StatusNotFound)
				_ = ctx.JSON(iris.Map{"error": "workspace not found"})
				return
			}
			ctx.StatusCode(iris.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		// Get members
		members, _ := store.ListMembers(context.Background(), id)

		// Get agents
		agents, _, _ := agent.ListAgentsByWorkspace(context.Background(), db, id, 100, 0)

		_ = ctx.JSON(iris.Map{
			"workspace": ws,
			"members":   members,
			"agents":    agents,
		})
	}
}

func adminUpdateWorkspaceHandler(db *gorm.DB) iris.Handler {
	return func(ctx iris.Context) {
		store := workspace.NewStore(db)
		id, _ := ctx.Params().GetUint("id")

		var input struct {
			Name        *string `json:"name"`
			Description *string `json:"description"`
		}
		if err := ctx.ReadJSON(&input); err != nil {
			ctx.StatusCode(iris.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "invalid request body"})
			return
		}

		ws, err := store.UpdateWorkspace(context.Background(), id, workspace.UpdateWorkspaceInput{
			Name:        input.Name,
			Description: input.Description,
		})
		if err != nil {
			if err == workspace.ErrNotFound {
				ctx.StatusCode(iris.StatusNotFound)
				_ = ctx.JSON(iris.Map{"error": "workspace not found"})
				return
			}
			ctx.StatusCode(iris.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(ws)
	}
}

func adminDeleteWorkspaceHandler(db *gorm.DB) iris.Handler {
	return func(ctx iris.Context) {
		store := workspace.NewStore(db)
		id, _ := ctx.Params().GetUint("id")

		if err := store.DeleteWorkspace(context.Background(), id); err != nil {
			if err == workspace.ErrNotFound {
				ctx.StatusCode(iris.StatusNotFound)
				_ = ctx.JSON(iris.Map{"error": "workspace not found"})
				return
			}
			ctx.StatusCode(iris.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(iris.Map{"ok": true})
	}
}

// ==================== Workspace Members ====================

func adminListMembersHandler(db *gorm.DB) iris.Handler {
	return func(ctx iris.Context) {
		store := workspace.NewStore(db)
		id, _ := ctx.Params().GetUint("id")

		members, err := store.ListMembers(context.Background(), id)
		if err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(iris.Map{"data": members})
	}
}

type adminAddMemberInput struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
}

func adminAddMemberHandler(db *gorm.DB) iris.Handler {
	return func(ctx iris.Context) {
		store := workspace.NewStore(db)
		wsID, _ := ctx.Params().GetUint("id")

		var input adminAddMemberInput
		if err := ctx.ReadJSON(&input); err != nil {
			ctx.StatusCode(iris.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "invalid request body"})
			return
		}

		role := workspace.Role(strings.ToUpper(input.Role))
		if !role.Valid() {
			ctx.StatusCode(iris.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "invalid role"})
			return
		}

		member, err := store.AddMember(context.Background(), workspace.AddMemberInput{
			WorkspaceID: wsID,
			UserID:      input.UserID,
			Email:       input.Email,
			Role:        role,
		})
		if err != nil {
			if err == workspace.ErrAlreadyExists {
				ctx.StatusCode(iris.StatusConflict)
				_ = ctx.JSON(iris.Map{"error": "member already exists"})
				return
			}
			ctx.StatusCode(iris.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(member)
	}
}

func adminUpdateMemberHandler(db *gorm.DB) iris.Handler {
	return func(ctx iris.Context) {
		store := workspace.NewStore(db)
		mID, _ := ctx.Params().GetUint("mID")

		var input struct {
			Role string `json:"role"`
		}
		if err := ctx.ReadJSON(&input); err != nil {
			ctx.StatusCode(iris.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "invalid request body"})
			return
		}

		role := workspace.Role(strings.ToUpper(input.Role))
		if !role.Valid() {
			ctx.StatusCode(iris.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "invalid role"})
			return
		}

		member, err := store.UpdateMemberRole(context.Background(), mID, role)
		if err != nil {
			if err == workspace.ErrNotFound {
				ctx.StatusCode(iris.StatusNotFound)
				_ = ctx.JSON(iris.Map{"error": "member not found"})
				return
			}
			ctx.StatusCode(iris.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(member)
	}
}

func adminRemoveMemberHandler(db *gorm.DB) iris.Handler {
	return func(ctx iris.Context) {
		store := workspace.NewStore(db)
		mID, _ := ctx.Params().GetUint("mID")

		if err := store.RemoveMember(context.Background(), mID); err != nil {
			if err == workspace.ErrNotFound {
				ctx.StatusCode(iris.StatusNotFound)
				_ = ctx.JSON(iris.Map{"error": "member not found"})
				return
			}
			ctx.StatusCode(iris.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(iris.Map{"ok": true})
	}
}

// ==================== Agents ====================

func adminListAgentsHandler(db *gorm.DB) iris.Handler {
	return func(ctx iris.Context) {
		limit, _ := strconv.Atoi(ctx.URLParamDefault("limit", "50"))
		offset, _ := strconv.Atoi(ctx.URLParamDefault("offset", "0"))

		agents, total, err := admin.ListAllAgents(context.Background(), db, limit, offset)
		if err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		_ = ctx.JSON(iris.Map{
			"data":   agents,
			"total":  total,
			"limit":  limit,
			"offset": offset,
		})
	}
}

func adminAgentStatsHandler(db *gorm.DB) iris.Handler {
	return func(ctx iris.Context) {
		stats, err := admin.GetWorkspaceStats(context.Background(), db)
		if err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(iris.Map{"data": stats})
	}
}
