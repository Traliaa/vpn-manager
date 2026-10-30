package amneziawg

import (
	"errors"
	"fmt"
	"github.com/vishvananda/netlink"
	"hash/crc32"
	"net"
	"net/netip"
	"syscall"
)

type linuxLinks struct {
	route *netlink.Route
	rule  *netlink.Rule
}

func (*linuxLinks) Exists(name string) (bool, error) {
	_, err := netlink.LinkByName(name)
	var missing netlink.LinkNotFoundError
	if errors.As(err, &missing) {
		return false, nil
	}
	return err == nil, err
}
func (*linuxLinks) Create(name string) error {
	return netlink.LinkAdd(&netlink.GenericLink{LinkAttrs: netlink.LinkAttrs{Name: name}, LinkType: "amneziawg"})
}
func (manager *linuxLinks) Remove(name string) error {
	var errs []error
	if manager.rule != nil {
		if err := networkRuleDel(manager.rule); err != nil && !errors.Is(err, syscall.ENOENT) && !errors.Is(err, syscall.ESRCH) {
			errs = append(errs, err)
		} else {
			manager.rule = nil
		}
	}
	if manager.route != nil {
		if err := netlink.RouteDel(manager.route); err != nil && !errors.Is(err, syscall.ENOENT) && !errors.Is(err, syscall.ESRCH) && !errors.Is(err, syscall.ENODEV) {
			errs = append(errs, err)
		} else {
			manager.route = nil
		}
	}
	l, err := netlink.LinkByName(name)
	var missing netlink.LinkNotFoundError
	if errors.As(err, &missing) {
		return errors.Join(errs...)
	}
	if err != nil {
		return err
	}
	return errors.Join(append(errs, netlink.LinkDel(l))...)
}
func (manager *linuxLinks) Configure(name, address string, mtu int) error {
	l, err := netlink.LinkByName(name)
	if err != nil {
		return err
	}
	if err := netlink.LinkSetMTU(l, mtu); err != nil {
		return fmt.Errorf("set VPN MTU: %w", err)
	}
	p, err := netip.ParsePrefix(address)
	if err != nil {
		return err
	}
	addr := &netlink.Addr{IPNet: &net.IPNet{IP: p.Addr().AsSlice(), Mask: net.CIDRMask(p.Bits(), p.Addr().BitLen())}}
	if err := netlink.AddrReplace(l, addr); err != nil {
		return fmt.Errorf("set VPN address: %w", err)
	}
	if err := netlink.LinkSetUp(l); err != nil {
		return err
	}
	route, rule, err := outboundRoute(name, address, l.Attrs().Index)
	if err != nil {
		return err
	}
	routes, err := netlink.RouteListFiltered(rule.Family, &netlink.Route{Table: route.Table}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return err
	}
	for _, old := range routes {
		if old.LinkIndex != route.LinkIndex || old.Protocol != 99 || old.Dst == nil || old.Dst.String() != route.Dst.String() {
			return fmt.Errorf("VPN routing table is occupied")
		}
	}
	rules, err := networkRuleList(rule.Family)
	if err != nil {
		return err
	}
	found := false
	for _, old := range rules {
		if old.Priority == rule.Priority {
			if old.Protocol != 99 || old.Table != rule.Table || old.Src == nil || old.Src.String() != rule.Src.String() {
				return fmt.Errorf("VPN routing priority is occupied")
			}
			found = true
		}
	}
	if len(routes) == 0 {
		if err := netlink.RouteAdd(route); err != nil {
			return err
		}
	}
	manager.route = route
	if !found {
		if err := networkRuleAdd(rule); err != nil {
			return err
		}
	}
	manager.rule = rule
	return nil
}

// Only sockets explicitly bound to the local VPN IP use this table; the host
// default and forwarded LAN traffic are left untouched.
func outboundRoute(name, address string, index int) (*netlink.Route, *netlink.Rule, error) {
	p, err := netip.ParsePrefix(address)
	if err != nil || index <= 0 {
		return nil, nil, fmt.Errorf("invalid outbound route")
	}
	family := syscall.AF_INET
	bits := 32
	zero := net.IPv4zero
	if p.Addr().Is6() {
		family = syscall.AF_INET6
		bits = 128
		zero = net.IPv6zero
	}
	table := 20000 + int(crc32.ChecksumIEEE([]byte(name))%10000)
	route := &netlink.Route{Table: table, LinkIndex: index, Dst: &net.IPNet{IP: zero, Mask: net.CIDRMask(0, bits)}, Protocol: 99}
	rule := netlink.NewRule()
	rule.Table = table
	rule.Priority = table
	rule.Family = family
	rule.Protocol = 99
	rule.Src = &net.IPNet{IP: p.Addr().AsSlice(), Mask: net.CIDRMask(bits, bits)}
	return route, rule, nil
}
