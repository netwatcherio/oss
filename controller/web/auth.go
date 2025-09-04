// web/auth.go
package web

import (
	"net/http"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
	"netwatcher-controller/internal/users"
)

func registerAuthRoutes(app *iris.Application, db *gorm.DB) {
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
		_ = ctx.JSON(iris.Map{"token": token, "data": u})
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
