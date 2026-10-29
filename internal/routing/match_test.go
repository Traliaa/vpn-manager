package routing

import (
	"github.com/Traliaa/vpn-manager/internal/db"
	"testing"
)

func TestDomainMatchDoesNotDependOnResolvedAddresses(t *testing.T) {
	r, err := matchRule(db.RuleTypeDomainSuffix, "youtube.com")
	if err != nil || len(r.DomainSuffix) != 1 || len(r.IPCIDR) != 0 {
		t.Fatalf("invalid domain match %#v %v", r, err)
	}
}
func TestUnsupportedOrMalformedMatchesRejected(t *testing.T) {
	for _, v := range []struct {
		kind  db.RuleType
		value string
	}{{db.RuleTypeGeosite, "youtube"}, {db.RuleTypeGeoip, "ru"}, {db.RuleTypeAsn, "123"}, {db.RuleTypeCidr, "invalid"}, {db.RuleTypeDomain, "https://youtube.com"}, {db.RuleTypeDomainSuffix, ""}} {
		if _, err := matchRule(v.kind, v.value); err == nil {
			t.Fatalf("accepted %#v", v)
		}
	}
}
