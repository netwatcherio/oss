package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

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

		entry, found, err := ouiStore.Lookup(mac)
		if err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{
				"error": err.Error(),
				"mac":   mac,
			})
		}

		decodedMAC, _ := url.PathUnescape(mac)
		body := fiber.Map{
			"mac":   decodedMAC,
			"oui":   formatOUI(decodedMAC),
			"found": found,
		}
		if found {
			body["vendor"] = entry.Vendor
		} else {
			body["vendor"] = nil
		}
		return c.JSON(body)
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
			entry, found, err := ouiStore.Lookup(mac)
			if err != nil {
				results[i] = fiber.Map{
					"mac":    mac,
					"oui":    formatOUI(mac),
					"vendor": nil,
					"found":  false,
					"error":  err.Error(),
				}
				continue
			}
			body := fiber.Map{
				"mac":   mac,
				"oui":   formatOUI(mac),
				"found": found,
			}
			if found {
				body["vendor"] = entry.Vendor
			} else {
				body["vendor"] = nil
			}
			results[i] = body
		}

		return c.JSON(fiber.Map{
			"results": results,
			"count":   len(results),
		})
	})

	// Status endpoint
	api.Get("/lookup/oui/status", func(c *fiber.Ctx) error {
		loadedAt := time.Time{}
		if ouiStore.IsLoaded() {
			loadedAt = ouiStore.LoadedAt().UTC()
		}
		return c.JSON(fiber.Map{
			"loaded":       ouiStore.IsLoaded(),
			"entry_count":  ouiStore.EntryCount(),
			"parse_errors": ouiStore.ParseErrors(),
			"loaded_at":    loadedAt.Format(time.RFC3339),
			"source_path":  ouiStore.SourcePath(),
		})
	})
}

// formatOUI returns the XX-XX-XX prefix from a MAC string for echo in API
// responses. Returns an empty string if the input doesn't contain at least
// 6 hex digits after separator-stripping.
func formatOUI(mac string) string {
	cleaned := strings.ToUpper(mac)
	cleaned = strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(cleaned)
	if len(cleaned) < 6 {
		return ""
	}
	return fmt.Sprintf("%s-%s-%s", cleaned[:2], cleaned[2:4], cleaned[4:6])
}
