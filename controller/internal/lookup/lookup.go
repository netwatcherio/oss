// Package lookup provides shared IP lookup logic for both agent and panel routes.
package lookup

import (
	"context"
	"database/sql"
	"net"
	"strings"
	"time"

	"netwatcher-controller/internal/geoip"

	"github.com/kataras/iris/v12"
	log "github.com/sirupsen/logrus"
)

// WhoAmIResult contains the full result of a public IP lookup.
type WhoAmIResult struct {
	IP         string              `json:"ip"`
	GeoIP      *geoip.CachedResult `json:"geoip,omitempty"`
	ReverseDNS string              `json:"reverse_dns,omitempty"`
	Timestamp  time.Time           `json:"timestamp"`
}

// IPOnlyResult is a minimal response containing just the IP address.
type IPOnlyResult struct {
	IP        string    `json:"ip"`
	Timestamp time.Time `json:"timestamp"`
}

// trustedProxyCIDRs defines private/trusted IP ranges.
// Requests from these IPs will have X-Forwarded-For examined.
var trustedProxyCIDRs = []string{
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"127.0.0.0/8",
	"fc00::/7",
	"::1/128",
}

// isPrivateIP checks if an IP is in a trusted/private range.
func isPrivateIP(ip net.IP) bool {
	for _, cidr := range trustedProxyCIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

// GetClientIP extracts the real client IP from an Iris context.
// It checks X-Forwarded-For and X-Real-IP headers when behind a trusted proxy.
// This is the canonical function for determining a client's public IP.
func GetClientIP(ctx iris.Context) string {
	// Get remote address (may include port)
	remoteAddr := ctx.RemoteAddr()

	// Strip port if present
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// No port, use as-is
		host = remoteAddr
	}

	remoteIP := net.ParseIP(host)
	if remoteIP == nil {
		// Failed to parse, return raw address
		return host
	}

	// If remote IP is not private/trusted, it's the real client
	if !isPrivateIP(remoteIP) {
		return remoteIP.String()
	}

	// Check X-Forwarded-For header (comma-separated list, leftmost is original client)
	if forwardedFor := ctx.GetHeader("X-Forwarded-For"); forwardedFor != "" {
		ips := strings.Split(forwardedFor, ",")
		for _, ip := range ips {
			parsedIP := net.ParseIP(strings.TrimSpace(ip))
			if parsedIP != nil && !isPrivateIP(parsedIP) {
				return parsedIP.String()
			}
		}
	}

	// Check X-Real-IP header (single IP set by proxy)
	if realIP := ctx.GetHeader("X-Real-IP"); realIP != "" {
		parsedIP := net.ParseIP(strings.TrimSpace(realIP))
		if parsedIP != nil && !isPrivateIP(parsedIP) {
			return parsedIP.String()
		}
	}

	// Fallback to remote address
	return remoteIP.String()
}

// ReverseDNS performs a PTR lookup for the given IP address.
// Returns empty string if lookup fails or no PTR record exists.
func ReverseDNS(ip string) string {
	names, err := net.LookupAddr(ip)
	if err != nil || len(names) == 0 {
		return ""
	}
	// Return first PTR record, strip trailing dot
	return strings.TrimSuffix(names[0], ".")
}

// UnifiedLookup performs a combined GeoIP + reverse DNS lookup for an IP.
// If geoStore is nil, only reverse DNS is performed.
// If ch (ClickHouse) is nil, no caching is used for GeoIP.
func UnifiedLookup(ctx context.Context, ch *sql.DB, geoStore *geoip.Store, ip string) (*WhoAmIResult, error) {
	result := &WhoAmIResult{
		IP:        ip,
		Timestamp: time.Now(),
	}

	// Validate IP address
	if net.ParseIP(ip) == nil {
		return result, nil // Return with just IP, no enrichment
	}

	// GeoIP lookup (if configured)
	if geoStore != nil {
		if ch != nil {
			// Use cached lookup
			geoResult, err := geoip.LookupWithCache(ctx, ch, geoStore, ip)
			if err != nil {
				log.WithError(err).WithField("ip", ip).Warn("GeoIP cached lookup failed")
			} else if geoResult != nil {
				result.GeoIP = geoResult
				// Log detailed result
				fields := log.Fields{"ip": ip, "cached": geoResult.Cached}
				if geoResult.LookupResult != nil {
					if geoResult.Country != nil {
						fields["country"] = geoResult.Country.Name
					}
					if geoResult.City != nil {
						fields["city"] = geoResult.City.Name
					}
					if geoResult.ASN != nil {
						fields["asn_org"] = geoResult.ASN.Organization
					}
				} else {
					fields["lookup_result"] = "nil"
				}
				log.WithFields(fields).Info("GeoIP lookup successful")
			} else {
				log.WithField("ip", ip).Warn("GeoIP cached lookup returned nil")
			}
		} else {
			// Direct lookup without cache
			directResult, err := geoStore.LookupAll(ip)
			if err != nil {
				log.WithError(err).WithField("ip", ip).Warn("GeoIP direct lookup failed")
			} else if directResult != nil {
				result.GeoIP = &geoip.CachedResult{LookupResult: directResult, Cached: false}
				log.WithField("ip", ip).Info("GeoIP direct lookup successful")
			}
		}
	} else {
		log.WithField("ip", ip).Warn("GeoIP lookup skipped: geoStore is nil")
	}

	// Reverse DNS lookup (quick, no caching)
	result.ReverseDNS = ReverseDNS(ip)

	return result, nil
}

// QuickLookup returns just the client IP without enrichment.
// Use this for minimal latency when only the IP is needed.
func QuickLookup(ctx iris.Context) *IPOnlyResult {
	return &IPOnlyResult{
		IP:        GetClientIP(ctx),
		Timestamp: time.Now(),
	}
}
