// web/auth.go
package web

import (
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

	// POST /auth/register
	auth.Post("/register", func(ctx iris.Context) {
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
			"id":    user.ID,
			"email": user.Email,
			"name":  user.Name,
			"role":  user.Role,
		})
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
		_ = ctx.JSON(iris.Map{"token": token, "data": u})
	})
}

// shouldSendRegistrationConfirmation checks if registration confirmation emails should be sent
func shouldSendRegistrationConfirmation() bool {
	v := strings.ToLower(os.Getenv("EMAIL_SEND_REGISTRATION_CONFIRMATION"))
	return v == "true" || v == "1" || v == "yes"
}
