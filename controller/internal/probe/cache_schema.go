// internal/probe/cache_schema.go
// ClickHouse cache tables for GeoIP and WHOIS lookups.
package probe

import (
	"context"
	"database/sql"
)

// MigrateCacheTablesCH creates the GeoIP and WHOIS cache tables (idempotent).
func MigrateCacheTablesCH(ctx context.Context, ch *sql.DB) error {
	// ip_geo_cache: Cache GeoIP lookups with 30-day TTL
	const geoipDDL = `
	CREATE TABLE IF NOT EXISTS ip_geo_cache (
		ip             String,
		lookup_time    DateTime('UTC') DEFAULT now('UTC'),
		city           String,
		subdivision    String,
		country_code   String,
		country_name   String,
		asn            UInt32,
		asn_org        String,
		latitude       Float64,
		longitude      Float64,
		accuracy       UInt16
	)
	ENGINE = ReplacingMergeTree(lookup_time)
	ORDER BY ip
	TTL lookup_time + INTERVAL 30 DAY DELETE
	SETTINGS index_granularity = 8192;
`

	if _, err := ch.ExecContext(ctx, geoipDDL); err != nil {
		return err
	}

	// ip_whois_cache: Cache WHOIS lookups with 7-day TTL
	const whoisDDL = `
	CREATE TABLE IF NOT EXISTS ip_whois_cache (
		query          String,
		lookup_time    DateTime('UTC') DEFAULT now('UTC'),
		raw_output     String,
		netname        String,
		netrange       String,
		organization   String,
		country        String,
		registrar      String,
		created        String,
		updated        String,
		abuse_email    String,
		lookup_ms      UInt32
	)
	ENGINE = ReplacingMergeTree(lookup_time)
	ORDER BY query
	TTL lookup_time + INTERVAL 7 DAY DELETE
	SETTINGS index_granularity = 8192;
`

	_, err := ch.ExecContext(ctx, whoisDDL)
	return err
}
