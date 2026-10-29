package gateway

import (
	"context"
	"errors"
	"go.uber.org/zap"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeProxy struct {
	enabled bool
	dead    bool
	starts  int
}

func (p *fakeProxy) SetGateway(_ context.Context, on bool) error {
	p.enabled = on
	if on {
		p.dead = false
		p.starts++
	}
	return nil
}
func (p *fakeProxy) IsRunning(context.Context) (bool, error) { return !p.dead, nil }

type profileHarness struct {
	commands []string
	foreign  bool
	failRule bool
	nat      bool
}

func (h *profileHarness) run(_ context.Context, name string, args []string, input string) ([]byte, error) {
	cmd := name + " " + strings.Join(args, " ")
	h.commands = append(h.commands, cmd+" "+input)
	if cmd == "ip -j -4 route show table all" {
		if h.foreign {
			return []byte(`[{"table":51840,"dst":"default","protocol":"static","dev":"eth0"}]`), nil
		}
		return []byte(`[]`), nil
	}
	if cmd == "ip -j -4 rule show" {
		return []byte(`[]`), nil
	}
	if cmd == "sysctl -n net.ipv4.ip_forward" {
		return []byte("1\n"), nil
	}
	if strings.HasPrefix(cmd, "nft -j list table") {
		if !h.nat {
			return nil, errObjectMissing
		}
		return []byte(`{"nftables":[{"table":{"family":"inet","name":"vpn_manager_profile","comment":"vpn-manager profile gateway"}}]}`), nil
	}
	if name == "nft" && input != "" {
		h.nat = true
	}
	if h.failRule && strings.HasPrefix(cmd, "ip -4 rule add") {
		return nil, errors.New("rule failure")
	}
	return nil, nil
}
func gatewayFixture(t *testing.T) (*Manager, *fakeProxy, *profileHarness) {
	t.Helper()
	m := NewManager(zap.NewNop())
	p := &fakeProxy{}
	h := &profileHarness{}
	m.profileCtrl = p
	m.run = h.run
	m.statePath = filepath.Join(t.TempDir(), "gateway.json")
	m.localAddresses = func() ([]netip.Addr, error) { return []netip.Addr{netip.MustParseAddr("192.168.1.40")}, nil }
	return m, p, h
}
func TestProfileRejectsUnsafeScopeBeforeNetworkChanges(t *testing.T) {
	for _, raw := range []string{"", "192.168.1.0/24", "192.168.1.40", "127.0.0.1", "::1", "224.0.0.1"} {
		m, p, h := gatewayFixture(t)
		if err := m.EnableProfile(context.Background(), ProfileConfig{ClientIP: raw, LANInterface: "eth0"}); err == nil {
			t.Errorf("accepted unsafe scope %q", raw)
		}
		if len(h.commands) != 0 || p.enabled {
			t.Fatal("invalid scope touched network")
		}
	}
}
func TestProfileCapturesOneHostAndDisablesToDirect(t *testing.T) {
	m, p, h := gatewayFixture(t)
	ctx := context.Background()
	if err := m.EnableProfile(ctx, ProfileConfig{ClientIP: "192.168.1.71", LANInterface: "eth0"}); err != nil {
		t.Fatal(err)
	}
	status := m.Status()
	if !status.Enabled || !status.Ready || status.ClientIP != "192.168.1.71" {
		t.Fatalf("wrong status %#v", status)
	}
	found := false
	for _, cmd := range h.commands {
		if strings.HasPrefix(cmd, "ip -4 rule add") {
			if !strings.Contains(cmd, "from 192.168.1.71/32") || !strings.Contains(cmd, "iif eth0") {
				t.Fatalf("unscoped capture %s", cmd)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("no source-specific capture rule")
	}
	if err := m.Disable(ctx); err != nil {
		t.Fatal(err)
	}
	if p.enabled || m.Status().Enabled || !m.Status().Ready {
		t.Fatal("disable did not enter direct/pass-through mode")
	}
	if !h.nat {
		t.Fatal("disable removed direct fallback while router may still point at LXC")
	}
	data, err := os.ReadFile(m.statePath)
	if err != nil || !strings.Contains(string(data), `"enabled":false`) {
		t.Fatal("disabled state not persisted")
	}
}
func TestProfileSetupFailureRollsBack(t *testing.T) {
	m, p, h := gatewayFixture(t)
	h.failRule = true
	if err := m.EnableProfile(context.Background(), ProfileConfig{ClientIP: "192.168.1.71", LANInterface: "eth0"}); err == nil {
		t.Fatal("ignored routing failure")
	}
	if p.enabled || m.Status().Enabled {
		t.Fatal("partial failure left capture active")
	}
	if _, err := os.Stat(m.statePath); !os.IsNotExist(err) {
		t.Fatal("failed operation persisted active state")
	}
}
func TestForeignTableIsNotOverwritten(t *testing.T) {
	m, p, h := gatewayFixture(t)
	h.foreign = true
	if err := m.EnableProfile(context.Background(), ProfileConfig{ClientIP: "192.168.1.71", LANInterface: "eth0"}); err == nil {
		t.Fatal("foreign table accepted")
	}
	if p.enabled {
		t.Fatal("foreign table modified proxy")
	}
	for _, cmd := range h.commands {
		if strings.Contains(cmd, "route add") || strings.Contains(cmd, "route flush") {
			t.Fatalf("foreign table modified: %s", cmd)
		}
	}
}

func TestRestoreDisabledProfileRetainsDirectMode(t *testing.T) {
	m, p, _ := gatewayFixture(t)
	ctx := context.Background()
	cfg := ProfileConfig{ClientIP: "192.168.1.71", LANInterface: "eth0"}
	if err := m.EnableProfile(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	if err := m.Disable(ctx); err != nil {
		t.Fatal(err)
	}
	restored, _, h := gatewayFixture(t)
	restored.statePath = m.statePath
	h.nat = true
	if err := restored.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	if restored.Status().Enabled || !restored.Status().Ready || p.enabled {
		t.Fatal("disabled profile restored as capture")
	}
}
func TestRestoreRejectsForeignRouteWithoutMutation(t *testing.T) {
	m, _, _ := gatewayFixture(t)
	ctx := context.Background()
	if err := m.EnableProfile(ctx, ProfileConfig{ClientIP: "192.168.1.71", LANInterface: "eth0"}); err != nil {
		t.Fatal(err)
	}
	restored, p, h := gatewayFixture(t)
	restored.statePath = m.statePath
	h.foreign = true
	h.nat = true
	if err := restored.Restore(ctx); err == nil {
		t.Fatal("foreign route adopted")
	}
	if p.enabled {
		t.Fatal("proxy changed before ownership validation")
	}
	for _, cmd := range h.commands {
		if strings.Contains(cmd, " add ") || strings.Contains(cmd, " delete ") {
			t.Fatal("restore changed foreign network")
		}
	}
}

func TestRecoveryRestoresOnlyConfiguredClientAfterProxyDeath(t *testing.T) {
	m, p, h := gatewayFixture(t)
	ctx := context.Background()
	if err := m.EnableProfile(ctx, ProfileConfig{ClientIP: "192.168.1.71", LANInterface: "eth0"}); err != nil {
		t.Fatal(err)
	}
	p.dead = true
	if m.Status().Ready {
		t.Fatal("dead proxy reported ready")
	}
	h.commands = nil
	if err := m.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if p.dead || p.starts != 2 || !m.Status().Ready {
		t.Fatal("proxy not recovered")
	}
	if err := m.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if p.starts != 2 {
		t.Fatal("healthy proxy restarted")
	}
	for _, cmd := range h.commands {
		if strings.HasPrefix(cmd, "ip -4 rule add") && !strings.Contains(cmd, "from 192.168.1.71/32 iif eth0") {
			t.Fatalf("wrong scope: %s", cmd)
		}
	}
}
