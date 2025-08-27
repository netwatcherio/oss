package web

import (
	"net/http"

	"github.com/kataras/iris/v12"

	"netwatcher-controller/internal/auth"
)

func addRouteAuth(r *Router) []*Route {
	var routes []*Route

	// POST /auth/login
	routes = append(routes, &Route{
		Name: "Login",
		Path: "/auth/login",
		JWT:  false,
		Type: RouteType_POST,
		Func: func(ctx iris.Context) error {
			ctx.ContentType("application/json")

			var in auth.Login
			if err := ctx.ReadJSON(&in); err != nil {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}

			ip := ctx.Values().GetString("client_ip")

			token, user, err := r.AuthSvc.Login(ctx, in, ip)
			if err != nil {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}

			resp := map[string]any{
				"token": token,
				"data":  user,
			}
			return ctx.JSON(resp)
		},
	})

	// POST /auth/register
	routes = append(routes, &Route{
		Name: "Register",
		Path: "/auth/register",
		JWT:  false,
		Type: RouteType_POST,
		Func: func(ctx iris.Context) error {
			ctx.ContentType("application/json")

			var in auth.Register
			if err := ctx.ReadJSON(&in); err != nil {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}

			ip := ctx.Values().GetString("client_ip")

			token, user, err := r.AuthSvc.Register(ctx, in, ip)
			if err != nil {
				// conflict is reasonable for "email already exists", otherwise 400/500; keep 409 to match old behavior
				ctx.StatusCode(http.StatusConflict)
				return nil
			}

			resp := map[string]any{
				"token": token,
				"data":  user,
			}
			return ctx.JSON(resp)
		},
	})

	return routes
}
