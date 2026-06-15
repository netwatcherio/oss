// web/ratelimit.go
package web

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

// ipBucket tracks hits for a single key within a fixed window.
type ipBucket struct {
	count   int
	resetAt time.Time
}

// ipLimiter is a tiny in-memory rate limiter keyed by an arbitrary string
// (typically the client IP). Suitable for a single-instance controller; not
// safe across multiple replicas. If you run a multi-replica deployment,
// swap this for a Redis-backed implementation.
type ipLimiter struct {
	mu     sync.Mutex
	hits   map[string]*ipBucket
	limit  int
	window time.Duration
}

func newIPLimiter(limit int, window time.Duration) *ipLimiter {
	l := &ipLimiter{
		hits:   make(map[string]*ipBucket),
		limit:  limit,
		window: window,
	}
	// Periodic GC to drop expired buckets and bound memory.
	go l.gc()
	return l
}

func (l *ipLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.hits[key]
	if !ok || now.After(b.resetAt) {
		l.hits[key] = &ipBucket{count: 1, resetAt: now.Add(l.window)}
		return true
	}
	if b.count >= l.limit {
		return false
	}
	b.count++
	return true
}

func (l *ipLimiter) gc() {
	t := time.NewTicker(l.window)
	defer t.Stop()
	for range t.C {
		l.mu.Lock()
		now := time.Now()
		for k, b := range l.hits {
			if now.After(b.resetAt) {
				delete(l.hits, k)
			}
		}
		l.mu.Unlock()
	}
}

// rateLimitByIP returns a Fiber middleware that allows at most `limit` requests
// per `window` per client IP. Excess requests receive a 429.
func rateLimitByIP(limit int, window time.Duration) fiber.Handler {
	lim := newIPLimiter(limit, window)
	return func(c *fiber.Ctx) error {
		ip := c.IP()
		if !lim.allow(ip) {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "too many requests, please slow down",
			})
		}
		return c.Next()
	}
}
