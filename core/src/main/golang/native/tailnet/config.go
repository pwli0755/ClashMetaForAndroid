package tailnet

import (
	"fmt"
	"github.com/metacubex/mihomo/common/orderedmap"
	"github.com/metacubex/mihomo/config"
	"github.com/metacubex/mihomo/tunnel"
)

// Patch changes only the in-memory candidate using the current core's schema.
// Downloaded bytes are never marshalled back through RawConfig.
func Patch(cfg *config.RawConfig) error {
	if !Enabled() {
		return nil
	}
	for _, list := range [][]map[string]any{cfg.Proxy, cfg.ProxyGroup} {
		for _, entry := range list {
			if entry["name"] == Name {
				return fmt.Errorf("subscription uses reserved name %s", Name)
			}
		}
	}
	cfg.Mode = tunnel.Rule
	cfg.IPv6 = true
	cfg.DNS.Enable = true
	cfg.DNS.IPv6 = true
	if cfg.DNS.NameServerPolicy == nil {
		cfg.DNS.NameServerPolicy = orderedmap.New[string, any]()
	}
	cfg.DNS.NameServerPolicy.Set("+.ts.net", "tailscale://"+Name)
	_ = cfg.DNS.NameServerPolicy.MoveToFront("+.ts.net")
	// Preserve original destinations: sniffing must not rewrite a protected IP.
	cfg.Sniffer.Enable = false
	return nil
}
