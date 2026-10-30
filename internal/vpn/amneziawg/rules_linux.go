//go:build linux

package amneziawg

import "github.com/vishvananda/netlink"

func networkRuleAdd(r *netlink.Rule) error               { return netlink.RuleAdd(r) }
func networkRuleDel(r *netlink.Rule) error               { return netlink.RuleDel(r) }
func networkRuleList(family int) ([]netlink.Rule, error) { return netlink.RuleList(family) }
