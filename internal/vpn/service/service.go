// Package service manages the lifecycle of VPN provider controllers,
// syncing them with provider configurations stored in the database.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

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
	queries *db.Queries
	logger  *zap.Logger
	mu      sync.Mutex
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

	for _, p := range providers {
		if err := s.syncProvider(ctx, p); err != nil {
			s.logger.Error("failed to sync provider",
				zap.String("name", p.Name),
				zap.Error(err),
			)
		}
	}

	s.logger.Info("providers synced", zap.Int("count", len(providers)))
	return nil
}

// syncProvider создаёт или обновляет контроллер для одного провайдера.
func (s *Service) syncProvider(ctx context.Context, p db.VpnProvider) error {
	if _, ok := s.manager.Get(p.Name); ok {
		return nil
	}

	controller, err := s.buildController(p)
	if err != nil {
		return fmt.Errorf("build controller for %q: %w", p.Name, err)
	}

	s.manager.Register(controller)

	// Применяем конфиг — создаём интерфейс в ядре
	var cfg interface{}
	_ = json.Unmarshal([]byte(p.Config), &cfg)
	if err := controller.ApplyConfig(ctx, cfg); err != nil {
		// Продолжаем даже с ошибкой — статус покажет проблему
		s.logger.Warn("apply config after sync",
			zap.String("provider", p.Name),
			zap.Error(err),
		)
	}

	s.logger.Info("provider synced from DB",
		zap.String("name", p.Name),
		zap.String("type", string(p.ProviderType)),
	)
	return nil
}

// buildController создаёт контроллер на основе записи провайдера в БД.
func (s *Service) buildController(p db.VpnProvider) (vpn.Provider, error) {
	switch p.ProviderType {
	case db.ProviderTypeAmneziawg:
		// Пробуем AmneziaWG, если не вышло — fallback на обычный WireGuard
		var awgCfg amneziawg.Config
		if err := json.Unmarshal([]byte(p.Config), &awgCfg); err == nil {
			if ctrl, err := amneziawg.NewController(p.Name, awgCfg, s.logger); err == nil {
				return ctrl, nil
			}
			s.logger.Warn("amneziawg controller failed, falling back to wireguard",
				zap.String("provider", p.Name),
				zap.Error(err),
			)
		}
		// Fallback: используем WireGuard контроллер
		var wgCfg wireguard.Config
		if err := json.Unmarshal([]byte(p.Config), &wgCfg); err != nil {
			return nil, fmt.Errorf("unmarshal fallback wireguard config: %w", err)
		}
		return wireguard.NewController(p.Name, wgCfg, s.logger)

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
		s.manager.SetOutboundTag(p.Name, p.Name)
		return provider, nil

	case db.ProviderTypeHysteria2:
		var cfg hysteria2.Config
		if err := json.Unmarshal([]byte(p.Config), &cfg); err != nil {
			return nil, fmt.Errorf("unmarshal hysteria2 config: %w", err)
		}
		cfg.OutboundTag = p.Name
		provider := vpn.NewSingBoxProvider(p.Name, vpn.ProviderHysteria2, cfg, s.sbCtrl, s.logger)
		s.manager.SetOutboundTag(p.Name, p.Name)
		return provider, nil

	case db.ProviderTypeTuic:
		var cfg tuic.Config
		if err := json.Unmarshal([]byte(p.Config), &cfg); err != nil {
			return nil, fmt.Errorf("unmarshal tuic config: %w", err)
		}
		cfg.OutboundTag = p.Name
		provider := vpn.NewSingBoxProvider(p.Name, vpn.ProviderTUIC, cfg, s.sbCtrl, s.logger)
		s.manager.SetOutboundTag(p.Name, p.Name)
		return provider, nil

	case db.ProviderTypeShadowsocks:
		var cfg shadowsocks.Config
		if err := json.Unmarshal([]byte(p.Config), &cfg); err != nil {
			return nil, fmt.Errorf("unmarshal shadowsocks config: %w", err)
		}
		cfg.OutboundTag = p.Name
		provider := vpn.NewSingBoxProvider(p.Name, vpn.ProviderShadowsocks, cfg, s.sbCtrl, s.logger)
		s.manager.SetOutboundTag(p.Name, p.Name)
		return provider, nil

	default:
		return nil, fmt.Errorf("unsupported provider type: %s", p.ProviderType)
	}
}

// RemoveProvider удаляет провайдера из менеджера.
func (s *Service) RemoveProvider(ctx context.Context, name string) error {
	p, ok := s.manager.Get(name)
	if !ok {
		return nil
	}
	if err := p.Remove(ctx); err != nil {
		return fmt.Errorf("remove provider %q: %w", name, err)
	}
	s.manager.Unregister(name)
	return nil
}
