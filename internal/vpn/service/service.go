// Package service manages the lifecycle of VPN provider controllers,
// syncing them with provider configurations stored in the database.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Traliaa/vpn-manager/internal/db"
	"github.com/Traliaa/vpn-manager/internal/vpn"
	"github.com/Traliaa/vpn-manager/internal/vpn/amneziawg"
	"github.com/Traliaa/vpn-manager/internal/vpn/hysteria2"
	"github.com/Traliaa/vpn-manager/internal/vpn/shadowsocks"
	"github.com/Traliaa/vpn-manager/internal/vpn/singbox"
	"github.com/Traliaa/vpn-manager/internal/vpn/tuic"
	"github.com/Traliaa/vpn-manager/internal/vpn/vless"
	"github.com/Traliaa/vpn-manager/internal/vpn/wireguard"
	"go.uber.org/zap"
)

// Service синхронизирует провайдеров из БД с VPN Manager.
type Service struct {
	manager *vpn.Manager
	sbCtrl  *singbox.Controller
	queries interface {
		ListEnabledProviders(context.Context) ([]db.VpnProvider, error)
	}
	build        func(db.VpnProvider) (vpn.Provider, error)
	logger       *zap.Logger
	mu           sync.Mutex
	fingerprints map[string][32]byte
}

// NewService создаёт сервис управления провайдерами.
func NewService(manager *vpn.Manager, sbCtrl *singbox.Controller, queries *db.Queries, logger *zap.Logger) *Service {
	return &Service{
		manager: manager,
		sbCtrl:  sbCtrl,
		queries: queries,
		logger:  logger.Named("vpn-service"),
	}
}

// SyncProviders синхронизирует всех включённых провайдеров из БД с менеджером.
func (s *Service) SyncProviders(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	providers, err := s.queries.ListEnabledProviders(ctx)
	if err != nil {
		return fmt.Errorf("list enabled providers: %w", err)
	}
	if s.fingerprints == nil {
		s.fingerprints = make(map[string][32]byte)
	}
	wanted := make(map[string]bool)
	for _, p := range providers {
		wanted[p.Name] = true
	}
	var errs []error
	// Validate all desired replacements before removing any currently working
	// provider, including a provider whose name is being changed.
	type replacement struct {
		record   db.VpnProvider
		provider vpn.Provider
		hash     [32]byte
	}
	var pending []replacement
	for _, p := range providers {
		fingerprint := sha256.Sum256([]byte(string(p.ProviderType) + "\x00" + p.Config))
		if _, ok := s.manager.Get(p.Name); ok && s.fingerprints[p.Name] == fingerprint {
			continue
		}
		candidate, err := s.prepareController(ctx, p)
		if err != nil {
			errs = append(errs, fmt.Errorf("provider %q: %w", p.Name, err))
			continue
		}
		pending = append(pending, replacement{p, candidate, fingerprint})
	}
	if len(errs) > 0 {
		for _, p := range pending {
			closePrepared(p.provider)
		}
		return errors.Join(errs...)
	}
	for _, p := range pending {
		if err := s.replaceProviderLocked(ctx, p.record, p.provider); err != nil {
			errs = append(errs, fmt.Errorf("provider %q: %w", p.record.Name, err))
			continue
		}
		s.fingerprints[p.record.Name] = p.hash
	}
	// A failed rename/startup keeps the previous name alive until the caller
	// restores desired database state. Never discard its only working runtime.
	if len(errs) == 0 {
		changed := false
		for _, p := range s.manager.List() {
			if !wanted[p.Name()] {
				if err := s.removeProviderLocked(ctx, p.Name()); err != nil {
					errs = append(errs, err)
				} else {
					changed = true
				}
			}
		}
		if changed {
			if err := s.sbCtrl.Reload(ctx); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}
func closePrepared(p vpn.Provider) {
	if closer, ok := p.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
}
func (s *Service) prepareController(ctx context.Context, p db.VpnProvider) (vpn.Provider, error) {
	build := s.build
	if build == nil {
		build = s.buildController
	}
	controller, err := build(p)
	if err != nil {
		return nil, err
	}
	var cfg any
	if err = json.Unmarshal([]byte(p.Config), &cfg); err != nil {
		closePrepared(controller)
		return nil, fmt.Errorf("invalid provider JSON")
	}
	if err = controller.ApplyConfig(ctx, cfg); err != nil {
		closePrepared(controller)
		return nil, err
	}
	if validator, ok := controller.(interface{ ValidateConfig() error }); ok {
		if err = validator.ValidateConfig(); err != nil {
			closePrepared(controller)
			return nil, err
		}
	}
	return controller, nil
}
func (s *Service) replaceProviderLocked(ctx context.Context, p db.VpnProvider, candidate vpn.Provider) error {
	old, hadOld := s.manager.Get(p.Name)
	previousOutbound, hadOutbound := s.sbCtrl.GetOutbound(p.Name)
	// Legacy WireGuard has no safe owned reconnect lifecycle. Do not destroy an
	// existing connection while replacing it; the AWG pilot uses owned userspace.
	if hadOld && old.Type() == vpn.ProviderWireGuard {
		closePrepared(candidate)
		return fmt.Errorf("live replacement of legacy WireGuard requires an explicit restart")
	}
	if hadOld {
		if err := old.Disconnect(ctx); err != nil {
			closePrepared(candidate)
			return err
		}
	}
	rollback := func(cause error) error {
		recovery, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cleanupErr := candidate.Disconnect(recovery)
		closePrepared(candidate)
		var restoreErr error
		if hadOld {
			restoreErr = old.Connect(recovery)
		}
		if hadOutbound {
			restoreErr = errors.Join(restoreErr, s.sbCtrl.AddOutbound(recovery, previousOutbound))
		} else {
			restoreErr = errors.Join(restoreErr, s.sbCtrl.RemoveOutbound(recovery, p.Name))
		}
		if hadOld || hadOutbound {
			restoreErr = errors.Join(restoreErr, s.sbCtrl.Reload(recovery))
		}
		return errors.Join(cause, cleanupErr, restoreErr)
	}
	if err := candidate.Connect(ctx); err != nil {
		return rollback(err)
	}
	if candidate.InterfaceName() != "" {
		var address struct {
			Address string `json:"address"`
		}
		if err := json.Unmarshal([]byte(p.Config), &address); err != nil {
			return rollback(err)
		}
		if err := s.sbCtrl.AddOutbound(ctx, singbox.InterfaceOutbound{Name: p.Name, Interface: candidate.InterfaceName(), Address: address.Address}); err != nil {
			return rollback(err)
		}
	}
	if err := s.sbCtrl.Reload(ctx); err != nil {
		return rollback(err)
	}
	s.manager.Register(candidate)
	s.manager.SetOutboundTag(p.Name, p.Name)
	return nil
}

// buildController создаёт контроллер на основе записи провайдера в БД.
func (s *Service) buildController(p db.VpnProvider) (vpn.Provider, error) {
	switch p.ProviderType {
	case db.ProviderTypeAmneziawg:
		var cfg amneziawg.Config
		if err := json.Unmarshal([]byte(p.Config), &cfg); err != nil {
			return nil, fmt.Errorf("unmarshal AmneziaWG config: %w", err)
		}
		return amneziawg.NewController(p.Name, cfg, s.logger)

	case db.ProviderTypeWireguard:
		var cfg wireguard.Config
		if err := json.Unmarshal([]byte(p.Config), &cfg); err != nil {
			return nil, fmt.Errorf("unmarshal wireguard config: %w", err)
		}
		return wireguard.NewController(p.Name, cfg, s.logger)

	case db.ProviderTypeVless:
		var cfg vless.Config
		if err := json.Unmarshal([]byte(p.Config), &cfg); err != nil {
			return nil, fmt.Errorf("unmarshal vless config: %w", err)
		}
		cfg.OutboundTag = p.Name
		provider := vpn.NewSingBoxProvider(p.Name, vpn.ProviderVLESS, cfg, s.sbCtrl, s.logger)
		return provider, nil

	case db.ProviderTypeHysteria2:
		var cfg hysteria2.Config
		if err := json.Unmarshal([]byte(p.Config), &cfg); err != nil {
			return nil, fmt.Errorf("unmarshal hysteria2 config: %w", err)
		}
		cfg.OutboundTag = p.Name
		provider := vpn.NewSingBoxProvider(p.Name, vpn.ProviderHysteria2, cfg, s.sbCtrl, s.logger)
		return provider, nil

	case db.ProviderTypeTuic:
		var cfg tuic.Config
		if err := json.Unmarshal([]byte(p.Config), &cfg); err != nil {
			return nil, fmt.Errorf("unmarshal tuic config: %w", err)
		}
		cfg.OutboundTag = p.Name
		provider := vpn.NewSingBoxProvider(p.Name, vpn.ProviderTUIC, cfg, s.sbCtrl, s.logger)
		return provider, nil

	case db.ProviderTypeShadowsocks:
		var cfg shadowsocks.Config
		if err := json.Unmarshal([]byte(p.Config), &cfg); err != nil {
			return nil, fmt.Errorf("unmarshal shadowsocks config: %w", err)
		}
		cfg.OutboundTag = p.Name
		provider := vpn.NewSingBoxProvider(p.Name, vpn.ProviderShadowsocks, cfg, s.sbCtrl, s.logger)
		return provider, nil

	default:
		return nil, fmt.Errorf("unsupported provider type: %s", p.ProviderType)
	}
}

// RemoveProvider удаляет провайдера из менеджера.
func (s *Service) RemoveProvider(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.removeProviderLocked(ctx, name)
}
func (s *Service) removeProviderLocked(ctx context.Context, name string) error {
	p, ok := s.manager.Get(name)
	if !ok {
		return nil
	}
	if p.InterfaceName() != "" {
		if err := s.sbCtrl.RemoveOutbound(ctx, name); err != nil {
			return err
		}
	}
	if err := p.Disconnect(ctx); err != nil {
		return err
	}
	s.manager.Unregister(name)
	delete(s.fingerprints, name)
	return nil
}
