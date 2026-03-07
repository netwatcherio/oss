// web/invite.go
package web

import (
	"context"
	"net/http"
	"strings"
	"time"

	"netwatcher-controller/internal/email"
	"netwatcher-controller/internal/users"
	"netwatcher-controller/internal/workspace"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// RegisterInviteRoutes registers public invite endpoints (no auth required)
func RegisterInviteRoutes(app *fiber.App, db *gorm.DB, emailStore *email.QueueStore) {
	store := workspace.NewStore(db)

	// GET /invite/:token - Validate token and get invite info
	app.Get("/invite/:token", func(c *fiber.Ctx) error {
		token := c.Params("token")
		if token == "" {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "token required"})
		}

		info, err := store.GetInviteInfo(c.UserContext(), token)
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
			return c.Status(status).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(info)
	})

	// POST /invite/:token/complete - Complete registration and accept invite
	app.Post("/invite/:token/complete", func(c *fiber.Ctx) error {
		token := c.Params("token")
		if token == "" {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "token required"})
		}

		var body struct {
			Name     string `json:"name"`
			Password string `json:"password"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid json"})
		}

		if strings.TrimSpace(body.Password) == "" {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "password required"})
		}

		// Get invite info first
		info, err := store.GetInviteInfo(c.UserContext(), token)
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
			return c.Status(status).JSON(fiber.Map{"error": err.Error()})
		}

		// Get or create user
		user, created, err := users.GetOrCreatePendingUser(c.UserContext(), db, info.Email, body.Name)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		// If user was created or is pending, complete their registration
		if created || users.IsPendingUser(user) {
			if err := users.CompleteRegistration(c.UserContext(), db, user.ID, body.Name, body.Password); err != nil {
				return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
			}
		}

		// Complete the invite - link member to user
		member, err := store.CompleteInviteWithToken(c.UserContext(), token, user.ID)
		if err != nil {
			status := http.StatusBadRequest
			switch err {
			case workspace.ErrInviteTokenInvalid, workspace.ErrInviteTokenExpired:
				status = http.StatusBadRequest
			case workspace.ErrAlreadyExists:
				status = http.StatusConflict
			}
			return c.Status(status).JSON(fiber.Map{"error": err.Error()})
		}

		// Create session and return JWT
		session, err := users.CreateUserSession(c.UserContext(), db, user.ID, 24*time.Hour*30) // 30 days
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create session"})
		}

		jwtToken, err := users.SignUserToken(session.SessionID, user.ID, 24*time.Hour*30)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "failed to sign token"})
		}

		// Refresh user data after updates
		user, _ = users.Get(c.UserContext(), db, user.ID)

		return c.JSON(fiber.Map{
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
	ctx context.Context,
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
	existingUser, err := users.GetByEmail(ctx, db, inviteeEmail)
	if err == nil && existingUser != nil {
		// User exists - add them directly to workspace (no invite needed)
		member, err := store.AddMember(ctx, workspace.AddMemberInput{
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
	member, token, err := store.CreateInvite(ctx, workspace.CreateInviteInput{
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
			ctx,
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
			_ = store.MarkInviteEmailSent(ctx, member.ID)
		}
	}

	return member, nil
}
