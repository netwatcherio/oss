package web

import (
	"net/http"

	"netwatcher-controller/internal/oui"

	"github.com/gofiber/fiber/v2"
)

// panelOUI registers OUI lookup endpoints.
func panelOUI(api fiber.Router, ouiStore *oui.Store) {
	// Lookup single MAC
	api.Get("/lookup/oui/:mac", func(c *fiber.Ctx) error {
		mac := c.Params("mac")
		if mac == "" {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "mac parameter required"})
		}

		if !ouiStore.IsLoaded() {
			return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
				"error":  "OUI database not loaded",
				"loaded": false,
			})
		}

		entry, found := ouiStore.Lookup(mac)
		if !found {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{
				"mac":    mac,
				"vendor": nil,
				"found":  false,
			})
		}

		return c.JSON(fiber.Map{
			"mac":    mac,
			"oui":    entry.OUI,
			"vendor": entry.Vendor,
			"found":  true,
		})
	})

	// Bulk lookup
	api.Post("/lookup/oui", func(c *fiber.Ctx) error {
		var req struct {
			MACs []string `json:"macs"`
		}

		if err := c.BodyParser(&req); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}

		if len(req.MACs) == 0 {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "macs array required"})
		}

		if len(req.MACs) > 100 {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "maximum 100 MACs per request"})
		}

		if !ouiStore.IsLoaded() {
			return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
				"error":  "OUI database not loaded",
				"loaded": false,
			})
		}

		results := make([]fiber.Map, len(req.MACs))
		for i, mac := range req.MACs {
			entry, found := ouiStore.Lookup(mac)
			if found {
				results[i] = fiber.Map{
					"mac":    mac,
					"oui":    entry.OUI,
					"vendor": entry.Vendor,
					"found":  true,
				}
			} else {
				results[i] = fiber.Map{
					"mac":    mac,
					"vendor": nil,
					"found":  false,
				}
			}
		}

		return c.JSON(fiber.Map{
			"results": results,
			"count":   len(results),
		})
	})

	// Status endpoint
	api.Get("/lookup/oui/status", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"loaded":      ouiStore.IsLoaded(),
			"entry_count": ouiStore.EntryCount(),
		})
	})
}
