package singbox

import (
	"fmt"
	"net/netip"
)

// InterfaceOutbound terminates connections in sing-box and dials through a VPN
// interface. Binding the source makes the dedicated source routing rule apply.
type InterfaceOutbound struct{ Name, Interface, Address string }

func (o InterfaceOutbound) Tag() string { return o.Name }
func (o InterfaceOutbound) BuildOutbound() (map[string]any, error) {
	p, err := netip.ParsePrefix(o.Address)
	if err != nil || o.Interface == "" || o.Name == "" {
		return nil, fmt.Errorf("invalid VPN interface outbound")
	}
	field := "inet4_bind_address"
	if p.Addr().Is6() {
		field = "inet6_bind_address"
	}
	return map[string]any{"type": "direct", "tag": o.Name, "bind_interface": o.Interface, field: p.Addr().String()}, nil
}

// Keep disabled provider tags as explicit block targets for existing rules.
type BlockedOutbound string

func (o BlockedOutbound) Tag() string { return string(o) }
func (o BlockedOutbound) BuildOutbound() (map[string]any, error) {
	return map[string]any{"type": "block", "tag": string(o)}, nil
}
