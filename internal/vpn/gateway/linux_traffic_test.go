//go:build linux

package gateway

import (
	"context"
	"fmt"
	"github.com/Traliaa/vpn-manager/internal/vpn/singbox"
	"go.uber.org/zap"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Helper processes run in child network namespaces of the isolated test only.
func TestLinuxTrafficHelper(t *testing.T) {
	mode := os.Getenv("VPN_MANAGER_TRAFFIC_HELPER")
	if mode == "" {
		t.Skip("helper process")
	}
	switch mode {
	case "idle":
		time.Sleep(60 * time.Second)
	case "server":
		server := &http.Server{Addr: "0.0.0.0:18080", ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "direct-ok") })}
		if err := server.ListenAndServe(); err != nil {
			t.Fatal(err)
		}
	case "client":
		dialer := &net.Dialer{Timeout: 2 * time.Second, LocalAddr: &net.TCPAddr{IP: net.ParseIP(os.Getenv("VPN_MANAGER_TEST_SOURCE"))}}
		transport := &http.Transport{DialContext: dialer.DialContext, DisableKeepAlives: true, Proxy: nil}
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
		req, err := http.NewRequest("GET", "http://203.0.113.2:18080/", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = os.Getenv("VPN_MANAGER_TEST_HOST")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil || string(body) != "direct-ok" {
			t.Fatal("unexpected response")
		}
	default:
		t.Fatal("unknown helper mode")
	}
}
func TestLinuxProfileTraffic(t *testing.T) { testLinuxTraffic(t, false) }
func TestLinuxRouterTraffic(t *testing.T)  { testLinuxTraffic(t, true) }
func testLinuxTraffic(t *testing.T, routerMode bool) {
	if os.Getenv("VPN_MANAGER_LINUX_TEST") != "1" {
		t.Skip("requires isolated Linux namespace")
	}
	current, e1 := os.Readlink("/proc/self/ns/net")
	initial, e2 := os.Readlink("/proc/1/ns/net")
	if e1 != nil || e2 != nil || current == initial {
		t.Fatal("refusing traffic test outside isolated namespace")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()
	command := func(name string, args ...string) string {
		t.Helper()
		out, err := runCommand(ctx, name, args, "")
		if err != nil {
			t.Fatalf("%s %v: %v", name, args, err)
		}
		return string(out)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spawn := func(mode string) *exec.Cmd {
		t.Helper()
		cmd := exec.CommandContext(ctx, "unshare", "-n", executable, "-test.run", "^TestLinuxTrafficHelper$")
		cmd.Env = append(os.Environ(), "VPN_MANAGER_TRAFFIC_HELPER="+mode)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
		deadline := time.Now().Add(time.Second)
		for {
			ns, e := os.Readlink(fmt.Sprintf("/proc/%d/ns/net", cmd.Process.Pid))
			if e == nil && ns != current {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("child namespace not ready")
			}
			time.Sleep(10 * time.Millisecond)
		}
		return cmd
	}
	clientNS := spawn("idle")
	wanNS := spawn("server")
	command("ip", "link", "set", "lo", "up")
	command("ip", "link", "add", "eth0", "type", "bridge")
	command("ip", "addr", "add", "192.168.1.40/24", "dev", "eth0")
	command("ip", "link", "set", "eth0", "up")
	attach := func(parent, peer, addr string, pid int) {
		t.Helper()
		id := strconv.Itoa(pid)
		command("ip", "link", "add", parent, "type", "veth", "peer", "name", peer)
		command("ip", "link", "set", parent, "master", "eth0")
		command("ip", "link", "set", parent, "up")
		command("ip", "link", "set", peer, "netns", id)
		command("nsenter", "-t", id, "-n", "ip", "link", "set", "lo", "up")
		command("nsenter", "-t", id, "-n", "ip", "addr", "add", addr, "dev", peer)
		command("nsenter", "-t", id, "-n", "ip", "link", "set", peer, "up")
	}
	attach("client-side", "client0", "192.168.1.71/24", clientNS.Process.Pid)
	attach("wan-side", "wan0", "192.168.1.1/24", wanNS.Process.Pid)
	command("nsenter", "-t", strconv.Itoa(clientNS.Process.Pid), "-n", "ip", "addr", "add", "192.168.1.72/32", "dev", "client0")
	command("nsenter", "-t", strconv.Itoa(clientNS.Process.Pid), "-n", "ip", "route", "add", "default", "via", "192.168.1.40")
	command("nsenter", "-t", strconv.Itoa(wanNS.Process.Pid), "-n", "ip", "addr", "add", "203.0.113.2/32", "dev", "lo")
	command("ip", "route", "add", "default", "via", "192.168.1.1", "dev", "eth0")
	command("sysctl", "-w", "net.ipv4.ip_forward=1")
	binary := os.Getenv("SINGBOX_TEST_BINARY")
	if binary == "" {
		t.Fatal("SINGBOX_TEST_BINARY required")
	}
	ctrl, err := singbox.NewController(singbox.Config{ConfigPath: filepath.Join(t.TempDir(), "config.json"), BinaryPath: binary}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer ctrl.Stop(context.Background())
	if err = ctrl.SetOutbounds(ctx, []singbox.Outbound{singbox.BlockedOutbound("vpn-test")}); err != nil {
		t.Fatal(err)
	}
	if err = ctrl.GenerateConfigWithRouting(ctx, ctrl.ListOutbounds(), singbox.RouteConfig{Final: "direct", AutoDetectInterface: true, Rules: []singbox.RouteRule{{DomainSuffix: []string{"blocked.test"}, OutboundTag: "vpn-test"}}}); err != nil {
		t.Fatal(err)
	}
	m := NewManager(zap.NewNop())
	m.profileCtrl = ctrl
	m.SetProfileReadiness(func() bool { return true })
	m.statePath = filepath.Join(t.TempDir(), "gateway.json")
	if err = m.EnableProfile(ctx, ProfileConfig{ClientIP: "192.168.1.71", LANInterface: "eth0"}); err != nil {
		t.Fatal(err)
	}
	request := func(source, host string, wantSuccess bool) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "nsenter", "-t", strconv.Itoa(clientNS.Process.Pid), "-n", executable, "-test.run", "^TestLinuxTrafficHelper$")
		cmd.Env = append(os.Environ(), "VPN_MANAGER_TRAFFIC_HELPER=client", "VPN_MANAGER_TEST_SOURCE="+source, "VPN_MANAGER_TEST_HOST="+host)
		out, e := cmd.CombinedOutput()
		if (e == nil) != wantSuccess {
			t.Fatalf("source %s host %s success=%v: %s", source, host, e == nil, strings.TrimSpace(string(out)))
		}
	}
	request("192.168.1.71", "allowed.test", true)
	request("192.168.1.71", "blocked.test", false)
	request("192.168.1.72", "blocked.test", true)
	if routerMode {
		if err = m.EnableProfile(ctx, ProfileConfig{ClientSubnet: "192.168.1.0/24", RouterIP: "192.168.1.1", ManagerIP: "192.168.1.40", LANInterface: "eth0"}); err != nil {
			t.Fatal(err)
		}
		request("192.168.1.71", "blocked.test", false)
		request("192.168.1.72", "allowed.test", true)
		request("192.168.1.72", "blocked.test", false)
		for _, source := range []string{"192.168.1.1", "192.168.1.40"} {
			route := command("ip", "-4", "route", "get", "203.0.113.2", "from", source, "iif", "eth0")
			if strings.Contains(route, "vmtun0") {
				t.Fatalf("infrastructure source captured: %s", route)
			}
		}
		route := command("ip", "-4", "route", "get", "192.168.1.200", "from", "192.168.1.72", "iif", "eth0")
		if strings.Contains(route, "vmtun0") {
			t.Fatal("local LAN captured")
		}
		// A router-unselected client sends straight to the upstream router, bypassing
		// this gateway even though its address is inside the accepted LAN scope.
		command("nsenter", "-t", strconv.Itoa(clientNS.Process.Pid), "-n", "ip", "route", "add", "203.0.113.2/32", "via", "192.168.1.1")
		request("192.168.1.72", "blocked.test", true)
		command("nsenter", "-t", strconv.Itoa(clientNS.Process.Pid), "-n", "ip", "route", "del", "203.0.113.2/32")
	}
	if err = ctrl.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	request("192.168.1.71", "allowed.test", true)
	request("192.168.1.71", "blocked.test", false)
	if routerMode {
		request("192.168.1.72", "blocked.test", false)
	}
	if err = ctrl.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	request("192.168.1.71", "allowed.test", false)
	if routerMode {
		request("192.168.1.72", "allowed.test", false)
	}
	if err = m.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	request("192.168.1.71", "allowed.test", true)
	if routerMode {
		// Reconstruct from persisted state as a fresh manager while retaining the
		// same owned proxy, like service startup after profile reconstruction.
		restored := NewManager(zap.NewNop())
		restored.profileCtrl = ctrl
		restored.SetProfileReadiness(func() bool { return true })
		restored.statePath = m.statePath
		if err = restored.Restore(ctx); err != nil {
			t.Fatal(err)
		}
		m = restored
		request("192.168.1.72", "allowed.test", true)
		request("192.168.1.72", "blocked.test", false)
	}
	if err = m.Disable(ctx); err != nil {
		t.Fatal(err)
	}
	request("192.168.1.71", "blocked.test", true)
	if routerMode {
		request("192.168.1.72", "blocked.test", true)
	}
	if err = m.Release(ctx); err != nil {
		t.Fatal(err)
	}
}
