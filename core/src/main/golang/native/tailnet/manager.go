package tailnet

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	"github.com/metacubex/mihomo/component/resolver"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/dns"
	"github.com/metacubex/mihomo/tunnel"
	"github.com/metacubex/tailscale/ipn"
	"net"
)

var QueryUID = func(net.Addr, net.Addr) int { return -1 }

var mu sync.Mutex
var settings Settings
var instance *outbound.Tailscale
var managed C.Proxy
var direct = adapter.NewProxy(outbound.NewDirect())
var reject = adapter.NewProxy(outbound.NewReject())
var knownRoutes []netip.Prefix
var activeRoutes []netip.Prefix
var lastError string
var polling bool
var backendReady bool

// The owner is the application service, not a subscription provider.
type persistentAdapter struct{ *outbound.Tailscale }

func (*persistentAdapter) Close() error { return nil }

func Configure(data string) error {
	var next Settings
	if err := json.Unmarshal([]byte(data), &next); err != nil {
		return err
	}
	next.Hostname = strings.TrimSpace(next.Hostname)
	if len(next.Hostname) > 63 || strings.ContainsAny(next.Hostname, "/\\\n\r\t ") {
		return errors.New("invalid Tailscale device name")
	}
	mu.Lock()
	defer mu.Unlock()
	if backendReady && instance != nil && (next.Hostname != settings.Hostname || next.AcceptRoutes != settings.AcceptRoutes || next.Enabled != settings.Enabled) {
		lc, err := instance.ManagedClient()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err = lc.EditPrefs(ctx, &ipn.MaskedPrefs{Prefs: ipn.Prefs{Hostname: next.Hostname, RouteAll: next.AcceptRoutes, WantRunning: next.Enabled}, HostnameSet: true, RouteAllSet: true, WantRunningSet: true})
		if err != nil {
			return err
		}
	}
	settings = next
	if !next.Enabled {
		dns.SetManagedTailnet("")
		tunnel.SetAppPolicy(nil)
		return nil
	}
	if instance == nil {
		dir := C.Path.Resolve("cmfa-tailnet")
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		if err := os.Chmod(dir, 0700); err != nil {
			return err
		}
		if data, err := os.ReadFile(filepath.Join(dir, "routes.json")); err == nil {
			_ = json.Unmarshal(data, &knownRoutes)
		}
		accept := next.AcceptRoutes
		t, err := outbound.NewManagedTailscale(outbound.TailscaleOption{Name: Name, Hostname: next.Hostname, StateDir: dir, UDP: true, AcceptRoutes: &accept})
		if err != nil {
			return err
		}
		instance = t
		managed = adapter.NewProxy(&persistentAdapter{t})
	}
	dns.SetManagedTailnetQuery(instance.ManagedDNS)
	dns.SetManagedTailnet(Name)
	tunnel.SetAppPolicy(route)
	if !polling {
		polling = true
		go poll()
	}
	return nil
}

func Enabled() bool { mu.Lock(); defer mu.Unlock(); return settings.Enabled }

func route(m *C.Metadata) C.Proxy {
	mu.Lock()
	refreshRoutes := settings.Enabled && settings.AcceptRoutes
	mu.Unlock()
	if refreshRoutes {
		// Query the current route map for each new flow; a timer alone has a leak
		// window when a subnet is newly approved or withdrawn.
		if snapshot("routes").State == "error" {
			return reject
		}
		if m.Host != "" && !m.DstIP.IsValid() && !tailDestination(m.Host, m.DstIP, nil) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			ip, err := resolver.ResolveIP(ctx, m.Host)
			cancel()
			if err != nil {
				return reject
			}
			m.DstIP = ip
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if !settings.Enabled {
		return nil
	}
	routes := knownRoutes
	if !settings.AcceptRoutes {
		routes = nil
	}
	var originalIP netip.Addr
	switch address := m.RawDstAddr.(type) {
	case *net.TCPAddr:
		originalIP, _ = netip.AddrFromSlice(address.IP)
	case *net.UDPAddr:
		originalIP, _ = netip.AddrFromSlice(address.IP)
	}
	originalIP = originalIP.Unmap()
	if tailDestination("", originalIP, routes) {
		m.DstIP, m.Host = originalIP, ""
	}
	if tailDestination(m.Host, m.DstIP, routes) {
		if tailDestination(m.Host, m.DstIP, nil) || tailDestination("", m.DstIP, activeRoutes) {
			return managed
		}
		return reject // withdrawn or not yet restored routes must not escape to the subscription
	}
	uid := -1
	if m.RawSrcAddr != nil && m.RawDstAddr != nil {
		uid = QueryUID(m.RawSrcAddr, m.RawDstAddr)
	}
	if !permitted(uid, settings.AllowedUIDs) {
		return direct
	}
	return nil
}

type Status struct {
	KeyExpiry int64          `json:"keyExpiry,omitempty"`
	State     string         `json:"state"`
	IPs       []string       `json:"ips"`
	Routes    []netip.Prefix `json:"routes"`
	Error     string         `json:"error,omitempty"`
	AuthURL   string         `json:"authUrl,omitempty"`
}

func snapshot(action string) Status {
	mu.Lock()
	current, enabled, ready := instance, settings.Enabled, backendReady
	mu.Unlock()
	result := Status{State: "offline", IPs: []string{}, Routes: []netip.Prefix{}}
	if !ready {
		return result
	}
	if current == nil {
		result.State = "unlogged"
		return result
	}
	lc, err := current.ManagedClient()
	if err != nil {
		result.State = "error"
		result.Error = "Tailscale backend could not start"
		return result
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	switch action {
	case "login":
		err = lc.StartLoginInteractive(ctx)
	case "network":
		if enabled {
			err = lc.DebugAction(ctx, "rebind")
		}
	case "logout":
		err = lc.Logout(ctx)
	case "retry":
		_, err = lc.EditPrefs(ctx, &ipn.MaskedPrefs{Prefs: ipn.Prefs{WantRunning: enabled}, WantRunningSet: true})
	}
	if err != nil {
		result.State = "error"
		result.Error = "Tailscale " + action + " failed; retry when network is available"
		return result
	}
	st, err := lc.Status(ctx)
	if action == "login" && err == nil {
		for st.AuthURL == "" && st.BackendState != "Running" && ctx.Err() == nil {
			time.Sleep(100 * time.Millisecond)
			st, err = lc.Status(ctx)
			if err != nil {
				break
			}
		}
	}
	if err != nil {
		result.State = "error"
		result.Error = "Cannot read Tailscale state; retry"
		return result
	}
	switch st.BackendState {
	case "NeedsLogin", "NeedsMachineAuth":
		result.State = "unlogged"
	case "Starting", "NoState":
		result.State = "connecting"
	case "Running":
		result.State = "connected"
		if st.Self == nil || !st.Self.Online {
			result.State = "offline"
		}
	}
	if !enabled {
		result.State = "offline"
	}
	if st.Self != nil && st.Self.KeyExpiry != nil {
		result.KeyExpiry = st.Self.KeyExpiry.Unix()
	}
	for _, ip := range st.TailscaleIPs {
		result.IPs = append(result.IPs, ip.String())
	}
	mu.Lock()
	defer mu.Unlock()
	activeRoutes = nil
	routesChanged := false
	if settings.AcceptRoutes && st.BackendState == "Running" {
		for _, peer := range st.Peer {
			if peer.PrimaryRoutes == nil {
				continue
			}
			for _, prefix := range peer.PrimaryRoutes.AsSlice() {
				if prefix.Bits() == 0 {
					continue
				} // no exit nodes
				activeRoutes = append(activeRoutes, prefix)
				found := false
				for _, old := range knownRoutes {
					if old == prefix {
						found = true
						break
					}
				}
				if !found {
					knownRoutes = append(knownRoutes, prefix)
					routesChanged = true
				}
			}
		}
		if routesChanged {
			data, _ := json.Marshal(knownRoutes)
			path := C.Path.Resolve("cmfa-tailnet/routes.json")
			if err := os.WriteFile(path+".tmp", data, 0600); err == nil {
				_ = os.Rename(path+".tmp", path)
			}
		}
	}
	result.Routes = append(result.Routes, activeRoutes...)
	// Only an explicit login action may expose the current embedded instance's URL.
	if action == "login" {
		u, parseErr := url.Parse(st.AuthURL)
		if parseErr == nil && u.Scheme == "https" && u.Host == "login.tailscale.com" && u.User == nil && strings.HasPrefix(u.Path, "/a/") {
			result.AuthURL = st.AuthURL
		}
	}
	if lastError != "" {
		result.Error = lastError
	}
	return result
}

func Command(action string) string {
	result := snapshot(action)
	data, _ := json.Marshal(result)
	return string(data)
}

func poll() {
	for {
		_ = snapshot("status")
		time.Sleep(3 * time.Second)
	}
}

func ConfigError(err error) {
	mu.Lock()
	defer mu.Unlock()
	lastError = ""
	if err != nil {
		lastError = err.Error()
	}
}

func Pause() {
	mu.Lock()
	next := settings
	mu.Unlock()
	next.Enabled = false
	data, _ := json.Marshal(next)
	_ = Configure(string(data))
}

// Android must install socket protection before tsnet starts any sockets.
func SetReady(ready bool) {
	mu.Lock()
	wasReady := backendReady
	backendReady = ready
	current, options := instance, settings
	mu.Unlock()
	if current == nil || (!ready && !wasReady) {
		return
	}
	lc, err := current.ManagedClient()
	if err != nil {
		ConfigError(err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = lc.EditPrefs(ctx, &ipn.MaskedPrefs{Prefs: ipn.Prefs{Hostname: options.Hostname, RouteAll: options.AcceptRoutes, WantRunning: ready && options.Enabled}, HostnameSet: true, RouteAllSet: true, WantRunningSet: true})
	if err != nil {
		ConfigError(err)
	}
}
