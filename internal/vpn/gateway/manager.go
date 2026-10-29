// Package gateway manages NAT masquerade and IP forwarding for VPN gateway mode.
package gateway

import (
	"context"
	"fmt"
	"net/netip"
	"os/exec"
	"strings"
	"sync"

	"go.uber.org/zap"
)

const nftTableName = "vpn_manager_nat"

// GatewayStatus represents the current gateway state.
type GatewayStatus struct {
	Enabled      bool   `json:"enabled"`
	Ready        bool   `json:"ready"`
	ClientIP     string `json:"client_ip,omitempty"`
	Error        string `json:"error,omitempty"`
	LANInterface string `json:"lan_interface,omitempty"`
	Interface    string `json:"interface,omitempty"`
}

// Manager controls IP forwarding and nftables masquerade rules.
type Manager struct {
	mu        sync.Mutex
	enabled   bool
	ifaceName string
	// ipForwardOrig сохраняет значение net.ipv4.ip_forward до вмешательства менеджера.
	ipForwardOrig  string
	logger         *zap.Logger
	profileCtrl    profileController
	run            commandRunner
	statePath      string
	localAddresses func() ([]netip.Addr, error)
	profile        *ProfileConfig
	ready          bool
	lastError      string
}

// NewManager creates a new gateway manager.
func NewManager(logger *zap.Logger) *Manager {
	return &Manager{
		logger: logger.Named("gateway"),
		run:    runCommand, localAddresses: interfaceAddresses,
	}
}

// readIPForward возвращает текущее значение net.ipv4.ip_forward.
func readIPForward(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "sysctl", "-n", "net.ipv4.ip_forward").Output()
	if err != nil {
		return "", fmt.Errorf("read ip_forward: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// interfaceIsUp проверяет, существует ли интерфейс и поднят ли он.
func interfaceIsUp(ctx context.Context, ifaceName string) error {
	out, err := exec.CommandContext(ctx, "ip", "link", "show", ifaceName).Output()
	if err != nil {
		return fmt.Errorf("interface %q not found: %w", ifaceName, err)
	}
	// Пример вывода: "3: awg0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1420 qdisc ..."
	if !strings.Contains(string(out), "UP") && !strings.Contains(string(out), "UNKNOWN") {
		return fmt.Errorf("interface %q exists but is not UP", ifaceName)
	}
	return nil
}

// Enable activates gateway mode: IP forwarding + masquerade through ifaceName.
func (m *Manager) Enable(ctx context.Context, ifaceName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.profile != nil {
		return fmt.Errorf("release profile gateway before enabling legacy NAT mode")
	}
	if m.enabled && m.ifaceName == ifaceName {
		m.logger.Info("gateway already enabled", zap.String("iface", ifaceName))
		return nil
	}

	// 0. Проверяем, что интерфейс действительно активен
	if err := interfaceIsUp(ctx, ifaceName); err != nil {
		return fmt.Errorf("cannot enable gateway: %w", err)
	}

	// Если уже включён с другим интерфейсом — переключиться
	if m.enabled {
		if err := m.disableNolock(ctx); err != nil {
			m.logger.Warn("disable before re-enable", zap.Error(err))
		}
	}

	m.logger.Info("enabling gateway mode", zap.String("iface", ifaceName))

	// 1. Сохраняем предыдущее значение ip_forward и включаем форвардинг
	prev, err := readIPForward(ctx)
	if err != nil {
		return err
	}
	m.ipForwardOrig = prev

	if err := exec.CommandContext(ctx, "sysctl", "-w", "net.ipv4.ip_forward=1").Run(); err != nil {
		return fmt.Errorf("enable ip_forward: %w", err)
	}

	// 2. Удаляем старую таблицу nft, если есть (для идемпотентности)
	_ = exec.CommandContext(ctx, "nft", "delete", "table", "inet", nftTableName).Run()

	// 3. Создаём таблицу и chain заново
	if err := exec.CommandContext(ctx, "nft", "add", "table", "inet", nftTableName).Run(); err != nil {
		return fmt.Errorf("nft add table: %w", err)
	}
	if err := exec.CommandContext(ctx, "nft", "add", "chain", "inet", nftTableName, "postrouting",
		"{ type nat hook postrouting priority 100; }").Run(); err != nil {
		return fmt.Errorf("nft add chain: %w", err)
	}

	// 4. Добавляем masquerade для выходного интерфейса
	if err := exec.CommandContext(ctx, "nft", "add", "rule", "inet", nftTableName, "postrouting",
		"oifname", ifaceName, "masquerade").Run(); err != nil {
		return fmt.Errorf("nft add masquerade rule: %w", err)
	}

	m.enabled = true
	m.ifaceName = ifaceName
	m.logger.Info("gateway mode enabled", zap.String("iface", ifaceName))
	return nil
}

// Disable deactivates gateway mode: removes rules and restores ip_forward.
func (m *Manager) Disable(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.profile != nil {
		return m.disableProfileLocked(ctx)
	}
	return m.disableNolock(ctx)
}

func (m *Manager) disableNolock(ctx context.Context) error {
	if !m.enabled {
		m.logger.Info("gateway already disabled")
		return nil
	}

	m.logger.Info("disabling gateway mode", zap.String("iface", m.ifaceName))

	// Удаляем nftables таблицу (с ней удаляются все правила)
	_ = exec.CommandContext(ctx, "nft", "delete", "table", "inet", nftTableName).Run()

	// Восстанавливаем ip_forward в предыдущее состояние
	if m.ipForwardOrig != "" {
		if err := exec.CommandContext(ctx, "sysctl", "-w", "net.ipv4.ip_forward="+m.ipForwardOrig).Run(); err != nil {
			m.logger.Warn("restore ip_forward", zap.String("value", m.ipForwardOrig), zap.Error(err))
		}
	}

	m.enabled = false
	m.ifaceName = ""
	m.ipForwardOrig = ""
	m.logger.Info("gateway mode disabled")
	return nil
}

// Status returns the current gateway state.
func (m *Manager) Status() GatewayStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	ready := m.ready
	if m.profile == nil {
		ready = m.enabled
	}
	message := m.lastError
	if m.profile != nil && m.enabled {
		running, err := m.profileCtrl.IsRunning(context.Background())
		if err != nil || !running {
			ready = false
			message = "transparent proxy is not running"
		}
	}
	return GatewayStatus{
		Enabled: m.enabled,
		LANInterface: func() string {
			if m.profile != nil {
				return m.profile.LANInterface
			}
			return ""
		}(),
		Ready: ready,
		Error: message,
		ClientIP: func() string {
			if m.profile != nil {
				return m.profile.ClientIP
			}
			return ""
		}(),
		Interface: m.ifaceName,
	}
}
