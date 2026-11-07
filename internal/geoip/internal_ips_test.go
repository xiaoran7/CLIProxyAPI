package geoip

import (
	"net"
	"testing"
)

func TestMatchSpecialIP(t *testing.T) {
	tests := []struct {
		name        string
		ip          string
		wantSpecial bool
		wantCountry string
		wantRegion  string
	}{
		{
			name:        "IPv4 Loopback",
			ip:          "127.0.0.1",
			wantSpecial: true,
			wantCountry: "内网",
			wantRegion:  "本机",
		},
		{
			name:        "IPv6 Loopback",
			ip:          "::1",
			wantSpecial: true,
			wantCountry: "内网",
			wantRegion:  "本机",
		},
		{
			name:        "Docker Bridge Gateway",
			ip:          "172.18.0.1",
			wantSpecial: true,
			wantCountry: "内网",
			wantRegion:  "Docker",
		},
		{
			name:        "Docker Container IP",
			ip:          "172.18.0.3",
			wantSpecial: true,
			wantCountry: "内网",
			wantRegion:  "Docker",
		},
		{
			name:        "Tailscale Mesh CGNAT IP",
			ip:          "100.79.74.121",
			wantSpecial: true,
			wantCountry: "内网",
			wantRegion:  "Tailnet",
		},
		{
			name:        "Private Class A 10.x",
			ip:          "10.0.1.5",
			wantSpecial: true,
			wantCountry: "内网",
			wantRegion:  "局域网",
		},
		{
			name:        "Private Class C 192.168.x",
			ip:          "192.168.1.100",
			wantSpecial: true,
			wantCountry: "内网",
			wantRegion:  "局域网",
		},
		{
			name:        "Public IP - China",
			ip:          "119.39.133.75",
			wantSpecial: false,
		},
		{
			name:        "Public IP - Google",
			ip:          "8.8.8.8",
			wantSpecial: false,
		},
		{
			name:        "Invalid IP",
			ip:          "not-an-ip",
			wantSpecial: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed := net.ParseIP(tc.ip)
			loc := MatchSpecialIP(parsed, tc.ip)
			if tc.wantSpecial {
				if loc == nil {
					t.Fatalf("expected special IP match for %s, got nil", tc.ip)
				}
				if loc.Country != tc.wantCountry {
					t.Fatalf("expected country %s, got %s", tc.wantCountry, loc.Country)
				}
				if loc.Region != tc.wantRegion {
					t.Fatalf("expected region %s, got %s", tc.wantRegion, loc.Region)
				}
			} else {
				if loc != nil {
					t.Fatalf("expected no special match for %s, got %+v", tc.ip, loc)
				}
			}
		})
	}
}
