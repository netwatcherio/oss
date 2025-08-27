package web

import (
	"net/http"

	"github.com/kataras/iris/v12"

	"netwatcher-controller/internal/auth"
)

// addRouteAuth mounts the /auth subrouter with public endpoints.
// IMPORTANT: call this BEFORE adding JWT middleware (VerifySession()) in Router.Init().
func addRouteAuth(r *Router) {
	authParty := r.App.Party("/auth") // sub-route: /auth

	// POST /auth/login
	authParty.Post("/login", func(ctx iris.Context) {
		ctx.ContentType("application/json")

		var in auth.Login
		if err := ctx.ReadJSON(&in); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			return
		}

		ip := ctx.Values().GetString("client_ip")
		token, user, err := r.AuthSvc.Login(ctx, in, ip)
		if err != nil {
			ctx.StatusCode(http.StatusUnauthorized)
			return
		}

		_ = ctx.JSON(iris.Map{
			"token": token,
			"data":  user,
		})
	})

	// POST /auth/register
	authParty.Post("/register", func(ctx iris.Context) {
		ctx.ContentType("application/json")

		var in auth.Register
		if err := ctx.ReadJSON(&in); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			return
		}

		ip := ctx.Values().GetString("client_ip")
		token, user, err := r.AuthSvc.Register(ctx, in, ip)
		if err != nil {
			// Keep 409 to mirror your prior behavior for "email exists", etc.
			ctx.StatusCode(http.StatusConflict)
			return
		}

		// 201 Created makes sense for a new account+session
		ctx.StatusCode(http.StatusCreated)
		_ = ctx.JSON(iris.Map{
			"token": token,
			"data":  user,
		})
	})
}
