//go:build !linux

package amneziawg

import (
	"fmt"
	"github.com/vishvananda/netlink"
)

func networkRuleAdd(r *netlink.Rule) error { return fmt.Errorf("VPN policy routing requires Linux") }
func networkRuleDel(r *netlink.Rule) error { return fmt.Errorf("VPN policy routing requires Linux") }
func networkRuleList(family int) ([]netlink.Rule, error) {
	return nil, fmt.Errorf("VPN policy routing requires Linux")
}
