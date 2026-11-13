package tailnet

import (
	"net/netip"
	"strings"
)

const Name = "CMFA-INTERNAL-TAILSCALE-v1"

var tailPrefixes = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("fd7a:115c:a1e0::/48"),
}

type Settings struct {
	Enabled      bool   `json:"enabled"`
	Hostname     string `json:"hostname"`
	AcceptRoutes bool   `json:"acceptRoutes"`
	AllowedUIDs  []int  `json:"allowedUids"`
}

func tailDestination(host string, ip netip.Addr, routes []netip.Prefix) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "ts.net" || strings.HasSuffix(host, ".ts.net") {
		return true
	}
	for _, prefix := range tailPrefixes {
		if prefix.Contains(ip) {
			return true
		}
	}
	for _, prefix := range routes {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

func permitted(uid int, allowed []int) bool {
	if uid < 10000 {
		return false
	}
	for _, candidate := range allowed {
		if candidate == uid {
			return true
		}
	}
	return false
}
