// web/geoip.go
package web

import (
	"net/http"
	"time"

	"netwatcher-controller/internal/geoip"
	"netwatcher-controller/internal/whois"

	"github.com/kataras/iris/v12"
)

// panelGeoIP registers GeoIP lookup endpoints.
// Routes: /geoip/*
func panelGeoIP(api iris.Party, geoStore *geoip.Store) {
	if geoStore == nil {
		return // GeoIP not configured
	}

	geo := api.Party("/geoip")

	// GET /geoip/lookup?ip={ip}
	// Single IP lookup
	geo.Get("/lookup", func(ctx iris.Context) {
		ip := ctx.URLParam("ip")
		if ip == "" {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "ip parameter is required"})
			return
		}

		result, err := geoStore.LookupAll(ip)
		if err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		_ = ctx.JSON(result)
	})

	// POST /geoip/lookup
	// Bulk IP lookup
	geo.Post("/lookup", func(ctx iris.Context) {
		var body struct {
			IPs []string `json:"ips"`
		}
		if err := ctx.ReadJSON(&body); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "invalid request body"})
			return
		}

		if len(body.IPs) == 0 {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "ips array is required"})
			return
		}

		// Limit bulk lookups to prevent abuse
		const maxBulk = 100
		if len(body.IPs) > maxBulk {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "maximum 100 IPs per request"})
			return
		}

		results := geoStore.LookupBulk(body.IPs)
		_ = ctx.JSON(iris.Map{"data": results, "total": len(results)})
	})

	// GET /geoip/status
	// Check which databases are loaded
	geo.Get("/status", func(ctx iris.Context) {
		_ = ctx.JSON(iris.Map{
			"city":    geoStore.HasCity(),
			"country": geoStore.HasCountry(),
			"asn":     geoStore.HasASN(),
		})
	})
}

// panelWhois registers WHOIS lookup endpoints.
// Routes: /whois/*
func panelWhois(api iris.Party) {
	ws := api.Party("/whois")

	// GET /whois/lookup?query={ip_or_domain}
	// Single WHOIS lookup
	ws.Get("/lookup", func(ctx iris.Context) {
		query := ctx.URLParam("query")
		if query == "" {
			// Also accept 'ip' param for backwards compatibility
			query = ctx.URLParam("ip")
		}
		if query == "" {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "query parameter is required"})
			return
		}

		// Validate input before running command
		sanitized, err := whois.ValidateQuery(query)
		if err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		// Perform lookup with 15-second timeout
		result, err := whois.LookupWithTimeout(sanitized, 15*time.Second)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		_ = ctx.JSON(result)
	})
}
