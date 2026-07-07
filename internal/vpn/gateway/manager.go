// Package gateway manages NAT masquerade and IP forwarding for VPN gateway mode.
package gateway

import (
	"context"
	"fmt"
	"os/exec"
	"sync"

	"go.uber.org/zap"
)

// GatewayStatus represents the current gateway state.
type GatewayStatus struct {
	Enabled   bool   `json:"enabled"`
	Interface string `json:"interface,omitempty"`
}

// Manager controls IP forwarding and nftables masquerade rules.
type Manager struct {
	mu        sync.Mutex
	enabled   bool
	ifaceName string
	logger    *zap.Logger
}

// NewManager creates a new gateway manager.
func NewManager(logger *zap.Logger) *Manager {
	return &Manager{
		logger: logger.Named("gateway"),
	}
}

// Enable activates gateway mode: IP forwarding + masquerade through ifaceName.
func (m *Manager) Enable(ctx context.Context, ifaceName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.enabled && m.ifaceName == ifaceName {
		m.logger.Info("gateway already enabled", zap.String("iface", ifaceName))
		return nil
	}

	// If already enabled with a different interface, disable first
	if m.enabled {
		if err := m.disableNolock(ctx); err != nil {
			m.logger.Warn("disable before re-enable", zap.Error(err))
		}
	}

	m.logger.Info("enabling gateway mode", zap.String("iface", ifaceName))

	// 1. Enable IP forwarding
	if err := exec.CommandContext(ctx, "sysctl", "-w", "net.ipv4.ip_forward=1").Run(); err != nil {
		return fmt.Errorf("enable ip_forward: %w", err)
	}

	// 2. Create nftables table for gateway
	if err := exec.CommandContext(ctx, "nft", "add", "table", "inet", "vpn-gateway").Run(); err != nil {
		// Table may already exist, that's ok
		m.logger.Debug("nft add table (may already exist)", zap.Error(err))
	}

	// 3. Create postrouting chain with nat hook
	if err := exec.CommandContext(ctx, "nft", "add", "chain", "inet", "vpn-gateway", "postrouting",
		"{ type nat hook postrouting priority 100; }").Run(); err != nil {
		return fmt.Errorf("nft add chain: %w", err)
	}

	// 4. Add masquerade rule for the VPN interface
	if err := exec.CommandContext(ctx, "nft", "add", "rule", "inet", "vpn-gateway", "postrouting",
		"oifname", ifaceName, "masquerade").Run(); err != nil {
		return fmt.Errorf("nft add masquerade rule: %w", err)
	}

	m.enabled = true
	m.ifaceName = ifaceName
	m.logger.Info("gateway mode enabled", zap.String("iface", ifaceName))
	return nil
}

// Disable deactivates gateway mode: removes rules and optionally disables forwarding.
func (m *Manager) Disable(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.disableNolock(ctx)
}

func (m *Manager) disableNolock(ctx context.Context) error {
	if !m.enabled {
		m.logger.Info("gateway already disabled")
		return nil
	}

	m.logger.Info("disabling gateway mode", zap.String("iface", m.ifaceName))

	// Remove nftables table (removes all rules in it)
	_ = exec.CommandContext(ctx, "nft", "delete", "table", "inet", "vpn-gateway").Run()

	// Disable IP forwarding
	_ = exec.CommandContext(ctx, "sysctl", "-w", "net.ipv4.ip_forward=0").Run()

	m.enabled = false
	m.ifaceName = ""
	m.logger.Info("gateway mode disabled")
	return nil
}

// Status returns the current gateway state.
func (m *Manager) Status() GatewayStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return GatewayStatus{
		Enabled:   m.enabled,
		Interface: m.ifaceName,
	}
}
