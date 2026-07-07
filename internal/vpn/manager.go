package vpn

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"
)

// Manager orchestrates multiple VPN providers and their lifecycle.
type Manager struct {
	mu        sync.RWMutex
	providers map[string]Provider
	// outboundTags maps provider name → sing-box outbound tag (if applicable)
	outboundTags map[string]string
	logger       *zap.Logger
}

// NewManager creates a new VPN provider manager.
func NewManager(logger *zap.Logger) *Manager {
	return &Manager{
		providers:    make(map[string]Provider),
		outboundTags: make(map[string]string),
		logger:       logger.Named("vpn-manager"),
	}
}

// Register adds a provider to the manager.
func (m *Manager) Register(p Provider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.providers[p.Name()] = p
	m.logger.Info("provider registered",
		zap.String("name", p.Name()),
		zap.String("type", string(p.Type())),
	)
}

// Unregister removes a provider from the manager.
func (m *Manager) Unregister(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.providers, name)
	m.logger.Info("provider unregistered", zap.String("name", name))
}

// Get returns a provider by name.
func (m *Manager) Get(name string) (Provider, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.providers[name]
	return p, ok
}

// List returns all registered providers.
func (m *Manager) List() []Provider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]Provider, 0, len(m.providers))
	for _, p := range m.providers {
		result = append(result, p)
	}
	return result
}

// SetOutboundTag records the sing-box outbound tag for a provider.
func (m *Manager) SetOutboundTag(providerName, tag string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.outboundTags[providerName] = tag
}

// OutboundTag returns the sing-box outbound tag for a provider.
func (m *Manager) OutboundTag(providerName string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tag, ok := m.outboundTags[providerName]
	return tag, ok
}

// AllOutboundTags returns a copy of all outbound tag mappings.
func (m *Manager) AllOutboundTags() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]string, len(m.outboundTags))
	for k, v := range m.outboundTags {
		result[k] = v
	}
	return result
}

// StartAll brings up all registered providers.
func (m *Manager) StartAll(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for name, p := range m.providers {
		status, _ := p.Status(ctx)
		if status != nil && status.State == StateUp {
			continue
		}
		m.logger.Info("starting provider", zap.String("name", name))
		if healthErr := p.HealthCheck(ctx); healthErr != nil {
			m.logger.Warn("provider health check failed",
				zap.String("name", name),
				zap.Error(healthErr),
			)
		}
	}
	return nil
}

// StopAll tears down all registered providers.
func (m *Manager) StopAll(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var lastErr error
	for name, p := range m.providers {
		m.logger.Info("stopping provider", zap.String("name", name))
		if err := p.Disconnect(ctx); err != nil {
			m.logger.Error("failed to stop provider",
				zap.String("name", name),
				zap.Error(err),
			)
			lastErr = fmt.Errorf("stop %s: %w", name, err)
		}
	}
	return lastErr
}

// AllStatuses returns the status of all registered providers.
func (m *Manager) AllStatuses(ctx context.Context) []*InterfaceStatus {
	providers := m.List()
	result := make([]*InterfaceStatus, 0, len(providers))
	for _, p := range providers {
		status, err := p.Status(ctx)
		if err != nil {
			m.logger.Warn("failed to get status",
				zap.String("name", p.Name()),
				zap.Error(err),
			)
			status = &InterfaceStatus{
				Name:  p.Name(),
				Type:  p.Type(),
				State: StateError,
				Error: err.Error(),
			}
		}
		result = append(result, status)
	}
	return result
}
