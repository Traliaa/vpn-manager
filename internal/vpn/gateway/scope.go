package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
)

// The two preceding priorities are reserved for infrastructure bypass. They
// never affect locally generated packets: all rules require the LAN iif.
func ownedPriority(value any) bool {
	s := fmt.Sprint(value)
	return s == "18038" || s == "18039" || s == profilePriority
}
func (cfg ProfileConfig) sourcePrefix() string {
	if cfg.ClientSubnet != "" {
		return cfg.ClientSubnet
	}
	return cfg.ClientIP + "/32"
}
func (m *Manager) validateRouterScope(cfg ProfileConfig) error {
	p, err := netip.ParsePrefix(cfg.ClientSubnet)
	if err != nil || !p.Addr().Is4() || !p.Addr().IsPrivate() || p != p.Masked() || p.Bits() < 16 || p.Bits() > 30 || cfg.ClientIP != "" {
		return fmt.Errorf("router mode requires one canonical private connected IPv4 subnet (/16 through /30), without client_ip")
	}
	router, e1 := netip.ParseAddr(cfg.RouterIP)
	manager, e2 := netip.ParseAddr(cfg.ManagerIP)
	host := func(a netip.Addr) bool {
		if !a.IsValid() || !p.Contains(a) || a == p.Addr() {
			return false
		}
		last := p.Addr().As4()
		bits := uint32(last[0])<<24 | uint32(last[1])<<16 | uint32(last[2])<<8 | uint32(last[3])
		bits |= (1 << uint(32-p.Bits())) - 1
		x := a.As4()
		return uint32(x[0])<<24|uint32(x[1])<<16|uint32(x[2])<<8|uint32(x[3]) != bits
	}
	if e1 != nil || e2 != nil || !host(router) || !host(manager) || router == manager {
		return fmt.Errorf("router and manager must be distinct host addresses within LAN subnet")
	}
	if !interfacePattern.MatchString(cfg.LANInterface) {
		return fmt.Errorf("invalid LAN interface")
	}
	if m.profileCtrl == nil || m.statePath == "" {
		return fmt.Errorf("transparent gateway unavailable")
	}
	// Read the real interface, rather than trusting user-supplied scope. Refuse
	// aliases in this subnet because each own source needs an explicit bypass.
	raw, err := m.run(context.Background(), "ip", []string{"-j", "-4", "address", "show", "dev", cfg.LANInterface}, "")
	if err != nil {
		return err
	}
	var links []struct {
		Addresses []struct {
			Local string `json:"local"`
			Bits  int    `json:"prefixlen"`
		} `json:"addr_info"`
	}
	if err = json.Unmarshal(raw, &links); err != nil {
		return err
	}
	matched := false
	for _, link := range links {
		for _, address := range link.Addresses {
			a, e := netip.ParseAddr(address.Local)
			if e != nil {
				continue
			}
			if p.Contains(a) {
				if a != manager {
					return fmt.Errorf("LAN subnet contains another manager address")
				}
				if netip.PrefixFrom(a, address.Bits).Masked() == p {
					matched = true
				}
			}
		}
	}
	if !matched {
		return fmt.Errorf("LAN subnet and manager address must match the selected interface")
	}
	return nil
}
func sourceRules(cfg ProfileConfig) [][]string {
	rules := [][]string{}
	if cfg.ClientSubnet != "" {
		rules = append(rules, []string{"priority", "18038", "from", cfg.ManagerIP + "/32", "iif", cfg.LANInterface, "table", "main", "protocol", "99"}, []string{"priority", "18039", "from", cfg.RouterIP + "/32", "iif", cfg.LANInterface, "table", "main", "protocol", "99"})
	}
	return append(rules, []string{"priority", profilePriority, "from", cfg.sourcePrefix(), "iif", cfg.LANInterface, "table", profileTable, "protocol", "99"})
}
func rulePresent(records []string, args []string) bool {
	for _, record := range records {
		var value map[string]any
		if json.Unmarshal([]byte(record), &value) != nil {
			continue
		}
		table := fmt.Sprint(value["table"])
		if table == "254" {
			table = "main"
		}
		src := fmt.Sprint(value["src"])
		if !containsSlash(src) {
			src += "/32"
		}
		if fmt.Sprint(value["priority"]) == args[1] && src == args[3] && value["iif"] == args[5] && table == args[7] {
			return true
		}
	}
	return false
}
func containsSlash(s string) bool {
	for _, r := range s {
		if r == '/' {
			return true
		}
	}
	return false
}
func (m *Manager) reconcileSourceRules(ctx context.Context, action string, cfg ProfileConfig, records []string) error {
	for _, rule := range sourceRules(cfg) {
		present := rulePresent(records, rule)
		if (action == "add" && !present) || (action == "del" && present) {
			if err := m.command(ctx, "ip", append([]string{"-4", "rule", action}, rule...)...); err != nil {
				return err
			}
		}
	}
	return nil
}
func profileNATBatch(cfg ProfileConfig) string {
	scope := "ip saddr " + cfg.sourcePrefix()
	destination := "ip daddr " + cfg.sourcePrefix()
	if cfg.ClientSubnet != "" {
		scope += fmt.Sprintf(" ip saddr != { %s, %s }", cfg.RouterIP, cfg.ManagerIP) + " ip daddr != { 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16 }"
		destination += fmt.Sprintf(" ip daddr != { %s, %s }", cfg.RouterIP, cfg.ManagerIP)
	}
	return fmt.Sprintf("add table inet %s { comment %q; }\nadd chain inet %s postrouting { type nat hook postrouting priority 100; }\nadd rule inet %s postrouting iifname %q oifname %q %s masquerade\nadd chain inet %s output { type filter hook output priority 0; }\nadd rule inet %s output oifname %q %s icmp type redirect drop\n", profileNFT, profileOwner, profileNFT, profileNFT, cfg.LANInterface, cfg.LANInterface, scope, profileNFT, profileNFT, cfg.LANInterface, destination)
}

func subnetContains(raw, host string) bool {
	p, e := netip.ParsePrefix(raw)
	a, e2 := netip.ParseAddr(host)
	return e == nil && e2 == nil && p.Contains(a)
}
