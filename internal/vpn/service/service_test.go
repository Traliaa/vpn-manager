package service

import (
	"github.com/Traliaa/vpn-manager/internal/db"
	"go.uber.org/zap"
	"testing"
)

func TestMalformedAmneziaWGNeverDowngradesToWireGuard(t *testing.T) {
	s := &Service{logger: zap.NewNop()}
	_, err := s.buildController(db.VpnProvider{Name: "bad-awg", ProviderType: db.ProviderTypeAmneziawg, Config: `{"h1":123,"private_key":"AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE="}`})
	if err == nil {
		t.Fatal("malformed AWG silently downgraded to a different protocol")
	}
}
