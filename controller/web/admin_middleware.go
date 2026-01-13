package web

import (
	"netwatcher-controller/internal/admin"
	"netwatcher-controller/internal/users"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
)

// AdminMiddleware checks if the authenticated user has SITE_ADMIN role.
// Must be used after JWTMiddleware.
func AdminMiddleware(db *gorm.DB) iris.Handler {
	return func(ctx iris.Context) {
		// Get user from context (set by JWTMiddleware)
		userVal := ctx.Values().Get("user")
		if userVal == nil {
			ctx.StatusCode(iris.StatusUnauthorized)
			_ = ctx.JSON(iris.Map{"error": "unauthorized"})
			return
		}

		user, ok := userVal.(*users.User)
		if !ok {
			ctx.StatusCode(iris.StatusUnauthorized)
			_ = ctx.JSON(iris.Map{"error": "invalid user context"})
			return
		}

		if !admin.IsSiteAdmin(user) {
			ctx.StatusCode(iris.StatusForbidden)
			_ = ctx.JSON(iris.Map{"error": "site admin access required"})
			return
		}

		ctx.Next()
	}
}
