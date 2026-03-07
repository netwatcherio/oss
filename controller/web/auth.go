// web/auth.go
package web

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"netwatcher-controller/internal/email"
	"netwatcher-controller/internal/users"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func registerAuthRoutes(app *fiber.App, db *gorm.DB, emailStore *email.QueueStore) {
	auth := app.Group("/auth")

	// GET /auth/config - public endpoint for panel to check registration settings
	auth.Get("/config", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"registration_enabled":        isRegistrationEnabled(),
			"email_verification_required": isEmailVerificationRequired(),
		})
	})

	// POST /auth/register
	auth.Post("/register", func(c *fiber.Ctx) error {
		// Check if registration is enabled
		if !isRegistrationEnabled() {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "registration is disabled"})
		}

		var body struct {
			Email    string         `json:"email"`
			Password string         `json:"password"`
			Name     string         `json:"name"`
			Role     string         `json:"role"`
			Labels   map[string]any `json:"labels"`
			Metadata map[string]any `json:"metadata"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.SendStatus(http.StatusBadRequest)
		}
		in := users.RegisterInput{
			Email:    body.Email,
			Password: body.Password,
			Name:     body.Name,
			Role:     body.Role,
			Labels:   jsonFromMap(body.Labels),
			Metadata: jsonFromMap(body.Metadata),
		}
		token, u, _, err := users.RegisterUser(c.UserContext(), db, in, c.IP())
		if err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}

		// Send registration confirmation email if enabled
		if emailStore != nil && shouldSendRegistrationConfirmation() {
			_ = emailStore.EnqueueRegistrationConfirmation(c.UserContext(), u.Email, u.Name)
		}

		// Send verification email if required and email store is available
		if emailStore != nil && isEmailVerificationRequired() {
			verifyToken, err := users.CreateToken(c.UserContext(), db, u.ID, users.TokenTypeEmailVerification, users.GetEmailVerificationExpiryHours())
			if err == nil {
				_ = emailStore.EnqueueEmailVerification(c.UserContext(), u.Email, u.Name, verifyToken.Token, u.ID)
			}
		}

		return c.JSON(fiber.Map{"token": token, "data": u})
	})

	// GET /auth/me - returns current authenticated user
	auth.Get("/me", JWTMiddleware(db), func(c *fiber.Ctx) error {
		userVal := c.Locals("user")
		if userVal == nil {
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}

		user, ok := userVal.(*users.User)
		if !ok {
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "invalid user context"})
		}

		return c.JSON(fiber.Map{
			"id":       user.ID,
			"email":    user.Email,
			"name":     user.Name,
			"role":     user.Role,
			"verified": user.Verified,
		})
	})

	// PUT /auth/me/password - change current user's password
	auth.Put("/me/password", JWTMiddleware(db), func(c *fiber.Ctx) error {
		userVal := c.Locals("user")
		if userVal == nil {
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}

		user, ok := userVal.(*users.User)
		if !ok {
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "invalid user context"})
		}

		var body struct {
			OldPassword string `json:"old_password"`
			NewPassword string `json:"new_password"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}

		if body.OldPassword == "" || body.NewPassword == "" {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "old_password and new_password are required"})
		}

		err := users.ChangePassword(c.UserContext(), db, user.ID, users.ChangePasswordInput{
			OldPassword: body.OldPassword,
			NewPassword: body.NewPassword,
		})
		if err != nil {
			if err == users.ErrBadPassword {
				return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "incorrect current password"})
			}
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{"success": true})
	})

	// POST /auth/login
	auth.Post("/login", func(c *fiber.Ctx) error {
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.SendStatus(http.StatusBadRequest)
		}
		in := users.LoginInput{Email: body.Email, Password: body.Password}
		token, u, _, err := users.LoginUser(c.UserContext(), db, in, c.IP())
		if err != nil {
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"token": token, "data": u, "email_verification_required": isEmailVerificationRequired()})
	})

	// POST /auth/verify-email - verify email with token
	auth.Post("/verify-email", func(c *fiber.Ctx) error {
		var body struct {
			Token string `json:"token"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}

		userID, err := users.ConsumeToken(c.UserContext(), db, body.Token, users.TokenTypeEmailVerification)
		if err != nil {
			status := http.StatusBadRequest
			msg := "invalid or expired token"
			if errors.Is(err, users.ErrTokenExpired) {
				msg = "token has expired"
			} else if errors.Is(err, users.ErrTokenNotFound) {
				msg = "token not found"
			}
			return c.Status(status).JSON(fiber.Map{"error": msg})
		}

		// Mark user as verified
		if err := users.MarkVerified(c.UserContext(), db, userID); err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "failed to verify user"})
		}

		return c.JSON(fiber.Map{"success": true, "message": "email verified successfully"})
	})

	// POST /auth/resend-verification - resend verification email
	auth.Post("/resend-verification", JWTMiddleware(db), func(c *fiber.Ctx) error {
		userVal := c.Locals("user")
		if userVal == nil {
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}

		user, ok := userVal.(*users.User)
		if !ok {
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "invalid user context"})
		}

		if user.Verified {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "email already verified"})
		}

		if emailStore == nil {
			return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{"error": "email service unavailable"})
		}

		// Create new verification token
		verifyToken, err := users.CreateToken(c.UserContext(), db, user.ID, users.TokenTypeEmailVerification, users.GetEmailVerificationExpiryHours())
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create verification token"})
		}

		// Queue verification email
		if err := emailStore.EnqueueEmailVerification(c.UserContext(), user.Email, user.Name, verifyToken.Token, user.ID); err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "failed to queue verification email"})
		}

		return c.JSON(fiber.Map{"success": true, "message": "verification email sent"})
	})

	// POST /auth/forgot-password - request password reset
	auth.Post("/forgot-password", func(c *fiber.Ctx) error {
		var body struct {
			Email string `json:"email"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}

		emailAddr := strings.ToLower(strings.TrimSpace(body.Email))
		if emailAddr == "" {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "email is required"})
		}

		// Always return success to prevent email enumeration
		// But only actually send email if user exists
		user, err := users.GetByEmail(c.UserContext(), db, emailAddr)
		if err == nil && user != nil && emailStore != nil {
			// Create password reset token
			resetToken, err := users.CreateToken(c.UserContext(), db, user.ID, users.TokenTypePasswordReset, users.GetPasswordResetExpiryHours())
			if err == nil {
				_ = emailStore.EnqueuePasswordReset(c.UserContext(), user.Email, user.Name, resetToken.Token, user.ID)
			}
		}

		// Always return success to prevent email enumeration
		return c.JSON(fiber.Map{"success": true, "message": "if that email exists, a reset link has been sent"})
	})

	// POST /auth/reset-password - complete password reset with token
	auth.Post("/reset-password", func(c *fiber.Ctx) error {
		var body struct {
			Token       string `json:"token"`
			NewPassword string `json:"new_password"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}

		if strings.TrimSpace(body.NewPassword) == "" {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "new_password is required"})
		}

		userID, err := users.ConsumeToken(c.UserContext(), db, body.Token, users.TokenTypePasswordReset)
		if err != nil {
			status := http.StatusBadRequest
			msg := "invalid or expired token"
			if errors.Is(err, users.ErrTokenExpired) {
				msg = "token has expired"
			} else if errors.Is(err, users.ErrTokenNotFound) {
				msg = "token not found"
			}
			return c.Status(status).JSON(fiber.Map{"error": msg})
		}

		// Update user's password (without requiring old password)
		if err := users.ChangePassword(c.UserContext(), db, userID, users.ChangePasswordInput{
			NewPassword: body.NewPassword,
		}); err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update password"})
		}

		return c.JSON(fiber.Map{"success": true, "message": "password reset successfully"})
	})
}

// shouldSendRegistrationConfirmation checks if registration confirmation emails should be sent
func shouldSendRegistrationConfirmation() bool {
	v := strings.ToLower(os.Getenv("EMAIL_SEND_REGISTRATION_CONFIRMATION"))
	return v == "true" || v == "1" || v == "yes"
}

// isRegistrationEnabled checks if user registration is enabled (default: true)
func isRegistrationEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("REGISTRATION_ENABLED")))
	// Default to enabled if not set or explicitly set to true
	if v == "" || v == "true" || v == "1" || v == "yes" {
		return true
	}
	return false
}

// isEmailVerificationRequired checks if email verification is required (default: false)
func isEmailVerificationRequired() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("REQUIRE_EMAIL_VERIFICATION")))
	return v == "true" || v == "1" || v == "yes"
}
