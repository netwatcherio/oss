// web/auth.go
package web

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"netwatcher-controller/internal/email"
	"netwatcher-controller/internal/users"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
)

func registerAuthRoutes(app *iris.Application, db *gorm.DB, emailStore *email.QueueStore) {
	auth := app.Party("/auth")

	// GET /auth/config - public endpoint for panel to check registration settings
	auth.Get("/config", func(ctx iris.Context) {
		_ = ctx.JSON(iris.Map{
			"registration_enabled":        isRegistrationEnabled(),
			"email_verification_required": isEmailVerificationRequired(),
		})
	})

	// POST /auth/register
	auth.Post("/register", func(ctx iris.Context) {
		// Check if registration is enabled
		if !isRegistrationEnabled() {
			ctx.StatusCode(http.StatusForbidden)
			_ = ctx.JSON(iris.Map{"error": "registration is disabled"})
			return
		}

		var body struct {
			Email    string         `json:"email"`
			Password string         `json:"password"`
			Name     string         `json:"name"`
			Role     string         `json:"role"`
			Labels   map[string]any `json:"labels"`
			Metadata map[string]any `json:"metadata"`
		}
		if err := ctx.ReadJSON(&body); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			return
		}
		in := users.RegisterInput{
			Email:    body.Email,
			Password: body.Password,
			Name:     body.Name,
			Role:     body.Role,
			Labels:   jsonFromMap(body.Labels),
			Metadata: jsonFromMap(body.Metadata),
		}
		token, u, _, err := users.RegisterUser(ctx.Request().Context(), db, in, ctx.RemoteAddr())
		if err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		// Send registration confirmation email if enabled
		if emailStore != nil && shouldSendRegistrationConfirmation() {
			_ = emailStore.EnqueueRegistrationConfirmation(ctx.Request().Context(), u.Email, u.Name)
		}

		// Send verification email if required and email store is available
		if emailStore != nil && isEmailVerificationRequired() {
			verifyToken, err := users.CreateToken(ctx.Request().Context(), db, u.ID, users.TokenTypeEmailVerification, users.GetEmailVerificationExpiryHours())
			if err == nil {
				_ = emailStore.EnqueueEmailVerification(ctx.Request().Context(), u.Email, u.Name, verifyToken.Token, u.ID)
			}
		}

		_ = ctx.JSON(iris.Map{"token": token, "data": u})
	})

	// GET /auth/me - returns current authenticated user
	auth.Get("/me", JWTMiddleware(db), func(ctx iris.Context) {
		userVal := ctx.Values().Get("user")
		if userVal == nil {
			ctx.StatusCode(http.StatusUnauthorized)
			_ = ctx.JSON(iris.Map{"error": "unauthorized"})
			return
		}

		user, ok := userVal.(*users.User)
		if !ok {
			ctx.StatusCode(http.StatusUnauthorized)
			_ = ctx.JSON(iris.Map{"error": "invalid user context"})
			return
		}

		_ = ctx.JSON(iris.Map{
			"id":       user.ID,
			"email":    user.Email,
			"name":     user.Name,
			"role":     user.Role,
			"verified": user.Verified,
		})
	})

	// PUT /auth/me/password - change current user's password
	auth.Put("/me/password", JWTMiddleware(db), func(ctx iris.Context) {
		userVal := ctx.Values().Get("user")
		if userVal == nil {
			ctx.StatusCode(http.StatusUnauthorized)
			_ = ctx.JSON(iris.Map{"error": "unauthorized"})
			return
		}

		user, ok := userVal.(*users.User)
		if !ok {
			ctx.StatusCode(http.StatusUnauthorized)
			_ = ctx.JSON(iris.Map{"error": "invalid user context"})
			return
		}

		var body struct {
			OldPassword string `json:"old_password"`
			NewPassword string `json:"new_password"`
		}
		if err := ctx.ReadJSON(&body); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "invalid request body"})
			return
		}

		if body.OldPassword == "" || body.NewPassword == "" {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "old_password and new_password are required"})
			return
		}

		err := users.ChangePassword(ctx.Request().Context(), db, user.ID, users.ChangePasswordInput{
			OldPassword: body.OldPassword,
			NewPassword: body.NewPassword,
		})
		if err != nil {
			if err == users.ErrBadPassword {
				ctx.StatusCode(http.StatusUnauthorized)
				_ = ctx.JSON(iris.Map{"error": "incorrect current password"})
				return
			}
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		_ = ctx.JSON(iris.Map{"success": true})
	})

	// POST /auth/login
	auth.Post("/login", func(ctx iris.Context) {
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := ctx.ReadJSON(&body); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			return
		}
		in := users.LoginInput{Email: body.Email, Password: body.Password}
		token, u, _, err := users.LoginUser(ctx.Request().Context(), db, in, ctx.RemoteAddr())
		if err != nil {
			ctx.StatusCode(http.StatusUnauthorized)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(iris.Map{"token": token, "data": u, "email_verification_required": isEmailVerificationRequired()})
	})

	// POST /auth/verify-email - verify email with token
	auth.Post("/verify-email", func(ctx iris.Context) {
		var body struct {
			Token string `json:"token"`
		}
		if err := ctx.ReadJSON(&body); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "invalid request body"})
			return
		}

		userID, err := users.ConsumeToken(ctx.Request().Context(), db, body.Token, users.TokenTypeEmailVerification)
		if err != nil {
			status := http.StatusBadRequest
			msg := "invalid or expired token"
			if errors.Is(err, users.ErrTokenExpired) {
				msg = "token has expired"
			} else if errors.Is(err, users.ErrTokenNotFound) {
				msg = "token not found"
			}
			ctx.StatusCode(status)
			_ = ctx.JSON(iris.Map{"error": msg})
			return
		}

		// Mark user as verified
		if err := users.MarkVerified(ctx.Request().Context(), db, userID); err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": "failed to verify user"})
			return
		}

		_ = ctx.JSON(iris.Map{"success": true, "message": "email verified successfully"})
	})

	// POST /auth/resend-verification - resend verification email
	auth.Post("/resend-verification", JWTMiddleware(db), func(ctx iris.Context) {
		userVal := ctx.Values().Get("user")
		if userVal == nil {
			ctx.StatusCode(http.StatusUnauthorized)
			_ = ctx.JSON(iris.Map{"error": "unauthorized"})
			return
		}

		user, ok := userVal.(*users.User)
		if !ok {
			ctx.StatusCode(http.StatusUnauthorized)
			_ = ctx.JSON(iris.Map{"error": "invalid user context"})
			return
		}

		if user.Verified {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "email already verified"})
			return
		}

		if emailStore == nil {
			ctx.StatusCode(http.StatusServiceUnavailable)
			_ = ctx.JSON(iris.Map{"error": "email service unavailable"})
			return
		}

		// Create new verification token
		verifyToken, err := users.CreateToken(ctx.Request().Context(), db, user.ID, users.TokenTypeEmailVerification, users.GetEmailVerificationExpiryHours())
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": "failed to create verification token"})
			return
		}

		// Queue verification email
		if err := emailStore.EnqueueEmailVerification(ctx.Request().Context(), user.Email, user.Name, verifyToken.Token, user.ID); err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": "failed to queue verification email"})
			return
		}

		_ = ctx.JSON(iris.Map{"success": true, "message": "verification email sent"})
	})

	// POST /auth/forgot-password - request password reset
	auth.Post("/forgot-password", func(ctx iris.Context) {
		var body struct {
			Email string `json:"email"`
		}
		if err := ctx.ReadJSON(&body); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "invalid request body"})
			return
		}

		email := strings.ToLower(strings.TrimSpace(body.Email))
		if email == "" {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "email is required"})
			return
		}

		// Always return success to prevent email enumeration
		// But only actually send email if user exists
		user, err := users.GetByEmail(ctx.Request().Context(), db, email)
		if err == nil && user != nil && emailStore != nil {
			// Create password reset token
			resetToken, err := users.CreateToken(ctx.Request().Context(), db, user.ID, users.TokenTypePasswordReset, users.GetPasswordResetExpiryHours())
			if err == nil {
				_ = emailStore.EnqueuePasswordReset(ctx.Request().Context(), user.Email, user.Name, resetToken.Token, user.ID)
			}
		}

		// Always return success to prevent email enumeration
		_ = ctx.JSON(iris.Map{"success": true, "message": "if that email exists, a reset link has been sent"})
	})

	// POST /auth/reset-password - complete password reset with token
	auth.Post("/reset-password", func(ctx iris.Context) {
		var body struct {
			Token       string `json:"token"`
			NewPassword string `json:"new_password"`
		}
		if err := ctx.ReadJSON(&body); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "invalid request body"})
			return
		}

		if strings.TrimSpace(body.NewPassword) == "" {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "new_password is required"})
			return
		}

		userID, err := users.ConsumeToken(ctx.Request().Context(), db, body.Token, users.TokenTypePasswordReset)
		if err != nil {
			status := http.StatusBadRequest
			msg := "invalid or expired token"
			if errors.Is(err, users.ErrTokenExpired) {
				msg = "token has expired"
			} else if errors.Is(err, users.ErrTokenNotFound) {
				msg = "token not found"
			}
			ctx.StatusCode(status)
			_ = ctx.JSON(iris.Map{"error": msg})
			return
		}

		// Update user's password (without requiring old password)
		if err := users.ChangePassword(ctx.Request().Context(), db, userID, users.ChangePasswordInput{
			NewPassword: body.NewPassword,
		}); err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": "failed to update password"})
			return
		}

		_ = ctx.JSON(iris.Map{"success": true, "message": "password reset successfully"})
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
