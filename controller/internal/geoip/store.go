// Package geoip provides MaxMind GeoIP2/GeoLite2 database lookups.
package geoip

import (
	"errors"
	"net"
	"os"

	"github.com/oschwald/geoip2-golang"
)

// -------------------- Errors --------------------

var (
	ErrInvalidIP      = errors.New("invalid IP address")
	ErrDatabaseClosed = errors.New("database reader is closed")
	ErrNoDatabases    = errors.New("no GeoIP databases configured")
)

// -------------------- Config --------------------

// Config holds paths to MaxMind database files.
type Config struct {
	CityPath    string // Path to GeoLite2-City.mmdb
	CountryPath string // Path to GeoLite2-Country.mmdb
	ASNPath     string // Path to GeoLite2-ASN.mmdb
}

// LoadConfigFromEnv loads GeoIP configuration from environment variables.
func LoadConfigFromEnv() Config {
	return Config{
		CityPath:    os.Getenv("GEOIP_CITY_PATH"),
		CountryPath: os.Getenv("GEOIP_COUNTRY_PATH"),
		ASNPath:     os.Getenv("GEOIP_ASN_PATH"),
	}
}

// IsConfigured returns true if at least one database path is set.
func (c Config) IsConfigured() bool {
	return c.CityPath != "" || c.CountryPath != "" || c.ASNPath != ""
}

// -------------------- Result Types --------------------

// CityInfo contains city-level geographic information.
type CityInfo struct {
	Name        string `json:"name,omitempty"`
	Subdivision string `json:"subdivision,omitempty"` // State/Province
}

// CountryInfo contains country-level information.
type CountryInfo struct {
	Code string `json:"code,omitempty"` // ISO 3166-1 alpha-2
	Name string `json:"name,omitempty"`
}

// ASNInfo contains autonomous system information.
type ASNInfo struct {
	Number       uint   `json:"number,omitempty"`
	Organization string `json:"organization,omitempty"`
}

// Coordinates contains geographic coordinates.
type Coordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Accuracy  uint16  `json:"accuracy_radius,omitempty"` // km
}

// LookupResult is the combined result of all GeoIP lookups for an IP.
type LookupResult struct {
	IP          string       `json:"ip"`
	City        *CityInfo    `json:"city,omitempty"`
	Country     *CountryInfo `json:"country,omitempty"`
	ASN         *ASNInfo     `json:"asn,omitempty"`
	Coordinates *Coordinates `json:"coordinates,omitempty"`
}

// -------------------- Store --------------------

// Store manages MaxMind database readers.
type Store struct {
	cityReader    *geoip2.Reader
	countryReader *geoip2.Reader
	asnReader     *geoip2.Reader
}

// NewStore initializes a new GeoIP store with the given configuration.
// It will skip databases that don't exist or have empty paths.
func NewStore(cfg Config) (*Store, error) {
	s := &Store{}

	if cfg.CityPath != "" {
		r, err := geoip2.Open(cfg.CityPath)
		if err != nil {
			return nil, err
		}
		s.cityReader = r
	}

	if cfg.CountryPath != "" {
		r, err := geoip2.Open(cfg.CountryPath)
		if err != nil {
			s.Close() // Clean up any opened readers
			return nil, err
		}
		s.countryReader = r
	}

	if cfg.ASNPath != "" {
		r, err := geoip2.Open(cfg.ASNPath)
		if err != nil {
			s.Close()
			return nil, err
		}
		s.asnReader = r
	}

	if s.cityReader == nil && s.countryReader == nil && s.asnReader == nil {
		return nil, ErrNoDatabases
	}

	return s, nil
}

// Close releases all database readers.
func (s *Store) Close() {
	if s.cityReader != nil {
		_ = s.cityReader.Close()
	}
	if s.countryReader != nil {
		_ = s.countryReader.Close()
	}
	if s.asnReader != nil {
		_ = s.asnReader.Close()
	}
}

// HasCity returns true if the City database is loaded.
func (s *Store) HasCity() bool { return s.cityReader != nil }

// HasCountry returns true if the Country database is loaded.
func (s *Store) HasCountry() bool { return s.countryReader != nil }

// HasASN returns true if the ASN database is loaded.
func (s *Store) HasASN() bool { return s.asnReader != nil }

// -------------------- Lookup Methods --------------------

// parseIP validates and parses an IP address string.
func parseIP(ipStr string) (net.IP, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, ErrInvalidIP
	}
	return ip, nil
}

// LookupCity retrieves city-level information for an IP.
func (s *Store) LookupCity(ipStr string) (*CityInfo, *Coordinates, error) {
	if s.cityReader == nil {
		return nil, nil, nil
	}

	ip, err := parseIP(ipStr)
	if err != nil {
		return nil, nil, err
	}

	record, err := s.cityReader.City(ip)
	if err != nil {
		return nil, nil, err
	}

	var city *CityInfo
	if record.City.Names["en"] != "" {
		city = &CityInfo{Name: record.City.Names["en"]}
		if len(record.Subdivisions) > 0 {
			city.Subdivision = record.Subdivisions[0].IsoCode
		}
	}

	var coords *Coordinates
	if record.Location.Latitude != 0 || record.Location.Longitude != 0 {
		coords = &Coordinates{
			Latitude:  record.Location.Latitude,
			Longitude: record.Location.Longitude,
			Accuracy:  record.Location.AccuracyRadius,
		}
	}

	return city, coords, nil
}

// LookupCountry retrieves country-level information for an IP.
func (s *Store) LookupCountry(ipStr string) (*CountryInfo, error) {
	// Try city reader first (it includes country data)
	if s.cityReader != nil {
		ip, err := parseIP(ipStr)
		if err != nil {
			return nil, err
		}
		record, err := s.cityReader.City(ip)
		if err != nil {
			return nil, err
		}
		if record.Country.IsoCode != "" {
			return &CountryInfo{
				Code: record.Country.IsoCode,
				Name: record.Country.Names["en"],
			}, nil
		}
	}

	// Fall back to country-only reader
	if s.countryReader == nil {
		return nil, nil
	}

	ip, err := parseIP(ipStr)
	if err != nil {
		return nil, err
	}

	record, err := s.countryReader.Country(ip)
	if err != nil {
		return nil, err
	}

	if record.Country.IsoCode == "" {
		return nil, nil
	}

	return &CountryInfo{
		Code: record.Country.IsoCode,
		Name: record.Country.Names["en"],
	}, nil
}

// LookupASN retrieves autonomous system information for an IP.
func (s *Store) LookupASN(ipStr string) (*ASNInfo, error) {
	if s.asnReader == nil {
		return nil, nil
	}

	ip, err := parseIP(ipStr)
	if err != nil {
		return nil, err
	}

	record, err := s.asnReader.ASN(ip)
	if err != nil {
		return nil, err
	}

	if record.AutonomousSystemNumber == 0 {
		return nil, nil
	}

	return &ASNInfo{
		Number:       record.AutonomousSystemNumber,
		Organization: record.AutonomousSystemOrganization,
	}, nil
}

// LookupAll performs all available lookups for an IP and returns combined results.
func (s *Store) LookupAll(ipStr string) (*LookupResult, error) {
	// Validate IP first
	if _, err := parseIP(ipStr); err != nil {
		return nil, err
	}

	result := &LookupResult{IP: ipStr}

	// City lookup (also gets coordinates)
	city, coords, err := s.LookupCity(ipStr)
	if err != nil {
		return nil, err
	}
	result.City = city
	result.Coordinates = coords

	// Country lookup
	country, err := s.LookupCountry(ipStr)
	if err != nil {
		return nil, err
	}
	result.Country = country

	// ASN lookup
	asn, err := s.LookupASN(ipStr)
	if err != nil {
		return nil, err
	}
	result.ASN = asn

	return result, nil
}

// LookupBulk performs lookups for multiple IPs.
// Errors for individual IPs are captured in the result's error field.
func (s *Store) LookupBulk(ips []string) []LookupResult {
	results := make([]LookupResult, 0, len(ips))
	for _, ip := range ips {
		result, err := s.LookupAll(ip)
		if err != nil {
			// Include the IP but with no data
			results = append(results, LookupResult{IP: ip})
		} else {
			results = append(results, *result)
		}
	}
	return results
}
