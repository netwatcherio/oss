package probe

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
)

func initNetInfo(db *sql.DB) {
	Register(NewHandler[netInfoPayload](
		TypeNetInfo,
		func(p netInfoPayload) error {
			return nil
		},
		func(ctx context.Context, data ProbeData, p netInfoPayload) error {
			if err := SaveRecordCH(ctx, db, data, string(TypeNetInfo), p); err != nil {
				log.WithError(err).Error("save netinfo record (CH)")
				return err
			}

			// Store to DB / compute / alert as needed:
			log.Printf("[netinfo] wan=%s lan=%s gw=%s source=%s",
				p.PublicAddress, p.LocalAddress, p.DefaultGateway, p.Source)
			return nil
		},
	))
}

// GeoInfo contains geographic and network information (new format).
// This matches the agent's GeoInfo structure for rich location data.
type GeoInfo struct {
	City        string  `json:"city,omitempty" bson:"city,omitempty"`
	Region      string  `json:"region,omitempty" bson:"region,omitempty"`
	Country     string  `json:"country,omitempty" bson:"country,omitempty"`
	CountryCode string  `json:"country_code,omitempty" bson:"country_code,omitempty"`
	Latitude    float64 `json:"latitude,omitempty" bson:"latitude,omitempty"`
	Longitude   float64 `json:"longitude,omitempty" bson:"longitude,omitempty"`
	ASN         uint    `json:"asn,omitempty" bson:"asn,omitempty"`
	ASNOrg      string  `json:"asn_org,omitempty" bson:"asn_org,omitempty"`
	ISP         string  `json:"isp,omitempty" bson:"isp,omitempty"`
	ReverseDNS  string  `json:"reverse_dns,omitempty" bson:"reverse_dns,omitempty"`
}

// InterfaceInfo contains detailed information about a network interface (P1.1).
type InterfaceInfo struct {
	Name      string   `json:"name" bson:"name"`
	Index     int      `json:"index" bson:"index"`
	Type      string   `json:"type" bson:"type"` // ethernet, wifi, loopback, vpn, etc.
	MAC       string   `json:"mac,omitempty" bson:"mac,omitempty"`
	MTU       int      `json:"mtu" bson:"mtu"`
	Flags     []string `json:"flags,omitempty" bson:"flags,omitempty"`
	IPv4      []string `json:"ipv4,omitempty" bson:"ipv4,omitempty"`
	IPv6      []string `json:"ipv6,omitempty" bson:"ipv6,omitempty"`
	Gateway   string   `json:"gateway,omitempty" bson:"gateway,omitempty"`
	IsDefault bool     `json:"is_default" bson:"is_default"`
}

// RouteEntry represents a single routing table entry (P1.1).
type RouteEntry struct {
	Destination string `json:"destination" bson:"destination"`
	Gateway     string `json:"gateway" bson:"gateway"`
	Interface   string `json:"interface" bson:"interface"`
	Metric      int    `json:"metric" bson:"metric"`
	Flags       string `json:"flags,omitempty" bson:"flags,omitempty"`
}

// netInfoPayload handles both old and new agent formats.
// Old agents send: LocalAddress, DefaultGateway, PublicAddress, InternetProvider, Lat, Long
// New agents send: same + Geo + Source + Interfaces + Routes (P1.1)
// The struct uses omitempty so both formats unmarshal correctly.
type netInfoPayload struct {
	// Core network info (always present)
	LocalAddress   string `json:"local_address" bson:"local_address"`
	DefaultGateway string `json:"default_gateway" bson:"default_gateway"`
	PublicAddress  string `json:"public_address" bson:"public_address"`

	// P1.1: Rich interface and route data (optional, new agents only)
	Interfaces []InterfaceInfo `json:"interfaces,omitempty" bson:"interfaces,omitempty"`
	Routes     []RouteEntry    `json:"routes,omitempty" bson:"routes,omitempty"`

	// New: Rich geographic info (optional, new agents only)
	Geo *GeoInfo `json:"geo,omitempty" bson:"geo,omitempty"`

	// Legacy fields (populated by both old and new agents for backward compat)
	InternetProvider string `json:"internet_provider,omitempty" bson:"internet_provider,omitempty"`
	Lat              string `json:"lat,omitempty" bson:"lat,omitempty"`
	Long             string `json:"long,omitempty" bson:"long,omitempty"`

	// Metadata
	Source    string    `json:"source,omitempty" bson:"source,omitempty"` // "controller" or "speedtest"
	Timestamp time.Time `json:"timestamp" bson:"timestamp"`
}

// GetISP returns the ISP name, preferring the new Geo.ISP field if available.
func (p *netInfoPayload) GetISP() string {
	if p.Geo != nil && p.Geo.ISP != "" {
		return p.Geo.ISP
	}
	return p.InternetProvider
}

// GetCountry returns the country name from Geo if available.
func (p *netInfoPayload) GetCountry() string {
	if p.Geo != nil {
		return p.Geo.Country
	}
	return ""
}

// GetCity returns the city name from Geo if available.
func (p *netInfoPayload) GetCity() string {
	if p.Geo != nil {
		return p.Geo.City
	}
	return ""
}

// GetASN returns ASN info from Geo if available.
func (p *netInfoPayload) GetASN() (uint, string) {
	if p.Geo != nil {
		return p.Geo.ASN, p.Geo.ASNOrg
	}
	return 0, ""
}

// HasRichGeoData returns true if this payload contains the new format with Geo data.
func (p *netInfoPayload) HasRichGeoData() bool {
	return p.Geo != nil
}

// NormalizeFromLegacy populates the Geo and Interfaces fields from legacy fields if not already set.
// This allows old format data to be converted to the new format when reading from DB.
func (p *netInfoPayload) NormalizeFromLegacy() {
	// Populate Geo if missing
	if p.Geo == nil {
		p.Geo = &GeoInfo{
			ISP: p.InternetProvider,
		}

		// Parse lat/long from legacy string format
		if p.Lat != "" {
			var lat float64
			if _, err := fmt.Sscanf(p.Lat, "%f", &lat); err == nil {
				p.Geo.Latitude = lat
			}
		}
		if p.Long != "" {
			var lon float64
			if _, err := fmt.Sscanf(p.Long, "%f", &lon); err == nil {
				p.Geo.Longitude = lon
			}
		}
	}

	// Populate Interfaces if missing (P1.1 backward compat)
	if len(p.Interfaces) == 0 && p.LocalAddress != "" {
		p.Interfaces = []InterfaceInfo{{
			Name:      "default",
			IPv4:      []string{p.LocalAddress},
			Gateway:   p.DefaultGateway,
			IsDefault: true,
			Type:      "unknown",
		}}
	}

	// Mark as converted
	if p.Source == "" {
		p.Source = "legacy"
	}
}

// ToNormalized returns a copy of the payload with Geo populated.
// Use this when returning data to ensure consistent format.
func (p *netInfoPayload) ToNormalized() *netInfoPayload {
	if p == nil {
		return nil
	}
	// Make a copy
	normalized := *p
	normalized.NormalizeFromLegacy()
	return &normalized
}
