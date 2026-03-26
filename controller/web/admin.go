package web

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"netwatcher-controller/internal/admin"
	"netwatcher-controller/internal/agent"
	"netwatcher-controller/internal/users"
	"netwatcher-controller/internal/workspace"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// RegisterAdminRoutes mounts admin API endpoints under /admin
// All routes require SITE_ADMIN role via AdminMiddleware
func RegisterAdminRoutes(api fiber.Router, db *gorm.DB) {
	adminAPI := api.Group("/admin")
	adminAPI.Use(AdminMiddleware(db))

	// Stats
	adminAPI.Get("/stats", adminStatsHandler(db))
	adminAPI.Get("/workspace-stats", adminWorkspaceStatsHandler(db))

	// Users
	adminAPI.Get("/users", adminListUsersHandler(db))
	adminAPI.Get("/users/:id", adminGetUserHandler(db))
	adminAPI.Put("/users/:id", adminUpdateUserHandler(db))
	adminAPI.Delete("/users/:id", adminDeleteUserHandler(db))
	adminAPI.Put("/users/:id/role", adminSetUserRoleHandler(db))

	// Workspaces
	adminAPI.Get("/workspaces", adminListWorkspacesHandler(db))
	adminAPI.Get("/workspaces/:id", adminGetWorkspaceHandler(db))
	adminAPI.Put("/workspaces/:id", adminUpdateWorkspaceHandler(db))
	adminAPI.Delete("/workspaces/:id", adminDeleteWorkspaceHandler(db))

	// Workspace members
	adminAPI.Get("/workspaces/:id/members", adminListMembersHandler(db))
	adminAPI.Post("/workspaces/:id/members", adminAddMemberHandler(db))
	adminAPI.Put("/workspaces/:wID/members/:mID", adminUpdateMemberHandler(db))
	adminAPI.Delete("/workspaces/:wID/members/:mID", adminRemoveMemberHandler(db))

	// Agents
	adminAPI.Get("/agents", adminListAgentsHandler(db))
	adminAPI.Get("/agents/stats", adminAgentStatsHandler(db))

	// Global Agents
	adminAPI.Get("/global-agents", adminListGlobalAgentsHandler(db))
	adminAPI.Put("/agents/:id/global", adminSetAgentGlobalHandler(db))

	// Debug endpoints for session/connection diagnostics
	adminAPI.Get("/debug/connections", adminDebugConnectionsHandler(db))
}

// ==================== Stats ====================

func adminStatsHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		stats, err := admin.GetStats(context.Background(), db)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(stats)
	}
}

func adminWorkspaceStatsHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		stats, err := admin.GetWorkspaceStats(context.Background(), db)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": stats})
	}
}

// ==================== Users ====================

func adminListUsersHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		limit, _ := strconv.Atoi(c.Query("limit", "50"))
		offset, _ := strconv.Atoi(c.Query("offset", "0"))
		query := c.Query("q")

		usersList, total, err := users.List(context.Background(), db, limit, offset, query)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{
			"data":   usersList,
			"total":  total,
			"limit":  limit,
			"offset": offset,
		})
	}
}

func adminGetUserHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := uintParam(c, "id")
		user, err := users.Get(context.Background(), db, id)
		if err != nil {
			if err == users.ErrNotFound {
				return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
			}
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(user)
	}
}

type adminUpdateUserInput struct {
	Email    *string `json:"email"`
	Name     *string `json:"name"`
	Verified *bool   `json:"verified"`
}

func adminUpdateUserHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := uintParam(c, "id")

		var input adminUpdateUserInput
		if err := c.BodyParser(&input); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}

		// Update profile fields
		if input.Email != nil || input.Name != nil {
			profileInput := users.UpdateProfileInput{
				Email: input.Email,
				Name:  input.Name,
			}
			if err := users.UpdateProfile(context.Background(), db, id, profileInput); err != nil {
				if err == users.ErrNotFound {
					return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
				}
				return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
			}
		}

		// Update verified status
		if input.Verified != nil && *input.Verified {
			_ = users.MarkVerified(context.Background(), db, id)
		}

		user, err := users.Get(context.Background(), db, id)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(user)
	}
}

func adminDeleteUserHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := uintParam(c, "id")

		// Prevent self-deletion
		currentUser := c.Locals("user").(*users.User)
		if currentUser.ID == id {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "cannot delete yourself"})
		}

		if err := users.Delete(context.Background(), db, id); err != nil {
			if err == users.ErrNotFound {
				return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
			}
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{"ok": true})
	}
}

type adminSetRoleInput struct {
	Role string `json:"role"`
}

func adminSetUserRoleHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := uintParam(c, "id")

		var input adminSetRoleInput
		if err := c.BodyParser(&input); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}

		role := strings.TrimSpace(input.Role)
		if role != "USER" && role != admin.SiteAdminRole {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "role must be USER or SITE_ADMIN"})
		}

		// Prevent self-demotion
		currentUser := c.Locals("user").(*users.User)
		if currentUser.ID == id && role != admin.SiteAdminRole {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "cannot demote yourself"})
		}

		if err := users.SetRole(context.Background(), db, id, role); err != nil {
			if err == users.ErrNotFound {
				return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
			}
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		user, _ := users.Get(context.Background(), db, id)
		return c.JSON(user)
	}
}

// ==================== Workspaces ====================

func adminListWorkspacesHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		store := workspace.NewStore(db)
		limit, _ := strconv.Atoi(c.Query("limit", "50"))
		offset, _ := strconv.Atoi(c.Query("offset", "0"))
		query := c.Query("q")

		workspaces, err := store.ListWorkspaces(context.Background(), workspace.ListWorkspacesFilter{
			Query:  query,
			Limit:  limit,
			Offset: offset,
		})
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{
			"data":   workspaces,
			"limit":  limit,
			"offset": offset,
		})
	}
}

func adminGetWorkspaceHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		store := workspace.NewStore(db)
		id := uintParam(c, "id")

		ws, err := store.GetWorkspace(context.Background(), id)
		if err != nil {
			if err == workspace.ErrNotFound {
				return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "workspace not found"})
			}
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		// Get members
		members, _ := store.ListMembers(context.Background(), id)

		// Get agents
		agents, _, _ := agent.ListAgentsByWorkspace(context.Background(), db, id, 100, 0)

		return c.JSON(fiber.Map{
			"workspace": ws,
			"members":   members,
			"agents":    agents,
		})
	}
}

func adminUpdateWorkspaceHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		store := workspace.NewStore(db)
		id := uintParam(c, "id")

		var input struct {
			Name        *string `json:"name"`
			Description *string `json:"description"`
		}
		if err := c.BodyParser(&input); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}

		ws, err := store.UpdateWorkspace(context.Background(), id, workspace.UpdateWorkspaceInput{
			Name:        input.Name,
			Description: input.Description,
		})
		if err != nil {
			if err == workspace.ErrNotFound {
				return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "workspace not found"})
			}
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(ws)
	}
}

func adminDeleteWorkspaceHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		store := workspace.NewStore(db)
		id := uintParam(c, "id")

		if err := store.DeleteWorkspace(context.Background(), id); err != nil {
			if err == workspace.ErrNotFound {
				return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "workspace not found"})
			}
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true})
	}
}

// ==================== Workspace Members ====================

func adminListMembersHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		store := workspace.NewStore(db)
		id := uintParam(c, "id")

		members, err := store.ListMembers(context.Background(), id)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": members})
	}
}

type adminAddMemberInput struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
}

func adminAddMemberHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		store := workspace.NewStore(db)
		wsID := uintParam(c, "id")

		var input adminAddMemberInput
		if err := c.BodyParser(&input); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}

		role := workspace.Role(strings.ToUpper(input.Role))
		if !role.Valid() {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid role"})
		}

		member, err := store.AddMember(context.Background(), workspace.AddMemberInput{
			WorkspaceID: wsID,
			UserID:      input.UserID,
			Email:       input.Email,
			Role:        role,
		})
		if err != nil {
			if err == workspace.ErrAlreadyExists {
				return c.Status(http.StatusConflict).JSON(fiber.Map{"error": "member already exists"})
			}
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(member)
	}
}

func adminUpdateMemberHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		store := workspace.NewStore(db)
		mID := uintParam(c, "mID")

		var input struct {
			Role string `json:"role"`
		}
		if err := c.BodyParser(&input); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}

		role := workspace.Role(strings.ToUpper(input.Role))
		if !role.Valid() {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid role"})
		}

		member, err := store.UpdateMemberRole(context.Background(), mID, role)
		if err != nil {
			if err == workspace.ErrNotFound {
				return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "member not found"})
			}
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(member)
	}
}

func adminRemoveMemberHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		store := workspace.NewStore(db)
		mID := uintParam(c, "mID")

		if err := store.RemoveMember(context.Background(), mID); err != nil {
			if err == workspace.ErrNotFound {
				return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "member not found"})
			}
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true})
	}
}

// ==================== Agents ====================

func adminListAgentsHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		limit, _ := strconv.Atoi(c.Query("limit", "50"))
		offset, _ := strconv.Atoi(c.Query("offset", "0"))

		agents, total, err := admin.ListAllAgents(context.Background(), db, limit, offset)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{
			"data":   agents,
			"total":  total,
			"limit":  limit,
			"offset": offset,
		})
	}
}

func adminAgentStatsHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		stats, err := admin.GetWorkspaceStats(context.Background(), db)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": stats})
	}
}

// ==================== Debug ====================

// EnrichedConnection adds friendly names to connection info
type EnrichedConnection struct {
	AgentID       uint   `json:"agent_id"`
	AgentName     string `json:"agent_name"`
	WorkspaceID   uint   `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
	ConnID        string `json:"conn_id"`
	ClientIP      string `json:"client_ip"`
	ConnectedAt   string `json:"connected_at"`
}

// WorkspaceGroup groups connections by workspace
type WorkspaceGroup struct {
	WorkspaceID   uint                 `json:"workspace_id"`
	WorkspaceName string               `json:"workspace_name"`
	AgentCount    int                  `json:"agent_count"`
	Connections   []EnrichedConnection `json:"connections"`
}

// adminDebugConnectionsHandler returns active WebSocket connection info for debugging
func adminDebugConnectionsHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		hub := GetAgentHub()
		connections := hub.GetActiveConnections()

		// Collect all agent IDs and workspace IDs
		agentIDs := make([]uint, 0, len(connections))
		workspaceIDs := make([]uint, 0, len(connections))
		for _, conn := range connections {
			agentIDs = append(agentIDs, conn.AgentID)
			workspaceIDs = append(workspaceIDs, conn.WorkspaceID)
		}

		// Fetch agent names
		agentNames := make(map[uint]string)
		if len(agentIDs) > 0 {
			var agents []struct {
				ID   uint
				Name string
			}
			db.Table("agents").Select("id, name").Where("id IN ?", agentIDs).Find(&agents)
			for _, a := range agents {
				agentNames[a.ID] = a.Name
			}
		}

		// Fetch workspace names
		workspaceNames := make(map[uint]string)
		if len(workspaceIDs) > 0 {
			var workspaces []struct {
				ID   uint
				Name string
			}
			db.Table("workspaces").Select("id, name").Where("id IN ?", workspaceIDs).Find(&workspaces)
			for _, w := range workspaces {
				workspaceNames[w.ID] = w.Name
			}
		}

		// Build enriched connections and group by workspace
		groupMap := make(map[uint]*WorkspaceGroup)
		var enriched []EnrichedConnection

		for _, conn := range connections {
			ec := EnrichedConnection{
				AgentID:       conn.AgentID,
				AgentName:     agentNames[conn.AgentID],
				WorkspaceID:   conn.WorkspaceID,
				WorkspaceName: workspaceNames[conn.WorkspaceID],
				ConnID:        conn.ConnID,
				ClientIP:      conn.ClientIP,
				ConnectedAt:   conn.ConnectedAt.Format("2006-01-02T15:04:05Z07:00"),
			}
			enriched = append(enriched, ec)

			// Group by workspace
			if _, exists := groupMap[conn.WorkspaceID]; !exists {
				groupMap[conn.WorkspaceID] = &WorkspaceGroup{
					WorkspaceID:   conn.WorkspaceID,
					WorkspaceName: workspaceNames[conn.WorkspaceID],
					Connections:   []EnrichedConnection{},
				}
			}
			groupMap[conn.WorkspaceID].Connections = append(groupMap[conn.WorkspaceID].Connections, ec)
			groupMap[conn.WorkspaceID].AgentCount++
		}

		// Convert map to slice
		groups := make([]WorkspaceGroup, 0, len(groupMap))
		for _, g := range groupMap {
			groups = append(groups, *g)
		}

		return c.JSON(fiber.Map{
			"connected_count": len(connections),
			"workspace_count": len(groups),
			"connections":     enriched,
			"by_workspace":    groups,
		})
	}
}

// ==================== Global Agents ====================

func adminListGlobalAgentsHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		agents, err := admin.ListGlobalAgents(context.Background(), db)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": agents})
	}
}

func adminSetAgentGlobalHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		agentID := uintParam(c, "id")

		var input struct {
			IsGlobal             bool  `json:"is_global"`
			BidirectionalDefault *bool `json:"bidirectional_default"`
		}
		if err := c.BodyParser(&input); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}

		// Default bidirectional to true if not specified
		bidirDefault := true
		if input.BidirectionalDefault != nil {
			bidirDefault = *input.BidirectionalDefault
		}

		if err := agent.SetGlobalStatus(context.Background(), db, agentID, input.IsGlobal, bidirDefault); err != nil {
			if err == agent.ErrNotFound {
				return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
			}
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		// Return updated agent
		a, err := agent.GetAgentByID(context.Background(), db, agentID)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(a)
	}
}
