// web/workspaces.go
package web

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"netwatcher-controller/internal/agent"
	"netwatcher-controller/internal/alert"
	"netwatcher-controller/internal/email"
	"netwatcher-controller/internal/limits"
	"netwatcher-controller/internal/users"
	"netwatcher-controller/internal/workspace"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func panelWorkspaces(api fiber.Router, db *gorm.DB, emailStore *email.QueueStore, limitsConfig *limits.Config) {
	wsParty := api.Group("/workspaces")
	store := workspace.NewStore(db)

	// GET /workspaces - returns all workspaces where user is a member, with stats
	wsParty.Get("/", func(c *fiber.Ctx) error {
		uid := currentUserID(c)
		workspaces, err := store.ListWorkspacesByUserID(c.UserContext(), uid)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		// Build enriched response with stats for each workspace
		type WorkspaceWithStats struct {
			workspace.Workspace
			AgentCount   int `json:"agent_count"`
			OnlineAgents int `json:"online_agents"`
			MemberCount  int `json:"member_count"`
			AlertCount   int `json:"alert_count"`
		}

		result := make([]WorkspaceWithStats, 0, len(workspaces))
		onlineThreshold := time.Now().Add(-2 * time.Minute) // Consider online if seen in last 2 minutes

		for _, ws := range workspaces {
			stats := WorkspaceWithStats{Workspace: ws}

			// Get agent counts
			agents, _, _ := agent.ListAgentsByWorkspace(c.UserContext(), db, ws.ID, 1000, 0)
			stats.AgentCount = len(agents)

			// Count online agents
			for _, a := range agents {
				if a.UpdatedAt.After(onlineThreshold) {
					stats.OnlineAgents++
				}
			}

			// Get member count
			members, _ := store.ListMembers(c.UserContext(), ws.ID)
			stats.MemberCount = len(members)

			// Get active alerts count
			activeStatus := alert.StatusActive
			alerts, _ := alert.ListAlerts(c.UserContext(), db, &ws.ID, &activeStatus, 0)
			stats.AlertCount = len(alerts)

			result = append(result, stats)
		}

		return c.JSON(result)
	})

	// POST /workspaces
	wsParty.Post("/", func(c *fiber.Ctx) error {
		uid := currentUserID(c)

		// Check email verification requirement
		if isEmailVerificationRequired() {
			userVal := c.Locals("user")
			if userVal != nil {
				if user, ok := userVal.(*users.User); ok && !user.Verified {
					return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "email_verification_required", "message": "Please verify your email before creating a workspace"})
				}
			}
		}

		var body struct {
			Name        string         `json:"name"`
			DisplayName string         `json:"displayName"`
			Settings    map[string]any `json:"settings"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid json"})
		}
		in := workspace.CreateWorkspaceInput{
			Name:        body.Name,
			OwnerID:     uid,
			Description: body.DisplayName,
			Settings:    jsonFromMap(body.Settings),
		}
		ws, err := store.CreateWorkspace(c.UserContext(), in)
		if err != nil {
			status := http.StatusBadRequest
			if err == workspace.ErrAlreadyExists {
				status = http.StatusConflict
			}
			return c.Status(status).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(http.StatusCreated).JSON(ws)
	})

	// /workspaces/:id
	wsID := wsParty.Group("/:id")

	// Apply permission middleware to all workspace ID routes
	wsID.Use(RequireWorkspaceAccess(store))

	// GET /workspaces/:id - requires CanView (any member)
	wsID.Get("/", func(c *fiber.Ctx) error {
		id := uintParam(c, "id")
		userID := currentUserID(c)
		ws, err := store.GetWorkspace(c.UserContext(), id)
		if err != nil || ws == nil {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "not found"})
		}
		// Add user's role to response
		member, _ := store.GetMemberByUserID(c.UserContext(), id, userID)
		response := fiber.Map{
			"id":          ws.ID,
			"name":        ws.Name,
			"description": ws.Description,
			"owner_id":    ws.OwnerID,
			"settings":    ws.Settings,
			"created_at":  ws.CreatedAt,
			"updated_at":  ws.UpdatedAt,
		}
		if member != nil {
			response["my_role"] = member.Role
		}
		return c.JSON(response)
	})

	// PATCH /workspaces/:id - requires CanManage (ADMIN+)
	wsID.Patch("/", RequireRole(store, CanManage), func(c *fiber.Ctx) error {
		id := uintParam(c, "id")
		var body struct {
			Name        *string         `json:"name"`
			Description *string         `json:"description"`
			Settings    *map[string]any `json:"settings"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid json"})
		}
		in := workspace.UpdateWorkspaceInput{
			Description: body.Description,
			Name:        body.Name,
			Settings:    jsonPtrFromMap(body.Settings),
		}
		ws, err := store.UpdateWorkspace(c.UserContext(), id, in)
		if err != nil {
			status := http.StatusBadRequest
			if err == workspace.ErrNotFound {
				status = http.StatusNotFound
			}
			return c.Status(status).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(ws)
	})

	// DELETE /workspaces/:id - requires CanOwn (OWNER only)
	wsID.Delete("/", RequireRole(store, CanOwn), func(c *fiber.Ctx) error {
		id := uintParam(c, "id")
		err := store.DeleteWorkspace(c.UserContext(), id)
		if err != nil {
			status := http.StatusBadRequest
			if err == workspace.ErrNotFound {
				status = http.StatusNotFound
			}
			return c.Status(status).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// ----- Members -----

	// GET /workspaces/:id/members
	wsID.Get("/members", func(c *fiber.Ctx) error {
		id := uintParam(c, "id")
		ms, err := store.ListMembers(c.UserContext(), id)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(NewListResponse(ms))
	})

	// POST /workspaces/:id/members - requires CanManage (ADMIN+)
	// If userId is provided, add existing user directly
	// If only email is provided, create an invite and send email
	wsID.Post("/members", RequireRole(store, CanManage), func(c *fiber.Ctx) error {
		wsIDv := uintParam(c, "id")
		userID := currentUserID(c)
		var body struct {
			UserID uint           `json:"userId"`
			Email  string         `json:"email"`
			Role   workspace.Role `json:"role"`
			Meta   map[string]any `json:"meta"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid json"})
		}

		// Check workspace member limit
		if err := limits.CanAddMember(c.UserContext(), db, limitsConfig, wsIDv); err != nil {
			if errors.Is(err, limits.ErrMemberLimitReached) {
				return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
			}
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		// Check if the user being added has reached their workspace membership limit
		if body.UserID != 0 {
			if err := limits.CanJoinWorkspace(c.UserContext(), db, limitsConfig, body.UserID); err != nil {
				if errors.Is(err, limits.ErrWorkspaceLimitReached) {
					return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
				}
				return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
			}
		}

		// If only email provided (no userId), use invite flow
		if body.UserID == 0 && strings.TrimSpace(body.Email) != "" {
			// Get workspace name for email
			ws, err := store.GetWorkspace(c.UserContext(), wsIDv)
			if err != nil {
				return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "workspace not found"})
			}

			m, err := InviteMemberWithEmail(c.UserContext(), db, store, emailStore, wsIDv, ws.Name, body.Email, body.Role, userID)
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
				return c.Status(status).JSON(fiber.Map{"error": err.Error()})
			}
			return c.Status(http.StatusCreated).JSON(m)
		}

		// Direct add (userId provided)
		m, err := store.AddMember(c.UserContext(), workspace.AddMemberInput{
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
			return c.Status(status).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(http.StatusCreated).JSON(m)
	})

	// PATCH /workspaces/:id/members/:memberId - requires CanManage (ADMIN+)
	wsID.Patch("/members/:memberId", RequireRole(store, CanManage), func(c *fiber.Ctx) error {
		wsIDv := uintParam(c, "id")
		memberID := uintParam(c, "memberId")
		var body struct {
			Role workspace.Role `json:"role"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid json"})
		}
		m, err := store.UpdateMemberRole(c.UserContext(), wsIDv, memberID, body.Role)
		if err != nil {
			status := http.StatusBadRequest
			if err == workspace.ErrNotFound {
				status = http.StatusNotFound
			} else if err == workspace.ErrInvalidRole {
				status = http.StatusBadRequest
			}
			return c.Status(status).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(m)
	})

	// DELETE /workspaces/:id/members/:memberId - requires CanManage (ADMIN+)
	wsID.Delete("/members/:memberId", RequireRole(store, CanManage), func(c *fiber.Ctx) error {
		wsIDv := uintParam(c, "id")
		memberID := uintParam(c, "memberId")
		if err := store.RemoveMember(c.UserContext(), wsIDv, memberID); err != nil {
			status := http.StatusBadRequest
			if err == workspace.ErrNotFound {
				status = http.StatusNotFound
			}
			return c.Status(status).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// POST /workspaces/:id/accept-invite
	wsID.Post("/accept-invite", func(c *fiber.Ctx) error {
		wsIDv := uintParam(c, "id")
		var body struct {
			Email string `json:"email"`
		}
		if err := c.BodyParser(&body); err != nil || strings.TrimSpace(body.Email) == "" {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "email required"})
		}
		userID := currentUserID(c)
		m, err := store.AcceptInvite(c.UserContext(), wsIDv, body.Email, userID)
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
			return c.Status(status).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(m)
	})

	// POST /workspaces/:id/transfer-ownership
	wsID.Post("/transfer-ownership", func(c *fiber.Ctx) error {
		wsIDv := uintParam(c, "id")
		var body struct {
			NewOwnerUserID uint `json:"newOwnerUserId"`
		}
		if err := c.BodyParser(&body); err != nil || body.NewOwnerUserID == 0 {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "newOwnerUserId required"})
		}
		if err := store.TransferOwnership(c.UserContext(), wsIDv, body.NewOwnerUserID); err != nil {
			status := http.StatusBadRequest
			if err == workspace.ErrInvalidInput {
				status = http.StatusBadRequest
			}
			return c.Status(status).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true})
	})
}
