// web/middleware.go
package web

import (
	"net/http"
	"strings"

	"netwatcher-controller/internal/users"
	"netwatcher-controller/internal/workspace"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

const (
	ctxUserKey    = "user"
	ctxUserIDKey  = "userID"
	ctxSessionKey = "session"
)

// JWTMiddleware validates Authorization: Bearer <token> using internal/users.
func JWTMiddleware(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		auth := c.Get("Authorization")
		const pref = "Bearer "
		if !strings.HasPrefix(auth, pref) {
			return c.SendStatus(http.StatusUnauthorized)
		}
		tok := strings.TrimSpace(auth[len(pref):])
		u, sess, err := users.GetUserFromToken(c.UserContext(), db, tok)
		if err != nil || u == nil || sess == nil {
			return c.SendStatus(http.StatusUnauthorized)
		}
		c.Locals(ctxUserKey, u)
		c.Locals(ctxUserIDKey, u.ID)
		c.Locals(ctxSessionKey, sess)
		return c.Next()
	}
}

// APIKeyAuthMiddleware validates X-API-Key header for workspace-scoped metrics endpoints.
func APIKeyAuthMiddleware(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		rawKey := c.Get("X-API-Key")
		if rawKey == "" {
			return c.SendStatus(http.StatusUnauthorized)
		}

		workspaceID, err := c.ParamsInt("id")
		if err != nil || workspaceID == 0 {
			return c.SendStatus(fiber.StatusBadRequest)
		}

		wsStore := workspace.NewStore(db)
		ak, err := wsStore.ValidateAPIKey(c.UserContext(), uint(workspaceID), rawKey)
		if err != nil {
			return c.SendStatus(http.StatusInternalServerError)
		}
		if ak == nil {
			return c.SendStatus(http.StatusUnauthorized)
		}

		return c.Next()
	}
}
