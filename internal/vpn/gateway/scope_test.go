package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func routerConfig(t *testing.T) ProfileConfig {
	t.Helper()
	var cfg ProfileConfig
	if err := json.Unmarshal([]byte(`{"client_subnet":"192.168.1.0/24","router_ip":"192.168.1.1","manager_ip":"192.168.1.40","lan_interface":"eth0"}`), &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}
func withLAN(m *Manager, h *profileHarness) {
	m.run = func(ctx context.Context, name string, args []string, input string) ([]byte, error) {
		if name == "ip" && strings.Join(args, " ") == "-j -4 address show dev eth0" {
			return []byte(`[{"addr_info":[{"family":"inet","local":"192.168.1.40","prefixlen":24}]}]`), nil
		}
		return h.run(ctx, name, args, input)
	}
}
func TestRouterScopeCapturesSubnetAndExcludesInfrastructure(t *testing.T) {
	m, _, h := gatewayFixture(t)
	withLAN(m, h)
	if err := m.EnableProfile(context.Background(), routerConfig(t)); err != nil {
		t.Fatal(err)
	}
	commands := strings.Join(h.commands, "\n")
	for _, want := range []string{"from 192.168.1.0/24 iif eth0 table 51840", "from 192.168.1.1/32 iif eth0 table main", "from 192.168.1.40/32 iif eth0 table main", "ip saddr 192.168.1.0/24", "ip saddr != { 192.168.1.1, 192.168.1.40 }"} {
		if !strings.Contains(commands, want) {
			t.Errorf("missing %q", want)
		}
	}
}
func TestRouterScopeRejectsInvalidOrUnconnectedLAN(t *testing.T) {
	for _, raw := range []string{
		`{"client_subnet":"0.0.0.0/0","router_ip":"192.168.1.1","manager_ip":"192.168.1.40","lan_interface":"eth0"}`,
		`{"client_subnet":"192.168.1.7/24","router_ip":"192.168.1.1","manager_ip":"192.168.1.40","lan_interface":"eth0"}`,
		`{"client_subnet":"192.168.2.0/24","router_ip":"192.168.2.1","manager_ip":"192.168.2.40","lan_interface":"eth0"}`,
		`{"client_subnet":"192.168.1.0/24","router_ip":"192.168.1.40","manager_ip":"192.168.1.40","lan_interface":"eth0"}`,
		`{"client_subnet":"192.168.1.0/24","client_ip":"192.168.1.71","router_ip":"192.168.1.1","manager_ip":"192.168.1.40","lan_interface":"eth0"}`,
	} {
		m, p, h := gatewayFixture(t)
		withLAN(m, h)
		var cfg ProfileConfig
		json.Unmarshal([]byte(raw), &cfg)
		if err := m.EnableProfile(context.Background(), cfg); err == nil {
			t.Errorf("accepted %s", raw)
		}
		if p.enabled || h.nat {
			t.Fatal("invalid scope mutated runtime")
		}
	}
}

type scopeHarness struct {
	base  *profileHarness
	rules []map[string]any
	nat   string
	fail  string
}

func (s *scopeHarness) run(ctx context.Context, name string, args []string, input string) ([]byte, error) {
	cmd := name + " " + strings.Join(args, " ")
	if s.fail != "" && strings.Contains(cmd+" "+input, s.fail) {
		s.fail = ""
		return nil, fmt.Errorf("injected failure")
	}
	switch {
	case cmd == "ip -j -4 address show dev eth0":
		return []byte(`[{"addr_info":[{"local":"192.168.1.40","prefixlen":24}]}]`), nil
	case cmd == "ip -j -4 rule show":
		return json.Marshal(s.rules)
	case strings.HasPrefix(cmd, "ip -4 rule "):
		rule := map[string]any{"priority": args[4], "src": args[6], "iif": args[8], "table": args[10], "protocol": "99"}
		if args[2] == "add" {
			s.rules = append(s.rules, rule)
		} else {
			for i, r := range s.rules {
				if fmt.Sprint(r) == fmt.Sprint(rule) {
					s.rules = append(s.rules[:i], s.rules[i+1:]...)
					break
				}
			}
		}
		return nil, nil
	case strings.HasPrefix(cmd, "nft -j list table"):
		if s.nat == "" {
			return nil, errObjectMissing
		}
		return json.Marshal(map[string]any{"nftables": []any{map[string]any{"table": map[string]any{"comment": profileOwner, "rules": s.nat}}}})
	case name == "nft" && input != "":
		s.nat = input[strings.Index(input, "add table"):]
		return nil, nil
	case strings.HasPrefix(cmd, "nft delete"):
		s.nat = ""
		return nil, nil
	}
	return s.base.run(ctx, name, args, input)
}
func scopedFixture(t *testing.T) (*Manager, *scopeHarness) {
	m, _, h := gatewayFixture(t)
	s := &scopeHarness{base: h}
	m.run = s.run
	return m, s
}
func TestExpandPilotScopeAndRestoreMissingExclusions(t *testing.T) {
	ctx := context.Background()
	m, h := scopedFixture(t)
	if err := m.EnableProfile(ctx, ProfileConfig{ClientIP: "192.168.1.71", LANInterface: "eth0"}); err != nil {
		t.Fatal(err)
	}
	if err := m.EnableProfile(ctx, routerConfig(t)); err != nil {
		t.Fatal(err)
	}
	if len(h.rules) != 3 {
		t.Fatalf("wrong rules: %v", h.rules)
	}
	if m.profile.ClientIP != "" {
		t.Fatal("legacy scope retained")
	}
	// Simulate one missing infrastructure rule after a partial restart.
	h.rules = h.rules[1:]
	if err := m.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	if len(h.rules) != 3 {
		t.Fatal("missing exclusion not restored")
	}
	if err := m.Disable(ctx); err != nil {
		t.Fatal(err)
	}
	if len(h.rules) != 0 || h.nat == "" {
		t.Fatal("direct fallback is broken")
	}
}
func TestExpandScopeFailureRestoresPilot(t *testing.T) {
	for _, failure := range []string{"from 192.168.1.0/24", "delete table inet vpn_manager_profile"} {
		t.Run(failure, func(t *testing.T) {
			ctx := context.Background()
			m, h := scopedFixture(t)
			old := ProfileConfig{ClientIP: "192.168.1.71", LANInterface: "eth0"}
			if err := m.EnableProfile(ctx, old); err != nil {
				t.Fatal(err)
			}
			previous, err := os.ReadFile(m.statePath)
			if err != nil {
				t.Fatal(err)
			}
			nat := h.nat
			h.fail = failure
			if err := m.EnableProfile(ctx, routerConfig(t)); err == nil {
				t.Fatal("ignored transition failure")
			}
			after, _ := os.ReadFile(m.statePath)
			if string(after) != string(previous) || h.nat != nat || len(h.rules) != 1 || !m.Status().Ready || *m.profile != old {
				t.Fatalf("pilot not restored: rules=%v status=%#v", h.rules, m.Status())
			}
		})
	}
}
