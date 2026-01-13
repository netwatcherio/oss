// internal/geoip/cache.go
// ClickHouse cache layer for GeoIP lookups.
package geoip

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// CachedResult extends LookupResult with cache metadata.
type CachedResult struct {
	*LookupResult
	Cached    bool      `json:"cached"`
	CacheTime time.Time `json:"cache_time,omitempty"`
}

// GetCached retrieves a cached GeoIP result from ClickHouse.
// Returns nil if not found or expired.
func GetCached(ctx context.Context, db *sql.DB, ip string) (*CachedResult, error) {
	const query = `
SELECT
    ip, lookup_time, city, subdivision, country_code, country_name,
    asn, asn_org, latitude, longitude, accuracy
FROM ip_geo_cache FINAL
WHERE ip = ?
LIMIT 1
`
	row := db.QueryRowContext(ctx, query, ip)

	var (
		ipAddr                   string
		lookupTime               time.Time
		city, subdivision        string
		countryCode, countryName string
		asn                      uint
		asnOrg                   string
		lat, lon                 float64
		accuracy                 uint16
	)

	err := row.Scan(&ipAddr, &lookupTime, &city, &subdivision, &countryCode, &countryName, &asn, &asnOrg, &lat, &lon, &accuracy)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("geoip cache query: %w", err)
	}

	result := &LookupResult{IP: ipAddr}

	if city != "" || subdivision != "" {
		result.City = &CityInfo{Name: city, Subdivision: subdivision}
	}
	if countryCode != "" || countryName != "" {
		result.Country = &CountryInfo{Code: countryCode, Name: countryName}
	}
	if asn != 0 || asnOrg != "" {
		result.ASN = &ASNInfo{Number: asn, Organization: asnOrg}
	}
	if lat != 0 || lon != 0 {
		result.Coordinates = &Coordinates{Latitude: lat, Longitude: lon, Accuracy: accuracy}
	}

	return &CachedResult{
		LookupResult: result,
		Cached:       true,
		CacheTime:    lookupTime,
	}, nil
}

// SaveToCache stores a GeoIP lookup result in ClickHouse.
func SaveToCache(ctx context.Context, db *sql.DB, result *LookupResult) error {
	if result == nil || result.IP == "" {
		return nil
	}

	var (
		city, subdivision        string
		countryCode, countryName string
		asn                      uint
		asnOrg                   string
		lat, lon                 float64
		accuracy                 uint16
	)

	if result.City != nil {
		city = result.City.Name
		subdivision = result.City.Subdivision
	}
	if result.Country != nil {
		countryCode = result.Country.Code
		countryName = result.Country.Name
	}
	if result.ASN != nil {
		asn = result.ASN.Number
		asnOrg = result.ASN.Organization
	}
	if result.Coordinates != nil {
		lat = result.Coordinates.Latitude
		lon = result.Coordinates.Longitude
		accuracy = result.Coordinates.Accuracy
	}

	const insert = `
INSERT INTO ip_geo_cache
(ip, lookup_time, city, subdivision, country_code, country_name, asn, asn_org, latitude, longitude, accuracy)
VALUES (?, now('UTC'), ?, ?, ?, ?, ?, ?, ?, ?, ?)
`
	_, err := db.ExecContext(ctx, insert,
		result.IP, city, subdivision, countryCode, countryName, asn, asnOrg, lat, lon, accuracy,
	)
	return err
}

// LookupWithCache performs a GeoIP lookup with caching.
// It checks the cache first, then falls back to live lookup and caches the result.
func LookupWithCache(ctx context.Context, db *sql.DB, store *Store, ip string) (*CachedResult, error) {
	// Check cache first
	cached, err := GetCached(ctx, db, ip)
	if err != nil {
		// Log but continue with live lookup
		fmt.Printf("GeoIP cache read error: %v\n", err)
	}
	if cached != nil {
		return cached, nil
	}

	// Live lookup
	result, err := store.LookupAll(ip)
	if err != nil {
		return nil, err
	}

	// Save to cache (async, don't block on failure)
	go func() {
		cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if saveErr := SaveToCache(cacheCtx, db, result); saveErr != nil {
			fmt.Printf("GeoIP cache write error: %v\n", saveErr)
		}
	}()

	return &CachedResult{
		LookupResult: result,
		Cached:       false,
	}, nil
}

// BulkLookupWithCache performs parallel lookups with caching.
func BulkLookupWithCache(ctx context.Context, db *sql.DB, store *Store, ips []string) []CachedResult {
	results := make([]CachedResult, len(ips))

	for i, ip := range ips {
		cached, err := LookupWithCache(ctx, db, store, ip)
		if err != nil {
			results[i] = CachedResult{
				LookupResult: &LookupResult{IP: ip},
				Cached:       false,
			}
		} else if cached != nil {
			results[i] = *cached
		}
	}

	return results
}

// GetLookupHistory retrieves past lookups for an IP from the cache.
func GetLookupHistory(ctx context.Context, db *sql.DB, ip string, limit int) ([]CachedResult, error) {
	if limit <= 0 {
		limit = 10
	}

	query := fmt.Sprintf(`
SELECT
    ip, lookup_time, city, subdivision, country_code, country_name,
    asn, asn_org, latitude, longitude, accuracy
FROM ip_geo_cache
WHERE ip = '%s'
ORDER BY lookup_time DESC
LIMIT %d
`, strings.ReplaceAll(ip, "'", "''"), limit)

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("geoip history query: %w", err)
	}
	defer rows.Close()

	var results []CachedResult
	for rows.Next() {
		var (
			ipAddr                   string
			lookupTime               time.Time
			city, subdivision        string
			countryCode, countryName string
			asn                      uint
			asnOrg                   string
			lat, lon                 float64
			accuracy                 uint16
		)

		if err := rows.Scan(&ipAddr, &lookupTime, &city, &subdivision, &countryCode, &countryName, &asn, &asnOrg, &lat, &lon, &accuracy); err != nil {
			return nil, err
		}

		result := &LookupResult{IP: ipAddr}
		if city != "" || subdivision != "" {
			result.City = &CityInfo{Name: city, Subdivision: subdivision}
		}
		if countryCode != "" || countryName != "" {
			result.Country = &CountryInfo{Code: countryCode, Name: countryName}
		}
		if asn != 0 || asnOrg != "" {
			result.ASN = &ASNInfo{Number: asn, Organization: asnOrg}
		}
		if lat != 0 || lon != 0 {
			result.Coordinates = &Coordinates{Latitude: lat, Longitude: lon, Accuracy: accuracy}
		}

		results = append(results, CachedResult{
			LookupResult: result,
			Cached:       true,
			CacheTime:    lookupTime,
		})
	}

	return results, rows.Err()
}
