//go:build linux

package gateway

import (
	"context"
	"github.com/Traliaa/vpn-manager/internal/vpn/singbox"
	"go.uber.org/zap"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Execute only via unshare -n, never in the home server's network namespace.
func TestLinuxProfileGatewayLifecycle(t *testing.T) {
	if os.Getenv("VPN_MANAGER_LINUX_TEST") != "1" {
		t.Skip("requires isolated Linux namespace")
	}
	current, e1 := os.Readlink("/proc/self/ns/net")
	initial, e2 := os.Readlink("/proc/1/ns/net")
	if e1 != nil || e2 != nil || current == initial {
		t.Fatal("refusing test outside an isolated network namespace")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := func(name string, args ...string) string {
		t.Helper()
		out, err := runCommand(ctx, name, args, "")
		if err != nil {
			t.Fatalf("%s %v: %v", name, args, err)
		}
		return string(out)
	}
	command("ip", "link", "set", "lo", "up")
	command("ip", "link", "add", "eth0", "type", "dummy")
	command("ip", "addr", "add", "192.168.1.40/24", "dev", "eth0")
	command("ip", "link", "set", "eth0", "up")
	command("ip", "route", "add", "default", "via", "192.168.1.1", "dev", "eth0")
	command("sysctl", "-w", "net.ipv4.ip_forward=1")
	mainBefore := command("ip", "-j", "-4", "route", "show", "table", "main")
	command("ip", "-4", "route", "add", "blackhole", "203.0.113.0/24", "table", "60000")
	foreignBefore := command("ip", "-j", "-4", "route", "show", "table", "60000")
	binary := os.Getenv("SINGBOX_TEST_BINARY")
	if binary == "" {
		t.Fatal("SINGBOX_TEST_BINARY required")
	}
	ctrl, err := singbox.NewController(singbox.Config{ConfigPath: filepath.Join(t.TempDir(), "config.json"), BinaryPath: binary}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer ctrl.Stop(context.Background())
	m := NewManager(zap.NewNop())
	m.profileCtrl = ctrl
	m.statePath = filepath.Join(t.TempDir(), "gateway.json")
	cfg := ProfileConfig{ClientIP: "192.168.1.71", LANInterface: "eth0"}
	if err = m.EnableProfile(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	route := command("ip", "-4", "route", "get", "198.51.100.1", "from", cfg.ClientIP, "iif", "eth0")
	if !strings.Contains(route, "dev vmtun0") {
		t.Fatalf("selected source not intercepted: %s", route)
	}
	route = command("ip", "-4", "route", "get", "198.51.100.1", "from", "192.168.1.72", "iif", "eth0")
	if strings.Contains(route, "vmtun0") {
		t.Fatal("other device intercepted")
	}
	route = command("ip", "-4", "route", "get", "192.168.1.200", "from", cfg.ClientIP, "iif", "eth0")
	if strings.Contains(route, "vmtun0") {
		t.Fatal("LAN destination intercepted")
	}
	// Simulate unexpected child exit. Its TUN default disappears, while the
	// persistent blackhole prevents falling through to the host main route.
	if err = ctrl.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if m.Status().Ready {
		t.Fatal("stopped proxy reported ready")
	}
	if _, err = runCommand(ctx, "ip", []string{"-4", "route", "get", "198.51.100.1", "from", cfg.ClientIP, "iif", "eth0"}, ""); err == nil {
		t.Fatal("proxy outage fell through to direct")
	}
	if err = m.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	route = command("ip", "-4", "route", "get", "198.51.100.1", "from", cfg.ClientIP, "iif", "eth0")
	if !strings.Contains(route, "dev vmtun0") {
		t.Fatal("TUN route not restored")
	}
	if err = m.Disable(ctx); err != nil {
		t.Fatal(err)
	}
	route = command("ip", "-4", "route", "get", "198.51.100.1", "from", cfg.ClientIP, "iif", "eth0")
	if strings.Contains(route, "vmtun0") || !strings.Contains(route, "192.168.1.1") {
		t.Fatalf("direct fallback missing: %s", route)
	}
	if err = m.Disable(ctx); err != nil {
		t.Fatal("repeated disable: ", err)
	}
	if err = m.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if command("ip", "-j", "-4", "route", "show", "table", "main") != mainBefore {
		t.Fatal("host main route changed")
	}
	if command("ip", "-j", "-4", "route", "show", "table", "60000") != foreignBefore {
		t.Fatal("foreign route changed")
	}
	if _, err = runCommand(ctx, "nft", []string{"-j", "list", "table", "inet", profileNFT}, ""); err != errObjectMissing {
		t.Fatal("profile NAT not released: ", err)
	}
}
