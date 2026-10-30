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
	base     *profileHarness
	rules    []map[string]any
	nat      string
	fail     string
	failures []string
}

func (s *scopeHarness) run(ctx context.Context, name string, args []string, input string) ([]byte, error) {
	cmd := name + " " + strings.Join(args, " ")
	if len(s.failures) > 0 && strings.Contains(cmd+" "+input, s.failures[0]) {
		s.failures = s.failures[1:]
		return nil, fmt.Errorf("injected sequenced failure")
	}
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

func TestExpandIncompleteRuleRollbackIsNotReady(t *testing.T) {
	m, h := scopedFixture(t)
	ctx := context.Background()
	if err := m.EnableProfile(ctx, ProfileConfig{ClientIP: "192.168.1.71", LANInterface: "eth0"}); err != nil {
		t.Fatal(err)
	}
	h.failures = []string{"rule add priority 18040 from 192.168.1.0/24", "rule del priority 18039"}
	if err := m.EnableProfile(ctx, routerConfig(t)); err == nil {
		t.Fatal("ignored partial rollback")
	}
	if m.Status().Ready {
		t.Fatal("incomplete rollback reported ready")
	}
}
func TestDisableFailurePreservesInfrastructureBypass(t *testing.T) {
	for _, failure := range []string{"rule del priority 18040", "rule del priority 18039"} {
		t.Run(failure, func(t *testing.T) {
			m, h := scopedFixture(t)
			ctx := context.Background()
			if err := m.EnableProfile(ctx, routerConfig(t)); err != nil {
				t.Fatal(err)
			}
			h.fail = failure
			if err := m.Disable(ctx); err == nil {
				t.Fatal("ignored failed disable")
			}
			if m.Status().Ready {
				t.Fatal("partial disable reported ready")
			}
			capture, manager, router := false, false, false
			for _, r := range h.rules {
				switch fmt.Sprint(r["priority"]) {
				case "18040":
					capture = true
				case "18038":
					manager = true
				case "18039":
					router = true
				}
			}
			if capture && (!manager || !router) {
				t.Fatal("capture retained without infrastructure bypass")
			}
		})
	}
}
func TestScopeDeletionFailureKeepsInfrastructureWhileCaptureExists(t *testing.T) {
	m, h := scopedFixture(t)
	ctx := context.Background()
	if err := m.EnableProfile(ctx, routerConfig(t)); err != nil {
		t.Fatal(err)
	}
	h.fail = "rule del priority 18040"
	if err := m.sourceRule(ctx, "del", routerConfig(t)); err == nil {
		t.Fatal("ignored removal failure")
	}
	if len(h.rules) != 3 {
		t.Fatalf("removed exclusions before capture: %v", h.rules)
	}
}

func TestSubnetRuleMatchesLinuxSeparateSourceLength(t *testing.T) {
	rule := sourceRules(routerConfig(t))[2]
	records := []string{`{"priority":18040,"src":"192.168.1.0","srclen":24,"iif":"eth0","table":51840,"protocol":"openr"}`}
	if !rulePresent(records, rule) {
		t.Fatal("existing Linux subnet rule would be re-added during recovery")
	}
}
