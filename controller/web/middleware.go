// web/middleware.go
package web

import (
	"net/http"
	"strings"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
	"netwatcher-controller/internal/users"
)

const (
	ctxUserKey    = "user"
	ctxUserIDKey  = "userID"
	ctxSessionKey = "session"
)

// JWTMiddleware validates Authorization: Bearer <token> using internal/users.
func JWTMiddleware(db *gorm.DB) iris.Handler {
	return func(ctx iris.Context) {
		auth := ctx.GetHeader("Authorization")
		const pref = "Bearer "
		if !strings.HasPrefix(auth, pref) {
			ctx.StatusCode(http.StatusUnauthorized)
			return
		}
		tok := strings.TrimSpace(auth[len(pref):])
		u, sess, err := users.GetUserFromToken(ctx.Request().Context(), db, tok)
		if err != nil || u == nil || sess == nil {
			ctx.StatusCode(http.StatusUnauthorized)
			return
		}
		ctx.Values().Set(ctxUserKey, u)
		ctx.Values().Set(ctxUserIDKey, u.ID)
		ctx.Values().Set(ctxSessionKey, sess)
		ctx.Next()
	}
}

func currentUserID(ctx iris.Context) uint {
	if v := ctx.Values().Get(ctxUserIDKey); v != nil {
		if id, ok := v.(uint); ok {
			return id
		}
	}
	return 0
}
