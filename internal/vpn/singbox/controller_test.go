package singbox

import (
	"context"
	"encoding/json"
	"go.uber.org/zap"
	"os"
	"path/filepath"
	"testing"
)

type testOutbound struct{ tag string }

func (o testOutbound) Tag() string { return o.tag }
func (o testOutbound) BuildOutbound() (map[string]any, error) {
	return map[string]any{"type": "direct", "tag": o.tag}, nil
}
func fakeBinary(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "sing-box")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestRejectedCandidatePreservesPreviousConfigAndOutbounds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	old := []byte(`{"log":{"level":"warn"}}`)
	if err := os.WriteFile(path, old, 0600); err != nil {
		t.Fatal(err)
	}
	c, err := NewController(Config{ConfigPath: path, BinaryPath: fakeBinary(t, "exit 2")}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.AddOutbound(context.Background(), testOutbound{"new"}); err == nil {
		t.Fatal("rejected configuration reported success")
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(old) {
		t.Fatal("bad candidate replaced working configuration")
	}
	if len(c.ListOutbounds()) != 0 {
		t.Fatal("bad candidate changed active outbounds")
	}
}
func TestMissingRouteOutboundIsRejected(t *testing.T) {
	c, err := NewController(Config{ConfigPath: filepath.Join(t.TempDir(), "config.json"), BinaryPath: fakeBinary(t, "exit 0")}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	err = c.GenerateConfigWithRouting(context.Background(), nil, RouteConfig{Final: "direct", Rules: []RouteRule{{OutboundTag: "missing", Domain: []string{"example.com"}}}})
	if err == nil {
		t.Fatal("accepted rule pointing to absent outbound")
	}
}
func TestDefaultDirectAndBlockAreAlwaysPresent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	c, err := NewController(Config{ConfigPath: path, BinaryPath: fakeBinary(t, "exit 0")}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetOutbounds(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var cfg struct {
		Outbounds []struct {
			Tag string `json:"tag"`
		}
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	tags := map[string]bool{}
	for _, o := range cfg.Outbounds {
		tags[o.Tag] = true
	}
	if !tags["direct"] || !tags["block"] {
		t.Fatal("built-in route targets missing")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("configuration file must be private")
	}
}
func TestReloadReportsEarlyExit(t *testing.T) {
	c, err := NewController(Config{ConfigPath: filepath.Join(t.TempDir(), "config.json"), BinaryPath: fakeBinary(t, "case \"$1\" in check) exit 0;; *) exit 7;; esac")}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetOutbounds(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Reload(context.Background()); err == nil {
		t.Fatal("dead child reported as running")
	}
	running, _ := c.IsRunning(context.Background())
	if running {
		t.Fatal("exited child reported running")
	}
}
func TestInterfaceOutboundBindsBothInterfaceAndSource(t *testing.T) {
	ob := InterfaceOutbound{Name: "budapest", Interface: "awg_test", Address: "10.0.0.2/32"}
	got, err := ob.BuildOutbound()
	if err != nil {
		t.Fatal(err)
	}
	if got["tag"] != "budapest" || got["bind_interface"] != "awg_test" || got["inet4_bind_address"] != "10.0.0.2" {
		t.Fatalf("VPN source/interface lost: %#v", got)
	}
}

func TestFailedReloadRestoresPreviouslyRunningConfig(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "fail")
	binary := fakeBinary(t, "if [ \"$1\" = check ]; then exit 0; fi\nif [ -f '"+marker+"' ]; then rm '"+marker+"'; exit 7; fi\nexec sleep 60")
	path := filepath.Join(dir, "config.json")
	c, err := NewController(Config{ConfigPath: path, BinaryPath: binary}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Stop(context.Background())
	if err := c.SetOutbounds(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	old, _ := os.ReadFile(path)
	if err := c.AddOutbound(context.Background(), testOutbound{"new"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := c.Reload(context.Background()); err == nil {
		t.Fatal("failed restart reported success")
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(old) {
		t.Fatal("failed reload did not restore accepted config")
	}
	if running, _ := c.IsRunning(context.Background()); !running {
		t.Fatal("previous runtime was not restarted")
	}
	if _, ok := c.GetOutbound("new"); ok {
		t.Fatal("rejected outbound remained active")
	}
}

func TestRemovingReferencedVPNBlocksInsteadOfLeakingDirect(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	c, err := NewController(Config{ConfigPath: path, BinaryPath: fakeBinary(t, "exit 0")}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.GenerateConfigWithRouting(context.Background(), []Outbound{testOutbound{"vpn"}}, RouteConfig{Final: "direct", Rules: []RouteRule{{OutboundTag: "vpn", Domain: []string{"example.com"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveOutbound(context.Background(), "vpn"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var cfg struct{ Outbounds []map[string]any }
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	for _, ob := range cfg.Outbounds {
		if ob["tag"] == "vpn" {
			if ob["type"] != "block" {
				t.Fatal("disabled VPN route can leak direct")
			}
			return
		}
	}
	t.Fatal("referenced VPN target disappeared")
}

func TestGatewayAddsManualTunWithoutGlobalRouting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	c, err := NewController(Config{ConfigPath: path, BinaryPath: fakeBinary(t, "if [ \"$1\" = check ]; then exit 0; fi\nexec sleep 60")}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Stop(context.Background())
	if err := c.SetGateway(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var cfg struct{ Inbounds []map[string]any }
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, in := range cfg.Inbounds {
		if in["type"] == "tun" {
			found = true
			if in["auto_route"] != false || in["interface_name"] != "vmtun0" {
				t.Fatal("TUN would modify global routes")
			}
		}
	}
	if !found {
		t.Fatal("no transparent inbound")
	}
	if err := c.SetGateway(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	for _, in := range cfg.Inbounds {
		if in["type"] == "tun" {
			t.Fatal("TUN capture not removed")
		}
	}
}

func TestStartupRebuildsMultipleProviderTargetsWithoutDirectFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	old := `{"outbounds":[{"type":"direct","tag":"one"},{"type":"direct","tag":"two"}],"route":{"final":"direct","rules":[{"domain_suffix":["one.example"],"outbound":"one"},{"domain_suffix":["two.example"],"outbound":"two"}]}}`
	if err := os.WriteFile(path, []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := NewController(Config{ConfigPath: path, BinaryPath: fakeBinary(t, "exit 0")}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if err = c.AddOutbound(context.Background(), testOutbound{"one"}); err != nil {
		t.Fatal(err)
	}
	ob, ok := c.GetOutbound("two")
	if !ok {
		t.Fatal("pending provider target missing")
	}
	value, err := ob.BuildOutbound()
	if err != nil || value["type"] != "block" {
		t.Fatal("pending VPN target was not blocked")
	}
	if err = c.AddOutbound(context.Background(), testOutbound{"two"}); err != nil {
		t.Fatal(err)
	}
}

func TestReloadRestoresGatewayRouteBeforeReportingSuccess(t *testing.T) {
	c, err := NewController(Config{ConfigPath: filepath.Join(t.TempDir(), "config.json"), BinaryPath: fakeBinary(t, "if [ \"$1\" = check ]; then exit 0; fi\nexec sleep 60")}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Stop(context.Background())
	calls := 0
	c.SetReloadHook(func(_ context.Context, tun bool) error {
		if !tun {
			t.Fatal("missing gateway input")
		}
		calls++
		return nil
	})
	if err = c.SetGateway(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if err = c.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("gateway route not restored on every reload: %d", calls)
	}
}

func TestGatewayStagingExcludesConcurrentConfigChanges(t *testing.T) {
	c, err := NewController(Config{ConfigPath: filepath.Join(t.TempDir(), "config.json"), BinaryPath: fakeBinary(t, "if [ \"$1\" = check ]; then exit 0; fi\nexec sleep 60")}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Stop(context.Background())
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- c.GatewayTransaction(context.Background(), true, func() error { close(entered); <-release; return nil })
	}()
	<-entered
	if c.mu.TryLock() {
		c.mu.Unlock()
		close(release)
		<-done
		t.Fatal("configuration lock released before routes and state staged")
	}
	close(release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}
