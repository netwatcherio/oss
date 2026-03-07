package web

import (
	"netwatcher-controller/internal/admin"
	"netwatcher-controller/internal/users"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// AdminMiddleware checks if the authenticated user has SITE_ADMIN role.
// Must be used after JWTMiddleware.
func AdminMiddleware(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get user from context (set by JWTMiddleware)
		userVal := c.Locals("user")
		if userVal == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}

		user, ok := userVal.(*users.User)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid user context"})
		}

		if !admin.IsSiteAdmin(user) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "site admin access required"})
		}

		return c.Next()
	}
}
