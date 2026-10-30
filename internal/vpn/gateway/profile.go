package gateway

import (
	"context"

	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const profileTable = "51840"
const profilePriority = "18040"
const profileNFT = "vpn_manager_profile"
const profileOwner = "vpn-manager profile gateway"

var errObjectMissing = errors.New("network object missing")
var interfacePattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,15}$`)

type ProfileConfig struct {
	ClientIP     string `json:"client_ip,omitempty"`
	ClientSubnet string `json:"client_subnet,omitempty"`
	RouterIP     string `json:"router_ip,omitempty"`
	ManagerIP    string `json:"manager_ip,omitempty"`
	LANInterface string `json:"lan_interface"`
}
type profileController interface {
	SetGateway(context.Context, bool) error
	IsRunning(context.Context) (bool, error)
}
type commandRunner func(context.Context, string, []string, string) ([]byte, error)
type profileState struct {
	Enabled bool          `json:"enabled"`
	Config  ProfileConfig `json:"config"`
	Routes  []string      `json:"routes"`
	Rules   []string      `json:"rules"`
	NAT     string        `json:"nat"`
}

func runCommand(ctx context.Context, name string, args []string, input string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(input)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.CombinedOutput()
	if err != nil && name == "nft" && strings.Contains(string(out), "No such file or directory") {
		return nil, errObjectMissing
	}
	if err != nil {
		return nil, fmt.Errorf("%s failed: %w", name, err)
	}
	return out, nil
}
func interfaceAddresses() ([]netip.Addr, error) {
	values, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	var out []netip.Addr
	for _, v := range values {
		p, e := netip.ParsePrefix(v.String())
		if e == nil {
			out = append(out, p.Addr())
		}
	}
	return out, nil
}
func (m *Manager) validateProfile(cfg ProfileConfig) error {
	if cfg.ClientSubnet != "" {
		return m.validateRouterScope(cfg)
	}
	if cfg.RouterIP != "" || cfg.ManagerIP != "" {
		return fmt.Errorf("router fields require client_subnet")
	}
	ip, err := netip.ParseAddr(cfg.ClientIP)
	if err != nil || !ip.Is4() || !ip.IsPrivate() || !ip.IsGlobalUnicast() {
		return fmt.Errorf("client must be one private IPv4 address")
	}
	if cfg.ClientIP != "192.168.1.71" {
		return fmt.Errorf("pilot gateway is restricted to 192.168.1.71")
	}
	if !interfacePattern.MatchString(cfg.LANInterface) {
		return fmt.Errorf("invalid LAN interface")
	}
	own, err := m.localAddresses()
	if err != nil {
		return err
	}
	for _, a := range own {
		if a.Unmap() == ip {
			return fmt.Errorf("client cannot be manager address")
		}
	}
	if m.profileCtrl == nil {
		return fmt.Errorf("transparent proxy unavailable")
	}
	if m.statePath == "" {
		return fmt.Errorf("gateway state path unavailable")
	}
	return nil
}
func (m *Manager) command(ctx context.Context, name string, args ...string) error {
	_, err := m.run(ctx, name, args, "")
	return err
}

// preflight refuses occupied policy space. Crash recovery must explicitly
// reconcile persisted ownership before reusing any existing network objects.
func (m *Manager) profilePreflight(ctx context.Context) error {
	raw, err := m.run(ctx, "ip", []string{"-j", "-4", "route", "show", "table", "all"}, "")
	if err != nil {
		return err
	}
	var routes []map[string]any
	if err = json.Unmarshal(raw, &routes); err != nil {
		return err
	}
	for _, r := range routes {
		if fmt.Sprint(r["table"]) == profileTable {
			return fmt.Errorf("gateway routing table is occupied")
		}
	}
	raw, err = m.run(ctx, "ip", []string{"-j", "-4", "rule", "show"}, "")
	if err != nil {
		return err
	}
	var rules []map[string]any
	if err = json.Unmarshal(raw, &rules); err != nil {
		return err
	}
	for _, r := range rules {
		if ownedPriority(r["priority"]) || fmt.Sprint(r["table"]) == profileTable {
			return fmt.Errorf("gateway routing rule is occupied")
		}
	}
	return nil
}
func (m *Manager) saveProfile(ctx context.Context, enabled bool, cfg ProfileConfig) error {
	state, err := m.networkSnapshot(ctx)
	if err != nil {
		return err
	}
	state.Enabled = enabled
	state.Config = cfg
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(m.statePath), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(m.statePath), ".gateway-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), m.statePath)
}
func (m *Manager) ensureProfileNAT(ctx context.Context, cfg ProfileConfig) (bool, error) {
	raw, err := m.run(ctx, "nft", []string{"-j", "list", "table", "inet", profileNFT}, "")
	if err == nil {
		// A matching name/comment alone cannot establish the scoped rule contents.
		// Existing tables are accepted only during this manager's current lifetime.
		if m.profile == nil || *m.profile != cfg || !strings.Contains(string(raw), profileOwner) {
			return false, fmt.Errorf("gateway NAT table already exists; explicit reconciliation required")
		}
		return false, nil
	}
	if !errors.Is(err, errObjectMissing) {
		return false, err
	}
	batch := profileNATBatch(cfg)
	_, err = m.run(ctx, "nft", []string{"-f", "-"}, batch)
	return err == nil, err
}
func (m *Manager) sourceRule(ctx context.Context, action string, cfg ProfileConfig) error {
	var installed [][]string
	for _, rule := range orderedSourceRules(cfg, action) {
		if err := m.command(ctx, "ip", append([]string{"-4", "rule", action}, rule...)...); err != nil {
			if action == "add" {
				recovery, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				var cleanup []error
				for i := len(installed) - 1; i >= 0; i-- {
					cleanup = append(cleanup, m.command(recovery, "ip", append([]string{"-4", "rule", "del"}, installed[i]...)...))
				}
				if cleanupErr := errors.Join(cleanup...); cleanupErr != nil {
					m.ready = false
					return errors.Join(err, fmt.Errorf("source rule rollback incomplete: %w", cleanupErr))
				}
				return err
			}
			m.ready = false
			return err
		}
		installed = append(installed, rule)
	}
	return nil
}

func profileRoutes() [][]string {
	return [][]string{
		{"blackhole", "default", "metric", "32767"},
		{"throw", "10.0.0.0/8"}, {"throw", "172.16.0.0/12"}, {"throw", "192.168.0.0/16"},
		{"default", "dev", "vmtun0", "metric", "10"},
	}
}
func (m *Manager) route(ctx context.Context, action string, route []string) error {
	args := []string{"-4", "route", action, "table", profileTable, "proto", "99"}
	args = append(args, route...)
	return m.command(ctx, "ip", args...)
}

// EnableProfile installs interception for one validated source only. No main
// routing entries are changed. Routing is installed after proxy readiness.
func (m *Manager) EnableProfile(ctx context.Context, cfg ProfileConfig) (result error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	defer func() {
		if result != nil {
			m.lastError = result.Error()
		} else {
			m.lastError = ""
		}
	}()
	if err := m.validateProfile(cfg); err != nil {
		return err
	}
	if m.profileReady == nil || !m.profileReady() {
		return fmt.Errorf("active routing profile is not ready")
	}
	m.configureReloadHook()
	if m.enabled {
		if m.profile != nil && m.profile.ClientIP != "" && cfg.ClientSubnet != "" {
			return m.expandScopeLocked(ctx, cfg)
		}
		if m.profile != nil && *m.profile == cfg {
			running, err := m.profileCtrl.IsRunning(ctx)
			if err != nil {
				return err
			}
			if running && m.ready {
				return nil
			}
			return m.restoreLocked(ctx)
		}
		return fmt.Errorf("another gateway mode is active")
	}
	if m.profile != nil && *m.profile != cfg {
		return fmt.Errorf("release previous client before changing scope")
	}
	if err := m.profilePreflight(ctx); err != nil {
		return err
	}
	raw, err := m.run(ctx, "sysctl", []string{"-n", "net.ipv4.ip_forward"}, "")
	if err != nil {
		return err
	}
	// Changing global forwarding could alter unrelated networks. Require existing
	// forwarding rather than silently changing the host's global behaviour.
	if strings.TrimSpace(string(raw)) != "1" {
		return fmt.Errorf("IPv4 forwarding must be enabled by administrator")
	}
	natCreated, err := m.ensureProfileNAT(ctx, cfg)
	if err != nil {
		return err
	}
	var installed [][]string
	ruleInstalled := false
	proxyAttempted := false
	rollback := func(cause error) error {
		recovery, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var cleanup []error
		if ruleInstalled {
			cleanup = append(cleanup, m.sourceRule(recovery, "del", cfg))
		}
		for i := len(installed) - 1; i >= 0; i-- {
			cleanup = append(cleanup, m.route(recovery, "del", installed[i]))
		}
		if proxyAttempted {
			cleanup = append(cleanup, m.profileCtrl.SetGateway(recovery, false))
		}
		if natCreated {
			cleanup = append(cleanup, m.command(recovery, "nft", "delete", "table", "inet", profileNFT))
		}
		return errors.Join(append([]error{cause}, cleanup...)...)
	}
	stage := func() error {
		running, err := m.profileCtrl.IsRunning(ctx)
		if err != nil {
			return err
		}
		if !running {
			return fmt.Errorf("transparent proxy is not running")
		}
		for _, r := range profileRoutes() {
			if err = m.route(ctx, "add", r); err != nil {
				return err
			}
			installed = append(installed, r)
		}
		if err = m.sourceRule(ctx, "add", cfg); err != nil {
			return err
		}
		ruleInstalled = true
		if err = m.saveProfile(ctx, true, cfg); err != nil {
			return err
		}
		m.profile = &cfg
		m.enabled = true
		m.ready = true
		m.ifaceName = "vmtun0"
		return nil
	}
	proxyAttempted = true
	if err = m.gatewayTransaction(ctx, true, stage); err != nil {
		return rollback(err)
	}
	return nil
}
func (m *Manager) disableProfileLocked(ctx context.Context) error {
	cfg := *m.profile
	raw, err := os.ReadFile(m.statePath)
	if err != nil {
		return err
	}
	var saved profileState
	if err = json.Unmarshal(raw, &saved); err != nil {
		return err
	}
	current, err := m.networkSnapshot(ctx)
	if err != nil {
		return err
	}
	if !contained(current.Routes, saved.Routes) || !contained(current.Rules, saved.Rules) || current.NAT != saved.NAT {
		return fmt.Errorf("gateway network objects changed; refusing cleanup")
	}
	// Retrying a partially completed disable removes only objects still present.
	m.ready = false
	if len(current.Rules) > 0 {
		if err = m.reconcileSourceRules(ctx, "del", cfg, current.Rules); err != nil {
			return err
		}
	}
	m.ready = false
	routes := profileRoutes()
	for i := len(routes) - 1; i >= 0; i-- {
		if routePresent(current.Routes, routes[i]) {
			if err = m.route(ctx, "del", routes[i]); err != nil {
				return err
			}
		}
	}
	if err = m.profileCtrl.SetGateway(ctx, false); err != nil {
		return err
	}
	if err = m.saveProfile(ctx, false, cfg); err != nil {
		return err
	}
	m.enabled = false
	m.ready = true
	m.ifaceName = cfg.LANInterface
	m.lastError = ""
	return nil
}

// Release removes the direct fallback after the router no longer uses this
// server as the client's gateway. Callers must obtain that confirmation first.
func (m *Manager) Release(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.profile == nil {
		return nil
	}
	if m.enabled {
		return fmt.Errorf("disable profile gateway before release")
	}
	raw, err := os.ReadFile(m.statePath)
	if err != nil {
		return err
	}
	var saved profileState
	if err = json.Unmarshal(raw, &saved); err != nil {
		return err
	}
	current, err := m.networkSnapshot(ctx)
	if err != nil {
		return err
	}
	if len(current.Routes) > 0 || len(current.Rules) > 0 {
		return fmt.Errorf("gateway routing still exists")
	}
	if current.NAT != "" {
		if current.NAT != saved.NAT {
			return fmt.Errorf("gateway NAT changed; refusing removal")
		}
		if err = m.command(ctx, "nft", "delete", "table", "inet", profileNFT); err != nil {
			return err
		}
	}
	if err = os.Remove(m.statePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	m.profile = nil
	m.enabled = false
	m.ready = false
	m.ifaceName = ""
	return nil
}
