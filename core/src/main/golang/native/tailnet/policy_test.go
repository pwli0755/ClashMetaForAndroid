package tailnet

import (
	"net"
	"net/netip"
	"reflect"
	"testing"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	"github.com/metacubex/mihomo/common/yaml"
	"github.com/metacubex/mihomo/config"
	C "github.com/metacubex/mihomo/constant"
)

func TestDestinations(t *testing.T) {
	routes := []netip.Prefix{netip.MustParsePrefix("192.168.50.0/24")}
	for _, tc := range []struct {
		host, ip string
		want     bool
	}{
		{"Mac.Example.ts.net.", "", true}, {"evilts.net", "", false},
		{"example.ts.net.evil.com", "", false}, {"nas", "", false},
		{"", "100.64.0.1", true}, {"", "fd7a:115c:a1e0::123", true},
		{"", "192.168.50.3", true}, {"", "192.168.1.1", false},
		{"example.org", "1.1.1.1", false},
	} {
		ip, _ := netip.ParseAddr(tc.ip)
		if got := tailDestination(tc.host, ip, routes); got != tc.want {
			t.Errorf("%+v: got %v", tc, got)
		}
	}
}

func TestApplicationPolicyOrder(t *testing.T) {
	oldSettings, oldManaged, oldQuery := settings, managed, QueryUID
	defer func() { settings, managed, QueryUID = oldSettings, oldManaged, oldQuery }()
	settings = Settings{Enabled: true, AllowedUIDs: []int{10042}}
	managed = reject // stand-in: never dial from a unit test
	uid := 10042
	QueryUID = func(net.Addr, net.Addr) int { return uid }
	m := &C.Metadata{Host: "service.tail.ts.net", RawSrcAddr: &net.TCPAddr{}, RawDstAddr: &net.TCPAddr{}}
	for _, value := range []int{10042, 10043, -1} {
		uid = value
		if route(m) != managed {
			t.Fatal("Tailnet must precede app permissions")
		}
	}
	m.Host = "public.example"
	uid = 10042
	if route(m) != nil {
		t.Fatal("allowed app must retain subscription rules")
	}
	for _, value := range []int{10043, -1, 0} {
		uid = value
		if route(m) != direct {
			t.Fatal("unrecognized/unselected source must use DIRECT")
		}
	}
	settings.Enabled = false
	if route(m) != nil {
		t.Fatal("disabled integration must preserve old behavior")
	}
}

func TestPatchIdempotentAndSubscriptionUpdates(t *testing.T) {
	old := settings
	defer func() { settings = old }()
	settings = Settings{Enabled: true}
	for _, name := range []string{"airport-A", "airport-renamed-B"} {
		source := []byte("proxies:\n  - name: " + name + "\n    type: direct\nrules:\n  - MATCH," + name + "\nfuture-field:\n  nested: preserved\n")
		original := append([]byte(nil), source...)
		cfg, err := config.UnmarshalRawConfig(source)
		if err != nil {
			t.Fatal(err)
		}
		if err = Patch(cfg); err != nil {
			t.Fatal(err)
		}
		first, _ := yaml.Marshal(cfg)
		if err = Patch(cfg); err != nil {
			t.Fatal(err)
		}
		second, _ := yaml.Marshal(cfg)
		if string(first) != string(second) {
			t.Fatal("patch is not idempotent")
		}
		if !reflect.DeepEqual(source, original) {
			t.Fatal("source was changed")
		}
		if len(cfg.Proxy) != 1 || cfg.Proxy[0]["name"] != name || cfg.Rule[0] != "MATCH,"+name {
			t.Fatal("subscription references changed")
		}
	}
}

func TestConflictRejectsCandidateWithoutMutation(t *testing.T) {
	old := settings
	defer func() { settings = old }()
	settings = Settings{Enabled: true}
	for _, group := range []bool{false, true} {
		cfg := config.DefaultRawConfig()
		entry := map[string]any{"name": Name, "type": "select"}
		if group {
			cfg.ProxyGroup = []map[string]any{entry}
		} else {
			cfg.Proxy = []map[string]any{entry}
		}
		before, _ := yaml.Marshal(cfg)
		if Patch(cfg) == nil {
			t.Fatal("reserved name collision accepted")
		}
		after, _ := yaml.Marshal(cfg)
		if string(before) != string(after) {
			t.Fatal("rejected candidate was mutated")
		}
	}
}

func TestOriginalTailnetDestinationCannotBeRewritten(t *testing.T) {
	oldSettings, oldManaged := settings, managed
	defer func() { settings, managed = oldSettings, oldManaged }()
	settings = Settings{Enabled: true}
	managed = reject
	for _, address := range []net.Addr{
		&net.TCPAddr{IP: net.ParseIP("100.64.1.2"), Port: 443},
		&net.UDPAddr{IP: net.ParseIP("fd7a:115c:a1e0::123"), Port: 443},
	} {
		m := &C.Metadata{Host: "rewritten.public.example", DstIP: netip.MustParseAddr("1.1.1.1"), RawDstAddr: address}
		if route(m) != managed {
			t.Fatal("original Tailnet target escaped")
		}
		if m.Host != "" {
			t.Fatal("rewritten hostname could alter the protected destination")
		}
	}
}

func TestSubnetRoutePolicy(t *testing.T) {
	oldSettings, oldManaged, oldKnown, oldActive, oldReady := settings, managed, knownRoutes, activeRoutes, backendReady
	defer func() {
		settings, managed, knownRoutes, activeRoutes, backendReady = oldSettings, oldManaged, oldKnown, oldActive, oldReady
	}()
	// Supply route snapshots directly; this test must not start a real backend.
	backendReady = false
	settings = Settings{Enabled: true, AcceptRoutes: true}
	managed = adapter.NewProxy(outbound.NewDirect())
	knownRoutes = []netip.Prefix{netip.MustParsePrefix("192.168.50.0/24")}
	activeRoutes = append([]netip.Prefix(nil), knownRoutes...)
	m := &C.Metadata{DstIP: netip.MustParseAddr("192.168.50.42")}
	if route(m) != managed {
		t.Fatal("accepted subnet must precede application permissions")
	}
	activeRoutes = nil
	if route(m) != reject {
		t.Fatal("withdrawn subnet must not fall through to DIRECT or subscription rules")
	}
	m.DstIP = netip.MustParseAddr("192.168.1.42")
	if route(m) != direct {
		t.Fatal("unadvertised private subnet must remain DIRECT for unselected apps")
	}
	settings.AcceptRoutes = false
	m.DstIP = netip.MustParseAddr("192.168.50.42")
	if route(m) != direct {
		t.Fatal("disabled subnet routing must restore ordinary app policy")
	}
}
