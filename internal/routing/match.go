package routing

import (
	"fmt"
	"github.com/Traliaa/vpn-manager/internal/db"
	"github.com/Traliaa/vpn-manager/internal/vpn/singbox"
	"net/netip"
	"strings"
)

// matchRule keeps domain predicates independent of resolver cache entries.
// Combining domain and IP fields in one sing-box rule would require both.
func matchRule(kind db.RuleType, value string) (singbox.RouteRule, error) {
	r := singbox.RouteRule{}
	value = strings.TrimSpace(value)
	switch kind {
	case db.RuleTypeDomain, db.RuleTypeDomainSuffix, db.RuleTypeDomainKeyword:
		if value == "" || strings.ContainsAny(value, "/:?# \t\r\n") {
			return r, fmt.Errorf("domain rule requires a hostname or keyword, without URL syntax")
		}
		switch kind {
		case db.RuleTypeDomain:
			r.Domain = []string{value}
		case db.RuleTypeDomainSuffix:
			r.DomainSuffix = []string{value}
		default:
			r.DomainKeyword = []string{value}
		}
	case db.RuleTypeIp, db.RuleTypeCidr:
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			addr, e := netip.ParseAddr(value)
			if e != nil {
				return r, fmt.Errorf("invalid IP match")
			}
			prefix = netip.PrefixFrom(addr, addr.BitLen())
		}
		r.IPCIDR = []string{prefix.Masked().String()}
	default:
		return r, fmt.Errorf("rule type %q needs a configured rule-set and is not supported", kind)
	}
	return r, nil
}
