package geoip

import (
	"net"
	"strings"
)

// MatchSpecialIP checks if an IP is loopback, Docker bridge, Tailscale, or private network,
// returning a pre-filled Location if so, or nil if it is an external IP requiring database lookup.
func MatchSpecialIP(ip net.IP, rawAddr string) *Location {
	if ip == nil {
		clean := strings.TrimSpace(rawAddr)
		if clean == "" {
			return nil
		}
		ip = net.ParseIP(clean)
		if ip == nil {
			return nil
		}
	}

	// 1. Loopback (127.0.0.0/8, ::1)
	if ip.IsLoopback() {
		return &Location{
			Country:     "内网",
			CountryCode: "LAN",
			Region:      "本机",
			City:        "回环地址",
			ASNOrg:      "Loopback",
			Formatted:   "内网 · 本机回环",
		}
	}

	ip4 := ip.To4()
	if ip4 != nil {
		// 2. Docker Bridge default subnets (172.17.0.0/16, 172.18.0.0/16, 172.19.0.0/16)
		if ip4[0] == 172 && (ip4[1] >= 17 && ip4[1] <= 22) {
			city := "容器网络"
			if ip4[2] == 0 && ip4[3] == 1 {
				city = "网桥网关"
			}
			return &Location{
				Country:     "内网",
				CountryCode: "LAN",
				Region:      "Docker",
				City:        city,
				ASNOrg:      "Docker Bridge",
				Formatted:   "内网 · Docker Gateway",
			}
		}

		// 3. Tailscale CGNAT mesh (100.64.0.0/10)
		if ip4[0] == 100 && (ip4[1]&0xc0) == 64 {
			return &Location{
				Country:     "内网",
				CountryCode: "VPN",
				Region:      "Tailnet",
				City:        "私有网格",
				ASNOrg:      "Tailscale Mesh",
				Formatted:   "内网 · Tailnet 专网",
			}
		}

		// 4. Standard RFC 1918 Private ranges
		// 10.0.0.0/8
		if ip4[0] == 10 {
			return &Location{
				Country:     "内网",
				CountryCode: "LAN",
				Region:      "局域网",
				City:        "私有地址 (10/8)",
				ASNOrg:      "Intranet",
				Formatted:   "内网 · 局域网私网",
			}
		}
		// 172.16.0.0/12 (remaining non-Docker)
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return &Location{
				Country:     "内网",
				CountryCode: "LAN",
				Region:      "局域网",
				City:        "私有地址 (172/12)",
				ASNOrg:      "Intranet",
				Formatted:   "内网 · 局域网私网",
			}
		}
		// 192.168.0.0/16
		if ip4[0] == 192 && ip4[1] == 168 {
			return &Location{
				Country:     "内网",
				CountryCode: "LAN",
				Region:      "局域网",
				City:        "家庭/办公网络",
				ASNOrg:      "Intranet",
				Formatted:   "内网 · 局域网私网",
			}
		}

		// Link local / unspecified
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return &Location{
				Country:     "内网",
				CountryCode: "LAN",
				Region:      "本地链路",
				City:        "Link-Local",
				ASNOrg:      "Local",
				Formatted:   "内网 · 本地链路",
			}
		}
	} else if len(ip) == 16 {
		// IPv6 Unique Local Address (fc00::/7)
		if (ip[0] & 0xfe) == 0xfc {
			return &Location{
				Country:     "内网",
				CountryCode: "LAN",
				Region:      "IPv6 ULA",
				City:        "私有网格",
				ASNOrg:      "IPv6 Intranet",
				Formatted:   "内网 · IPv6 专网",
			}
		}
	}

	return nil
}
