package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"github.com/Traliaa/vpn-manager/internal/db"
	"github.com/Traliaa/vpn-manager/internal/vpn"
	"github.com/Traliaa/vpn-manager/internal/vpn/singbox"
	"go.uber.org/zap"
	"os"
	"path/filepath"
	"testing"
)

type providerList []db.VpnProvider

func (p providerList) ListEnabledProviders(context.Context) ([]db.VpnProvider, error) { return p, nil }

type replacementProvider struct {
	name                 string
	alive                bool
	applyErr, connectErr error
	disconnects          int
}

func (p *replacementProvider) Name() string                           { return p.name }
func (p *replacementProvider) Type() vpn.ProviderType                 { return vpn.ProviderVLESS }
func (p *replacementProvider) InterfaceName() string                  { return "" }
func (p *replacementProvider) ApplyConfig(context.Context, any) error { return p.applyErr }
func (p *replacementProvider) Connect(context.Context) error {
	if p.connectErr != nil {
		return p.connectErr
	}
	p.alive = true
	return nil
}
func (p *replacementProvider) Disconnect(context.Context) error {
	p.disconnects++
	p.alive = false
	return nil
}
func (p *replacementProvider) Remove(ctx context.Context) error { return p.Disconnect(ctx) }
func (p *replacementProvider) Status(context.Context) (*vpn.InterfaceStatus, error) {
	state := vpn.StateDown
	if p.alive {
		state = vpn.StateUp
	}
	return &vpn.InterfaceStatus{State: state}, nil
}
func (p *replacementProvider) HealthCheck(context.Context) error { return nil }
func TestFailedReplacementRetainsPreviousWorkingProvider(t *testing.T) {
	for _, stage := range []string{"validation", "startup"} {
		t.Run(stage, func(t *testing.T) {
			dir := t.TempDir()
			binary := filepath.Join(dir, "sing-box")
			if err := os.WriteFile(binary, []byte("#!/bin/sh\nif [ \"$1\" = check ]; then exit 0; fi\nexec sleep 60\n"), 0700); err != nil {
				t.Fatal(err)
			}
			ctrl, err := singbox.NewController(singbox.Config{ConfigPath: filepath.Join(dir, "config.json"), BinaryPath: binary}, zap.NewNop())
			if err != nil {
				t.Fatal(err)
			}
			defer ctrl.Stop(context.Background())
			manager := vpn.NewManager(zap.NewNop())
			old := &replacementProvider{name: "vpn", alive: true}
			manager.Register(old)
			manager.SetOutboundTag("vpn", "vpn")
			candidate := &replacementProvider{name: "vpn"}
			if stage == "validation" {
				candidate.applyErr = errors.New("invalid config")
			} else {
				candidate.connectErr = errors.New("startup failed")
			}
			oldHash := sha256.Sum256([]byte("old"))
			s := &Service{manager: manager, sbCtrl: ctrl, logger: zap.NewNop(), queries: providerList{{Name: "vpn", ProviderType: db.ProviderTypeVless, Config: `{"new":true}`}}, fingerprints: map[string][32]byte{"vpn": oldHash}, build: func(db.VpnProvider) (vpn.Provider, error) { return candidate, nil }}
			if err = s.SyncProviders(context.Background()); err == nil {
				t.Fatal("replacement failure ignored")
			}
			current, ok := manager.Get("vpn")
			if !ok || current != old || !old.alive {
				t.Fatal("previous working provider lost")
			}
			if stage == "validation" && old.disconnects != 0 {
				t.Fatal("invalid config disconnected working provider")
			}
			if s.fingerprints["vpn"] != oldHash {
				t.Fatal("failed replacement committed fingerprint")
			}
		})
	}
}
