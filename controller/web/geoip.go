// web/geoip.go
package web

import (
	"database/sql"
	"net/http"
	"time"

	"netwatcher-controller/internal/geoip"
	"netwatcher-controller/internal/whois"

	"github.com/kataras/iris/v12"
)

// CombinedLookupResult contains both GeoIP and WHOIS data for an IP.
type CombinedLookupResult struct {
	IP        string              `json:"ip"`
	GeoIP     *geoip.CachedResult `json:"geoip,omitempty"`
	Whois     *whois.CachedResult `json:"whois,omitempty"`
	Cached    bool                `json:"cached"`
	CacheTime *time.Time          `json:"cache_time,omitempty"`
}

// panelGeoIP registers GeoIP lookup endpoints.
// Routes: /geoip/*
func panelGeoIP(api iris.Party, geoStore *geoip.Store, ch *sql.DB) {
	geo := api.Party("/geoip")

	// GET /geoip/lookup?ip={ip}
	// Single IP lookup with caching
	geo.Get("/lookup", func(ctx iris.Context) {
		ip := ctx.URLParam("ip")
		if ip == "" {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "ip parameter is required"})
			return
		}

		// Use cache if available, otherwise fall back to direct lookup
		if geoStore == nil {
			ctx.StatusCode(http.StatusServiceUnavailable)
			_ = ctx.JSON(iris.Map{"error": "GeoIP not configured"})
			return
		}

		if ch != nil {
			result, err := geoip.LookupWithCache(ctx.Request().Context(), ch, geoStore, ip)
			if err != nil {
				ctx.StatusCode(http.StatusBadRequest)
				_ = ctx.JSON(iris.Map{"error": err.Error()})
				return
			}
			_ = ctx.JSON(result)
			return
		}

		// No cache available, direct lookup
		result, err := geoStore.LookupAll(ip)
		if err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		_ = ctx.JSON(result)
	})

	// POST /geoip/lookup
	// Bulk IP lookup with caching
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

		if geoStore == nil {
			ctx.StatusCode(http.StatusServiceUnavailable)
			_ = ctx.JSON(iris.Map{"error": "GeoIP not configured"})
			return
		}

		if ch != nil {
			results := geoip.BulkLookupWithCache(ctx.Request().Context(), ch, geoStore, body.IPs)
			_ = ctx.JSON(iris.Map{"data": results, "total": len(results)})
			return
		}

		// No cache, use direct bulk lookup
		results := geoStore.LookupBulk(body.IPs)
		_ = ctx.JSON(iris.Map{"data": results, "total": len(results)})
	})

	// GET /geoip/history?ip={ip}&limit={n}
	// Get cached lookup history for an IP
	geo.Get("/history", func(ctx iris.Context) {
		ip := ctx.URLParam("ip")
		if ip == "" {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "ip parameter is required"})
			return
		}

		if ch == nil {
			ctx.StatusCode(http.StatusServiceUnavailable)
			_ = ctx.JSON(iris.Map{"error": "cache not available"})
			return
		}

		limit := ctx.URLParamIntDefault("limit", 10)
		results, err := geoip.GetLookupHistory(ctx.Request().Context(), ch, ip, limit)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		_ = ctx.JSON(iris.Map{"data": results, "total": len(results)})
	})

	// GET /geoip/status
	// Check which databases are loaded
	geo.Get("/status", func(ctx iris.Context) {
		if geoStore == nil {
			_ = ctx.JSON(iris.Map{
				"configured": false,
				"city":       false,
				"country":    false,
				"asn":        false,
			})
			return
		}
		_ = ctx.JSON(iris.Map{
			"configured": true,
			"city":       geoStore.HasCity(),
			"country":    geoStore.HasCountry(),
			"asn":        geoStore.HasASN(),
		})
	})
}

// panelWhois registers WHOIS lookup endpoints.
// Routes: /whois/*
func panelWhois(api iris.Party, ch *sql.DB) {
	ws := api.Party("/whois")

	// GET /whois/lookup?query={ip_or_domain}
	// Single WHOIS lookup with caching
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

		// Use cache if available
		if ch != nil {
			result, err := whois.LookupWithCache(ctx.Request().Context(), ch, query, 15*time.Second)
			if err != nil {
				ctx.StatusCode(http.StatusBadRequest)
				_ = ctx.JSON(iris.Map{"error": err.Error()})
				return
			}
			_ = ctx.JSON(result)
			return
		}

		// No cache, direct lookup
		sanitized, err := whois.ValidateQuery(query)
		if err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		result, err := whois.LookupWithTimeout(sanitized, 15*time.Second)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		_ = ctx.JSON(result)
	})

	// GET /whois/history?query={ip_or_domain}&limit={n}
	// Get cached lookup history
	ws.Get("/history", func(ctx iris.Context) {
		query := ctx.URLParam("query")
		if query == "" {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "query parameter is required"})
			return
		}

		if ch == nil {
			ctx.StatusCode(http.StatusServiceUnavailable)
			_ = ctx.JSON(iris.Map{"error": "cache not available"})
			return
		}

		limit := ctx.URLParamIntDefault("limit", 10)
		results, err := whois.GetLookupHistory(ctx.Request().Context(), ch, query, limit)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		_ = ctx.JSON(iris.Map{"data": results, "total": len(results)})
	})
}

// panelLookup registers combined lookup endpoints.
// Routes: /lookup/*
func panelLookup(api iris.Party, geoStore *geoip.Store, ch *sql.DB) {
	lookup := api.Party("/lookup")

	// GET /lookup/combined?ip={ip}
	// Combined GeoIP + WHOIS lookup in one request
	lookup.Get("/combined", func(ctx iris.Context) {
		ip := ctx.URLParam("ip")
		if ip == "" {
			ctx.StatusCode(http.StatusBadRequest)
			_ = ctx.JSON(iris.Map{"error": "ip parameter is required"})
			return
		}

		result := CombinedLookupResult{
			IP:     ip,
			Cached: false,
		}

		// GeoIP lookup
		if geoStore != nil {
			if ch != nil {
				geoResult, err := geoip.LookupWithCache(ctx.Request().Context(), ch, geoStore, ip)
				if err == nil {
					result.GeoIP = geoResult
					if geoResult.Cached {
						result.Cached = true
						result.CacheTime = &geoResult.CacheTime
					}
				}
			} else {
				directResult, err := geoStore.LookupAll(ip)
				if err == nil {
					result.GeoIP = &geoip.CachedResult{LookupResult: directResult, Cached: false}
				}
			}
		}

		// WHOIS lookup
		if ch != nil {
			whoisResult, err := whois.LookupWithCache(ctx.Request().Context(), ch, ip, 15*time.Second)
			if err == nil {
				result.Whois = whoisResult
				if whoisResult.Cached && result.CacheTime == nil {
					result.Cached = true
					result.CacheTime = &whoisResult.CacheTime
				}
			}
		} else {
			sanitized, err := whois.ValidateQuery(ip)
			if err == nil {
				whoisResult, err := whois.LookupWithTimeout(sanitized, 15*time.Second)
				if err == nil {
					result.Whois = &whois.CachedResult{Result: whoisResult, Cached: false}
				}
			}
		}

		_ = ctx.JSON(result)
	})
}
