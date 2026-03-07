// web/geoip.go
package web

import (
	"database/sql"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"netwatcher-controller/internal/geoip"
	"netwatcher-controller/internal/lookup"
	"netwatcher-controller/internal/whois"

	"github.com/gofiber/fiber/v2"
)

// CombinedLookupResult contains both GeoIP and WHOIS data for an IP.
type CombinedLookupResult struct {
	IP        string              `json:"ip"`
	Hostname  string              `json:"hostname,omitempty"` // Original hostname if resolved
	GeoIP     *geoip.CachedResult `json:"geoip,omitempty"`
	Whois     *whois.CachedResult `json:"whois,omitempty"`
	Cached    bool                `json:"cached"`
	CacheTime *time.Time          `json:"cache_time,omitempty"`
}

// panelGeoIP registers GeoIP lookup endpoints.
// Routes: /geoip/*
func panelGeoIP(api fiber.Router, geoStore *geoip.Store, ch *sql.DB) {
	geo := api.Group("/geoip")

	// GET /geoip/lookup?ip={ip}
	// Single IP lookup with caching
	geo.Get("/lookup", func(c *fiber.Ctx) error {
		ip := c.Query("ip")
		if ip == "" {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "ip parameter is required"})
		}

		// Use cache if available, otherwise fall back to direct lookup
		if geoStore == nil {
			return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{"error": "GeoIP not configured"})
		}

		if ch != nil {
			result, err := geoip.LookupWithCache(c.UserContext(), ch, geoStore, ip)
			if err != nil {
				return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
			}
			return c.JSON(result)
		}

		// No cache available, direct lookup
		result, err := geoStore.LookupAll(ip)
		if err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(result)
	})

	// POST /geoip/lookup
	// Bulk IP lookup with caching
	geo.Post("/lookup", func(c *fiber.Ctx) error {
		var body struct {
			IPs []string `json:"ips"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}

		if len(body.IPs) == 0 {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "ips array is required"})
		}

		// Limit bulk lookups to prevent abuse
		const maxBulk = 100
		if len(body.IPs) > maxBulk {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "maximum 100 IPs per request"})
		}

		if geoStore == nil {
			return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{"error": "GeoIP not configured"})
		}

		if ch != nil {
			results := geoip.BulkLookupWithCache(c.UserContext(), ch, geoStore, body.IPs)
			return c.JSON(fiber.Map{"data": results, "total": len(results)})
		}

		// No cache, use direct bulk lookup
		results := geoStore.LookupBulk(body.IPs)
		return c.JSON(fiber.Map{"data": results, "total": len(results)})
	})

	// GET /geoip/history?ip={ip}&limit={n}
	// Get cached lookup history for an IP
	geo.Get("/history", func(c *fiber.Ctx) error {
		ip := c.Query("ip")
		if ip == "" {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "ip parameter is required"})
		}

		if ch == nil {
			return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{"error": "cache not available"})
		}

		limit, _ := strconv.Atoi(c.Query("limit", "10"))
		results, err := geoip.GetLookupHistory(c.UserContext(), ch, ip, limit)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{"data": results, "total": len(results)})
	})

	// GET /geoip/status
	// Check which databases are loaded
	geo.Get("/status", func(c *fiber.Ctx) error {
		if geoStore == nil {
			return c.JSON(fiber.Map{
				"configured": false,
				"city":       false,
				"country":    false,
				"asn":        false,
			})
		}
		return c.JSON(fiber.Map{
			"configured": true,
			"city":       geoStore.HasCity(),
			"country":    geoStore.HasCountry(),
			"asn":        geoStore.HasASN(),
		})
	})
}

// panelWhois registers WHOIS lookup endpoints.
// Routes: /whois/*
func panelWhois(api fiber.Router, ch *sql.DB) {
	ws := api.Group("/whois")

	// GET /whois/lookup?query={ip_or_domain}
	// Single WHOIS lookup with caching
	ws.Get("/lookup", func(c *fiber.Ctx) error {
		query := c.Query("query")
		if query == "" {
			// Also accept 'ip' param for backwards compatibility
			query = c.Query("ip")
		}
		if query == "" {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "query parameter is required"})
		}

		// Use cache if available
		if ch != nil {
			result, err := whois.LookupWithCache(c.UserContext(), ch, query, whois.DefaultTimeout)
			if err != nil {
				return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
			}
			return c.JSON(result)
		}

		// No cache, direct lookup
		sanitized, err := whois.ValidateQuery(query)
		if err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}

		result, err := whois.LookupWithTimeout(sanitized, whois.DefaultTimeout)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(result)
	})

	// GET /whois/history?query={ip_or_domain}&limit={n}
	// Get cached lookup history
	ws.Get("/history", func(c *fiber.Ctx) error {
		query := c.Query("query")
		if query == "" {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "query parameter is required"})
		}

		if ch == nil {
			return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{"error": "cache not available"})
		}

		limit, _ := strconv.Atoi(c.Query("limit", "10"))
		results, err := whois.GetLookupHistory(c.UserContext(), ch, query, limit)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{"data": results, "total": len(results)})
	})
}

// panelLookup registers combined lookup endpoints.
// Routes: /lookup/*
func panelLookup(api fiber.Router, geoStore *geoip.Store, ch *sql.DB) {
	lookupParty := api.Group("/lookup")

	// GET /lookup/ip/:ip
	// Unified IP lookup: GeoIP + ASN + Reverse DNS
	// Uses shared lookup package for consistency with agent API
	lookupParty.Get("/ip/:ip", func(c *fiber.Ctx) error {
		ip := c.Params("ip")
		if ip == "" {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "ip parameter required"})
		}

		result, err := lookup.UnifiedLookup(c.UserContext(), ch, geoStore, ip)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(result)
	})

	// GET /lookup/combined?ip={ip}
	// Combined GeoIP + WHOIS lookup in one request
	// Accepts both IP addresses and hostnames
	lookupParty.Get("/combined", func(c *fiber.Ctx) error {
		query := c.Query("ip")
		if query == "" {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "ip parameter is required"})
		}

		// Determine if input is an IP or hostname
		query = strings.TrimSpace(query)
		resolvedIP := query
		isHostname := false

		// Check if it's a valid IP address
		if net.ParseIP(query) == nil {
			// Not an IP, try to resolve as hostname
			ips, err := net.LookupIP(query)
			if err != nil || len(ips) == 0 {
				return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "could not resolve hostname: " + query})
			}
			// Use the first resolved IP (prefer IPv4)
			for _, ip := range ips {
				if ipv4 := ip.To4(); ipv4 != nil {
					resolvedIP = ipv4.String()
					break
				}
			}
			if resolvedIP == query {
				// No IPv4 found, use first IPv6
				resolvedIP = ips[0].String()
			}
			isHostname = true
		}

		result := CombinedLookupResult{
			IP:     resolvedIP,
			Cached: false,
		}

		// Add hostname to result if we resolved one
		if isHostname {
			result.Hostname = query
		}

		// Track errors for diagnostics
		var geoErr, whoisErr string

		// GeoIP lookup using resolved IP
		if geoStore != nil {
			if ch != nil {
				geoResult, err := geoip.LookupWithCache(c.UserContext(), ch, geoStore, resolvedIP)
				if err != nil {
					geoErr = err.Error()
				} else if geoResult != nil {
					result.GeoIP = geoResult
					if geoResult.Cached {
						result.Cached = true
						result.CacheTime = &geoResult.CacheTime
					}
				}
			} else {
				directResult, err := geoStore.LookupAll(resolvedIP)
				if err != nil {
					geoErr = err.Error()
				} else if directResult != nil {
					result.GeoIP = &geoip.CachedResult{LookupResult: directResult, Cached: false}
				}
			}
		} else {
			geoErr = "GeoIP databases not configured"
		}

		// WHOIS lookup - use original query for domains, resolved IP for IPs
		whoisQuery := query
		if ch != nil {
			whoisResult, err := whois.LookupWithCache(c.UserContext(), ch, whoisQuery, whois.DefaultTimeout)
			if err != nil {
				whoisErr = err.Error()
			} else if whoisResult != nil {
				result.Whois = whoisResult
				if whoisResult.Cached && result.CacheTime == nil {
					result.Cached = true
					result.CacheTime = &whoisResult.CacheTime
				}
			}
		} else {
			// No cache, try direct lookup
			sanitized, err := whois.ValidateQuery(whoisQuery)
			if err != nil {
				whoisErr = "invalid query: " + err.Error()
			} else {
				whoisResult, err := whois.LookupWithTimeout(sanitized, whois.DefaultTimeout)
				if err != nil {
					whoisErr = err.Error()
				} else if whoisResult != nil {
					result.Whois = &whois.CachedResult{Result: whoisResult, Cached: false}
				}
			}
		}

		// Include errors in response for diagnostics
		if geoErr != "" || whoisErr != "" {
			return c.JSON(fiber.Map{
				"ip":         result.IP,
				"hostname":   result.Hostname,
				"geoip":      result.GeoIP,
				"whois":      result.Whois,
				"cached":     result.Cached,
				"cache_time": result.CacheTime,
				"errors": fiber.Map{
					"geoip": geoErr,
					"whois": whoisErr,
				},
			})
		}

		return c.JSON(result)
	})
}
