package web

import (
	"netwatcher-controller/internal/oui"

	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/core/router"
)

// panelOUI registers OUI lookup endpoints.
func panelOUI(api router.Party, ouiStore *oui.Store) {
	// Lookup single MAC
	api.Get("/lookup/oui/{mac}", func(ctx iris.Context) {
		mac := ctx.Params().Get("mac")
		if mac == "" {
			ctx.StatusCode(iris.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "mac parameter required"})
			return
		}

		if !ouiStore.IsLoaded() {
			ctx.StatusCode(iris.StatusServiceUnavailable)
			ctx.JSON(iris.Map{
				"error":  "OUI database not loaded",
				"loaded": false,
			})
			return
		}

		entry, found := ouiStore.Lookup(mac)
		if !found {
			ctx.StatusCode(iris.StatusNotFound)
			ctx.JSON(iris.Map{
				"mac":    mac,
				"vendor": nil,
				"found":  false,
			})
			return
		}

		ctx.JSON(iris.Map{
			"mac":    mac,
			"oui":    entry.OUI,
			"vendor": entry.Vendor,
			"found":  true,
		})
	})

	// Bulk lookup
	api.Post("/lookup/oui", func(ctx iris.Context) {
		var req struct {
			MACs []string `json:"macs"`
		}

		if err := ctx.ReadJSON(&req); err != nil {
			ctx.StatusCode(iris.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "invalid request body"})
			return
		}

		if len(req.MACs) == 0 {
			ctx.StatusCode(iris.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "macs array required"})
			return
		}

		if len(req.MACs) > 100 {
			ctx.StatusCode(iris.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "maximum 100 MACs per request"})
			return
		}

		if !ouiStore.IsLoaded() {
			ctx.StatusCode(iris.StatusServiceUnavailable)
			ctx.JSON(iris.Map{
				"error":  "OUI database not loaded",
				"loaded": false,
			})
			return
		}

		results := make([]iris.Map, len(req.MACs))
		for i, mac := range req.MACs {
			entry, found := ouiStore.Lookup(mac)
			if found {
				results[i] = iris.Map{
					"mac":    mac,
					"oui":    entry.OUI,
					"vendor": entry.Vendor,
					"found":  true,
				}
			} else {
				results[i] = iris.Map{
					"mac":    mac,
					"vendor": nil,
					"found":  false,
				}
			}
		}

		ctx.JSON(iris.Map{
			"results": results,
			"count":   len(results),
		})
	})

	// Status endpoint
	api.Get("/lookup/oui/status", func(ctx iris.Context) {
		ctx.JSON(iris.Map{
			"loaded":      ouiStore.IsLoaded(),
			"entry_count": ouiStore.EntryCount(),
		})
	})
}
