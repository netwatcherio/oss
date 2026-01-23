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
	"netwatcher-controller/internal/workspace"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
)

func panelWorkspaces(api iris.Party, db *gorm.DB, emailStore *email.QueueStore, limitsConfig *limits.Config) {
	wsParty := api.Party("/workspaces")
	store := workspace.NewStore(db)

	// GET /workspaces - returns all workspaces where user is a member, with stats
	wsParty.Get("/", func(ctx iris.Context) {
		uid := currentUserID(ctx)
		workspaces, err := store.ListWorkspacesByUserID(ctx.Request().Context(), uid)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
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
			agents, _, _ := agent.ListAgentsByWorkspace(ctx.Request().Context(), db, ws.ID, 1000, 0)
			stats.AgentCount = len(agents)

			// Count online agents
			for _, a := range agents {
				if a.UpdatedAt.After(onlineThreshold) {
					stats.OnlineAgents++
				}
			}

			// Get member count
			members, _ := store.ListMembers(ctx.Request().Context(), ws.ID)
			stats.MemberCount = len(members)

			// Get active alerts count
			activeStatus := alert.StatusActive
			alerts, _ := alert.ListAlerts(ctx.Request().Context(), db, &ws.ID, &activeStatus, 0)
			stats.AlertCount = len(alerts)

			result = append(result, stats)
		}

		_ = ctx.JSON(result)
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

	// Apply permission middleware to all workspace ID routes
	wsID.Use(RequireWorkspaceAccess(store))

	// GET /workspaces/{id} - requires CanView (any member)
	wsID.Get("/", func(ctx iris.Context) {
		id := uintParam(ctx, "id")
		userID := currentUserID(ctx)
		ws, err := store.GetWorkspace(ctx.Request().Context(), id)
		if err != nil || ws == nil {
			ctx.StatusCode(http.StatusNotFound)
			_ = ctx.JSON(iris.Map{"error": "not found"})
			return
		}
		// Add user's role to response
		member, _ := store.GetMemberByUserID(ctx.Request().Context(), id, userID)
		response := iris.Map{
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
		_ = ctx.JSON(response)
	})

	// PATCH /workspaces/{id} - requires CanManage (ADMIN+)
	wsID.Patch("/", RequireRole(store, CanManage), func(ctx iris.Context) {
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

	// DELETE /workspaces/{id} - requires CanOwn (OWNER only)
	wsID.Delete("/", RequireRole(store, CanOwn), func(ctx iris.Context) {
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
		_ = ctx.JSON(NewListResponse(ms))
	})

	// POST /workspaces/{id}/members - requires CanManage (ADMIN+)
	// If userId is provided, add existing user directly
	// If only email is provided, create an invite and send email
	wsID.Post("/members", RequireRole(store, CanManage), func(ctx iris.Context) {
		wsIDv := uintParam(ctx, "id")
		userID := currentUserID(ctx)
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

		// Check workspace member limit
		if err := limits.CanAddMember(ctx.Request().Context(), db, limitsConfig, wsIDv); err != nil {
			if errors.Is(err, limits.ErrMemberLimitReached) {
				ctx.StatusCode(http.StatusForbidden)
				_ = ctx.JSON(iris.Map{"error": err.Error()})
				return
			}
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		// Check if the user being added has reached their workspace membership limit
		if body.UserID != 0 {
			if err := limits.CanJoinWorkspace(ctx.Request().Context(), db, limitsConfig, body.UserID); err != nil {
				if errors.Is(err, limits.ErrWorkspaceLimitReached) {
					ctx.StatusCode(http.StatusForbidden)
					_ = ctx.JSON(iris.Map{"error": err.Error()})
					return
				}
				ctx.StatusCode(http.StatusInternalServerError)
				_ = ctx.JSON(iris.Map{"error": err.Error()})
				return
			}
		}

		// If only email provided (no userId), use invite flow
		if body.UserID == 0 && strings.TrimSpace(body.Email) != "" {
			// Get workspace name for email
			ws, err := store.GetWorkspace(ctx.Request().Context(), wsIDv)
			if err != nil {
				ctx.StatusCode(http.StatusNotFound)
				_ = ctx.JSON(iris.Map{"error": "workspace not found"})
				return
			}

			m, err := InviteMemberWithEmail(ctx, db, store, emailStore, wsIDv, ws.Name, body.Email, body.Role, userID)
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
			return
		}

		// Direct add (userId provided)
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

	// PATCH /workspaces/{id}/members/{memberId} - requires CanManage (ADMIN+)
	wsID.Patch("/members/{memberId:uint}", RequireRole(store, CanManage), func(ctx iris.Context) {
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

	// DELETE /workspaces/{id}/members/{memberId} - requires CanManage (ADMIN+)
	wsID.Delete("/members/{memberId:uint}", RequireRole(store, CanManage), func(ctx iris.Context) {
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
