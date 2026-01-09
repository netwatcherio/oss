// web/invite.go
package web

import (
	"net/http"
	"strings"
	"time"

	"netwatcher-controller/internal/email"
	"netwatcher-controller/internal/users"
	"netwatcher-controller/internal/workspace"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
)

// RegisterInviteRoutes registers public invite endpoints (no auth required)
func RegisterInviteRoutes(app *iris.Application, db *gorm.DB, emailStore *email.QueueStore) {
	store := workspace.NewStore(db)

	// GET /invite/{token} - Validate token and get invite info
	app.Get("/invite/{token}", func(ctx iris.Context) {
		token := ctx.Params().Get("token")
		if token == "" {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "token required"})
			return
		}

		info, err := store.GetInviteInfo(ctx.Request().Context(), token)
		if err != nil {
			status := http.StatusBadRequest
			switch err {
			case workspace.ErrInviteTokenInvalid:
				status = http.StatusNotFound
			case workspace.ErrInviteTokenExpired:
				status = http.StatusGone
			case workspace.ErrAlreadyExists:
				status = http.StatusConflict
			}
			ctx.StatusCode(status)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		_ = ctx.JSON(info)
	})

	// POST /invite/{token}/complete - Complete registration and accept invite
	app.Post("/invite/{token}/complete", func(ctx iris.Context) {
		token := ctx.Params().Get("token")
		if token == "" {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "token required"})
			return
		}

		var body struct {
			Name     string `json:"name"`
			Password string `json:"password"`
		}
		if err := ctx.ReadJSON(&body); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "invalid json"})
			return
		}

		if strings.TrimSpace(body.Password) == "" {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "password required"})
			return
		}

		// Get invite info first
		info, err := store.GetInviteInfo(ctx.Request().Context(), token)
		if err != nil {
			status := http.StatusBadRequest
			switch err {
			case workspace.ErrInviteTokenInvalid:
				status = http.StatusNotFound
			case workspace.ErrInviteTokenExpired:
				status = http.StatusGone
			case workspace.ErrAlreadyExists:
				status = http.StatusConflict
			}
			ctx.StatusCode(status)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		// Get or create user
		user, created, err := users.GetOrCreatePendingUser(ctx.Request().Context(), db, info.Email, body.Name)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		// If user was created or is pending, complete their registration
		if created || users.IsPendingUser(user) {
			if err := users.CompleteRegistration(ctx.Request().Context(), db, user.ID, body.Name, body.Password); err != nil {
				ctx.StatusCode(http.StatusBadRequest)
				_ = ctx.JSON(iris.Map{"error": err.Error()})
				return
			}
		}

		// Complete the invite - link member to user
		member, err := store.CompleteInviteWithToken(ctx.Request().Context(), token, user.ID)
		if err != nil {
			status := http.StatusBadRequest
			switch err {
			case workspace.ErrInviteTokenInvalid, workspace.ErrInviteTokenExpired:
				status = http.StatusBadRequest
			case workspace.ErrAlreadyExists:
				status = http.StatusConflict
			}
			ctx.StatusCode(status)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		// Create session and return JWT
		session, err := users.CreateUserSession(ctx.Request().Context(), db, user.ID, 24*time.Hour*30) // 30 days
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": "failed to create session"})
			return
		}

		jwtToken, err := users.SignUserToken(session.SessionID, user.ID, 24*time.Hour*30)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": "failed to sign token"})
			return
		}

		// Refresh user data after updates
		user, _ = users.Get(ctx.Request().Context(), db, user.ID)

		_ = ctx.JSON(iris.Map{
			"token":        jwtToken,
			"user":         user,
			"member":       member,
			"workspace_id": info.WorkspaceID,
		})
	})
}

// InviteMemberWithEmail adds a member to workspace
// If user already has an account, adds them directly
// If user doesn't exist, creates invite and queues email
func InviteMemberWithEmail(
	ctx iris.Context,
	db *gorm.DB,
	store *workspace.Store,
	emailStore *email.QueueStore,
	wsID uint,
	wsName string,
	inviteeEmail string,
	role workspace.Role,
	invitedByUserID uint,
) (*workspace.Member, error) {
	// Check if user already exists with this email
	existingUser, err := users.GetByEmail(ctx.Request().Context(), db, inviteeEmail)
	if err == nil && existingUser != nil {
		// User exists - add them directly to workspace (no invite needed)
		member, err := store.AddMember(ctx.Request().Context(), workspace.AddMemberInput{
			WorkspaceID: wsID,
			UserID:      existingUser.ID,
			Email:       inviteeEmail,
			Role:        role,
		})
		if err != nil {
			return nil, err
		}
		// Note: No invite email sent for existing users - they're added directly
		return member, nil
	}

	// User doesn't exist - create invite with token
	member, token, err := store.CreateInvite(ctx.Request().Context(), workspace.CreateInviteInput{
		WorkspaceID: wsID,
		Email:       inviteeEmail,
		Role:        role,
		InvitedBy:   invitedByUserID,
	})
	if err != nil {
		return nil, err
	}

	// Queue invite email (pass token, the template builds the URL from PANEL_ENDPOINT)
	if emailStore != nil && token != "" {
		if err := emailStore.EnqueueInvite(
			ctx.Request().Context(),
			member.Email,
			"", // name not known yet
			token,
			wsName,
			wsID,
			member.ID,
		); err != nil {
			// Log but don't fail - member was created
			// The admin can resend the invite if needed
			_ = err
		} else {
			// Mark email as sent
			_ = store.MarkInviteEmailSent(ctx.Request().Context(), member.ID)
		}
	}

	return member, nil
}
