package web

import (
	"net"
	"strings"

	"github.com/gofiber/fiber/v2"
)

var trustedProxies = []string{
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"fc00::/7",
}

func isPrivateIP(ip net.IP) bool {
	for _, cidr := range trustedProxies {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

func ProxyIPMiddleware(c *fiber.Ctx) error {
	remoteIP := net.ParseIP(c.IP())
	if remoteIP == nil {
		c.Locals("client_ip", c.IP())
		return c.Next()
	}

	if !isPrivateIP(remoteIP) {
		c.Locals("client_ip", remoteIP.String())
		return c.Next()
	}

	if forwardedFor := c.Get("X-Forwarded-For"); forwardedFor != "" {
		ips := strings.Split(forwardedFor, ",")
		for _, ip := range ips {
			parsedIP := net.ParseIP(strings.TrimSpace(ip))
			if parsedIP != nil && !isPrivateIP(parsedIP) {
				c.Locals("client_ip", parsedIP.String())
				return c.Next()
			}
		}
	}

	if realIP := c.Get("X-Real-IP"); realIP != "" {
		parsedIP := net.ParseIP(realIP)
		if parsedIP != nil && !isPrivateIP(parsedIP) {
			c.Locals("client_ip", parsedIP.String())
			return c.Next()
		}
	}

	// If we couldn't determine a public IP, fall back to the remote address
	c.Locals("client_ip", c.IP())
	return c.Next()
}
