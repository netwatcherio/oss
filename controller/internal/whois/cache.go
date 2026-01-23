// internal/whois/cache.go
// ClickHouse cache layer for WHOIS lookups.
package whois

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// CacheTTL is the time-to-live for cached WHOIS entries.
// Entries older than this are treated as expired and refreshed on next access.
// Can be overridden via WHOIS_CACHE_TTL_DAYS environment variable.
var CacheTTL = 30 * 24 * time.Hour // 30 days default

func init() {
	if envTTL := os.Getenv("WHOIS_CACHE_TTL_DAYS"); envTTL != "" {
		if days, err := strconv.Atoi(envTTL); err == nil && days > 0 {
			CacheTTL = time.Duration(days) * 24 * time.Hour
			log.Infof("WHOIS cache TTL set to %d days from environment", days)
		}
	}
}

// CachedResult extends Result with cache metadata.
type CachedResult struct {
	*Result
	Cached    bool      `json:"cached"`
	CacheTime time.Time `json:"cache_time,omitempty"`
}

// GetCached retrieves a cached WHOIS result from ClickHouse.
// Returns nil if not found or expired.
func GetCached(ctx context.Context, db *sql.DB, query string) (*CachedResult, error) {
	const q = `
SELECT
    query, lookup_time, raw_output, netname, netrange, organization,
    country, registrar, created, updated, abuse_email, lookup_ms
FROM ip_whois_cache FINAL
WHERE query = ?
LIMIT 1
`
	row := db.QueryRowContext(ctx, q, query)

	var (
		queryStr   string
		lookupTime time.Time
		rawOutput  string
		netname    string
		netrange   string
		org        string
		country    string
		registrar  string
		created    string
		updated    string
		abuseEmail string
		lookupMs   uint32
	)

	err := row.Scan(&queryStr, &lookupTime, &rawOutput, &netname, &netrange, &org, &country, &registrar, &created, &updated, &abuseEmail, &lookupMs)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("whois cache query: %w", err)
	}

	parsed := make(map[string]string)
	if netname != "" {
		parsed["netname"] = netname
	}
	if netrange != "" {
		parsed["netrange"] = netrange
	}
	if org != "" {
		parsed["organization"] = org
	}
	if country != "" {
		parsed["country"] = country
	}
	if registrar != "" {
		parsed["registrar"] = registrar
	}
	if created != "" {
		parsed["created"] = created
	}
	if updated != "" {
		parsed["updated"] = updated
	}
	if abuseEmail != "" {
		parsed["abuse_email"] = abuseEmail
	}

	result := &Result{
		Query:      queryStr,
		RawOutput:  rawOutput,
		Parsed:     parsed,
		LookupTime: time.Duration(lookupMs) * time.Millisecond,
	}

	return &CachedResult{
		Result:    result,
		Cached:    true,
		CacheTime: lookupTime,
	}, nil
}

// SaveToCache stores a WHOIS lookup result in ClickHouse.
func SaveToCache(ctx context.Context, db *sql.DB, result *Result) error {
	if result == nil || result.Query == "" {
		return nil
	}

	const insert = `
INSERT INTO ip_whois_cache
(query, lookup_time, raw_output, netname, netrange, organization, country, registrar, created, updated, abuse_email, lookup_ms)
VALUES (?, now('UTC'), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`
	_, err := db.ExecContext(ctx, insert,
		result.Query,
		result.RawOutput,
		result.Parsed["netname"],
		result.Parsed["netrange"],
		result.Parsed["organization"],
		result.Parsed["country"],
		result.Parsed["registrar"],
		result.Parsed["created"],
		result.Parsed["updated"],
		result.Parsed["abuse_email"],
		uint32(result.LookupTime/time.Millisecond),
	)
	return err
}

// LookupWithCache performs a WHOIS lookup with caching.
// It checks the cache first, then falls back to live lookup and caches the result.
// Cached entries older than CacheTTL (default 30 days) are treated as expired.
func LookupWithCache(ctx context.Context, db *sql.DB, query string, timeout time.Duration) (*CachedResult, error) {
	// Validate input first
	sanitized, err := ValidateQuery(query)
	if err != nil {
		return nil, err
	}

	// Check cache first
	cached, err := GetCached(ctx, db, sanitized)
	if err != nil {
		// Log but continue with live lookup
		log.WithError(err).Debug("WHOIS cache read error")
	}

	// Check if cache entry exists and is still valid (not expired)
	if cached != nil {
		age := time.Since(cached.CacheTime)
		if age < CacheTTL {
			return cached, nil
		}
		// Cache entry expired, will refresh below
		log.WithField("query", sanitized).WithField("age_days", int(age.Hours()/24)).Debug("WHOIS cache entry expired, refreshing")
	}

	// Live lookup (either no cache or expired)
	result, err := LookupWithTimeout(sanitized, timeout)
	if err != nil {
		// If we have an expired cache entry and live lookup fails, return stale data
		if cached != nil {
			log.WithError(err).WithField("query", sanitized).Warn("WHOIS live lookup failed, returning stale cache")
			return cached, nil
		}
		return nil, err
	}

	// Save to cache (async, don't block on failure)
	go func() {
		cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if saveErr := SaveToCache(cacheCtx, db, result); saveErr != nil {
			log.WithError(saveErr).Debug("WHOIS cache write error")
		}
	}()

	return &CachedResult{
		Result: result,
		Cached: false,
	}, nil
}

// GetLookupHistory retrieves past lookups for a query from the cache.
func GetLookupHistory(ctx context.Context, db *sql.DB, query string, limit int) ([]CachedResult, error) {
	if limit <= 0 {
		limit = 10
	}

	q := fmt.Sprintf(`
SELECT
    query, lookup_time, raw_output, netname, netrange, organization,
    country, registrar, created, updated, abuse_email, lookup_ms
FROM ip_whois_cache
WHERE query = '%s'
ORDER BY lookup_time DESC
LIMIT %d
`, strings.ReplaceAll(query, "'", "''"), limit)

	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("whois history query: %w", err)
	}
	defer rows.Close()

	var results []CachedResult
	for rows.Next() {
		var (
			queryStr   string
			lookupTime time.Time
			rawOutput  string
			netname    string
			netrange   string
			org        string
			country    string
			registrar  string
			created    string
			updated    string
			abuseEmail string
			lookupMs   uint32
		)

		if err := rows.Scan(&queryStr, &lookupTime, &rawOutput, &netname, &netrange, &org, &country, &registrar, &created, &updated, &abuseEmail, &lookupMs); err != nil {
			return nil, err
		}

		parsed := make(map[string]string)
		if netname != "" {
			parsed["netname"] = netname
		}
		if netrange != "" {
			parsed["netrange"] = netrange
		}
		if org != "" {
			parsed["organization"] = org
		}
		if country != "" {
			parsed["country"] = country
		}
		if registrar != "" {
			parsed["registrar"] = registrar
		}
		if created != "" {
			parsed["created"] = created
		}
		if updated != "" {
			parsed["updated"] = updated
		}
		if abuseEmail != "" {
			parsed["abuse_email"] = abuseEmail
		}

		results = append(results, CachedResult{
			Result: &Result{
				Query:      queryStr,
				RawOutput:  rawOutput,
				Parsed:     parsed,
				LookupTime: time.Duration(lookupMs) * time.Millisecond,
			},
			Cached:    true,
			CacheTime: lookupTime,
		})
	}

	return results, rows.Err()
}
