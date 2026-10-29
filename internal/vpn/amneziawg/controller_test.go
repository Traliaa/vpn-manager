package amneziawg

import (
	"context"
	"go.uber.org/zap"
	"testing"
)

func validTestConfig() Config {
	return Config{PrivateKey: "AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=", Address: "10.0.0.2/32", Peer: PeerConfig{PublicKey: "AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI=", Endpoint: "192.0.2.1:51820", AllowedIPs: []string{"0.0.0.0/0"}}}
}

// Invalid configs must be rejected before Connect can change the host network.
func TestApplyConfigRejectsMalformed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Config)
	}{
		{"private key", func(c *Config) { c.PrivateKey = "bad" }},
		{"peer key", func(c *Config) { c.Peer.PublicKey = "bad" }},
		{"address", func(c *Config) { c.Address = "not-a-prefix" }},
		{"endpoint", func(c *Config) { c.Peer.Endpoint = "server-without-port" }},
		{"port", func(c *Config) { c.Peer.Endpoint = "192.0.2.1:99999" }},
		{"allowed IP", func(c *Config) { c.Peer.AllowedIPs = []string{"bad"} }},
		{"empty allowed IPs", func(c *Config) { c.Peer.AllowedIPs = nil }},
		{"negative keepalive", func(c *Config) { c.Peer.PersistentKeepalive = -1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewController("test", Config{}, zap.NewNop())
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			cfg := validTestConfig()
			tc.mutate(&cfg)
			if err := c.ApplyConfig(context.Background(), cfg); err == nil {
				t.Fatal("accepted invalid config")
			}
		})
	}
}

func TestApplyConfigRejectsTypedNil(t *testing.T) {
	c, err := NewController("test", validTestConfig(), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var cfg *Config
	if err := c.ApplyConfig(context.Background(), cfg); err == nil {
		t.Fatal("accepted typed nil")
	}
}

func TestLongProviderNamesHaveDistinctInterfaces(t *testing.T) {
	a := sanitizeIfaceName("Hungary Budapest provider one", "awg")
	b := sanitizeIfaceName("Hungary Budapest provider two", "awg")
	if a == b {
		t.Fatal("distinct provider names collide and can delete another provider's interface")
	}
	if len(a) > 15 || len(b) > 15 {
		t.Fatal("Linux interface name limit exceeded")
	}
}

func TestRejectedConfigDoesNotReplaceLastValidConfig(t *testing.T) {
	c, err := NewController("test", validTestConfig(), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	bad := validTestConfig()
	bad.Address = "invalid"
	if err := c.ApplyConfig(context.Background(), bad); err == nil {
		t.Fatal("accepted invalid address")
	}
	if c.cfg.Address != "10.0.0.2/32" {
		t.Fatal("rejected update changed previous configuration")
	}
}

func TestOutboundRuleUsesOnlyLocalVPNAddress(t *testing.T) {
	route, rule, err := outboundRoute("awg_test", "10.0.0.2/24", 7)
	if err != nil {
		t.Fatal(err)
	}
	if rule.Src.String() != "10.0.0.2/32" {
		t.Fatalf("rule captures subnet rather than VPN socket: %s", rule.Src)
	}
	if route.Table == 254 || route.Table == 0 || rule.Table != route.Table || rule.Priority >= 32766 {
		t.Fatal("outbound table/priority would replace host defaults")
	}
	if route.LinkIndex != 7 || route.Dst.String() != "0.0.0.0/0" {
		t.Fatal("wrong VPN route")
	}
}
