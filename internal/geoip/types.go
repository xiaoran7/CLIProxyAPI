package geoip

import (
	"strings"
	"time"
)

// Location represents the resolved geographical and network metadata for an IP address.
type Location struct {
	Country     string `json:"country,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
	Region      string `json:"region,omitempty"`
	City        string `json:"city,omitempty"`
	ASN         uint   `json:"asn,omitempty"`
	ASNOrg      string `json:"asn_org,omitempty"`
	IsProxy     bool   `json:"is_proxy,omitempty"`
	IsVPN       bool   `json:"is_vpn,omitempty"`
	IsTor       bool   `json:"is_tor,omitempty"`
	Formatted   string `json:"formatted,omitempty"`
}

// Config holds configuration parameters for GeoIP database loading and automated updates.
type Config struct {
	DBPath         string
	AutoUpdate     bool
	UpdateInterval time.Duration
	DownloadURL    string
}

// Format returns a human-friendly single-line representation (e.g. "中国 · 广东 · 深圳").
func (l *Location) Format() string {
	if l == nil {
		return "未知地域"
	}
	if l.Formatted != "" {
		return l.Formatted
	}

	parts := make([]string, 0, 3)
	if l.Country != "" {
		parts = append(parts, l.Country)
	}
	if l.Region != "" && l.Region != l.Country {
		parts = append(parts, l.Region)
	}
	if l.City != "" && l.City != l.Region && l.City != l.Country {
		parts = append(parts, l.City)
	}

	if len(parts) == 0 {
		if l.CountryCode != "" {
			return l.CountryCode
		}
		return "未知地域"
	}
	return strings.Join(parts, " · ")
}

// mmdbRecord corresponds to the schema structure inside Merged-IP.mmdb.
type mmdbRecord struct {
	City struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"city"`
	Country struct {
		IsoCode string            `maxminddb:"iso_code"`
		Names   map[string]string `maxminddb:"names"`
	} `maxminddb:"country"`
	RegisteredCountry struct {
		IsoCode string            `maxminddb:"iso_code"`
		Names   map[string]string `maxminddb:"names"`
	} `maxminddb:"registered_country"`
	Subdivisions []struct {
		IsoCode string            `maxminddb:"iso_code"`
		Names   map[string]string `maxminddb:"names"`
	} `maxminddb:"subdivisions"`
	ASN struct {
		AutonomousSystemNumber       uint   `maxminddb:"autonomous_system_number"`
		AutonomousSystemOrganization string `maxminddb:"autonomous_system_organization"`
		ASDomain                     string `maxminddb:"as_domain"`
	} `maxminddb:"asn"`
	Proxy struct {
		IsAnonymous bool `maxminddb:"is_anonymous"`
		IsProxy     bool `maxminddb:"is_proxy"`
		IsVPN       bool `maxminddb:"is_vpn"`
		IsTor       bool `maxminddb:"is_tor"`
		IsHosting   bool `maxminddb:"is_hosting"`
		IsCDN       bool `maxminddb:"is_cdn"`
	} `maxminddb:"proxy"`
}
