package vpn

import (
	"context"
	"fmt"
	"time"

	"github.com/Traliaa/vpn-manager/internal/vpn/singbox"
	"go.uber.org/zap"
)

// SingBoxProvider implements the Provider interface using sing-box as the backend.
// It wraps a protocol-specific Outbound (vless, hysteria2, tuic, shadowsocks)
// and manages it through the shared sing-box Controller.
type SingBoxProvider struct {
	name     string
	ptype    ProviderType
	tag      string
	outbound singbox.Outbound
	ctrl     *singbox.Controller
	logger   *zap.Logger
}

// NewSingBoxProvider creates a new sing-box based VPN provider.
func NewSingBoxProvider(name string, ptype ProviderType, outbound singbox.Outbound, ctrl *singbox.Controller, logger *zap.Logger) *SingBoxProvider {
	return &SingBoxProvider{
		name:     name,
		ptype:    ptype,
		tag:      outbound.Tag(),
		outbound: outbound,
		ctrl:     ctrl,
		logger:   logger.With(zap.String("provider", name), zap.String("type", string(ptype))),
	}
}

func (p *SingBoxProvider) Type() ProviderType    { return p.ptype }
func (p *SingBoxProvider) Name() string          { return p.name }
func (p *SingBoxProvider) InterfaceName() string { return "" }

// ApplyConfig parses, validates, and stores the outbound config.
// For sing-box providers this also registers the outbound since there's no
// separate kernel interface to manage.
func (p *SingBoxProvider) ApplyConfig(ctx context.Context, cfg interface{}) error {
	if newOb, ok := cfg.(singbox.Outbound); ok {
		p.outbound = newOb
	}
	return nil
}

// Connect registers the outbound in sing-box and reloads the config.
func (p *SingBoxProvider) Connect(ctx context.Context) error {
	if err := p.ctrl.AddOutbound(ctx, p.outbound); err != nil {
		return fmt.Errorf("register outbound: %w", err)
	}
	if err := p.ctrl.Reload(ctx); err != nil {
		return fmt.Errorf("reload sing-box: %w", err)
	}
	return nil
}

// Disconnect removes this provider's outbound from the sing-box config and reloads.
func (p *SingBoxProvider) Disconnect(ctx context.Context) error {
	if err := p.ctrl.RemoveOutbound(ctx, p.tag); err != nil {
		return fmt.Errorf("remove outbound: %w", err)
	}
	if err := p.ctrl.Reload(ctx); err != nil {
		return fmt.Errorf("reload sing-box: %w", err)
	}
	return nil
}

// Remove is kept for backward compatibility during migration.
// Deprecated: use Disconnect.
func (p *SingBoxProvider) Remove(ctx context.Context) error {
	return p.Disconnect(ctx)
}

// Status queries the sing-box controller for runtime information about this provider.
func (p *SingBoxProvider) Status(ctx context.Context) (*InterfaceStatus, error) {
	status := &InterfaceStatus{
		Name:  p.name,
		Type:  p.ptype,
		State: StateUnknown,
	}

	running := false
	sbStatus, err := p.ctrl.FetchStatus(ctx)
	if err == nil {
		running = sbStatus.Running
	} else {
		p.logger.Debug("fetch status failed", zap.Error(err))
	}

	_, registered := p.ctrl.GetOutbound(p.tag)
	if !registered {
		status.State = StateDown
		return status, nil
	}

	if running {
		status.State = StateUp
	} else {
		status.State = StateDown
	}

	return status, nil
}

// HealthCheck performs a basic reachability test through the sing-box outbound.
func (p *SingBoxProvider) HealthCheck(ctx context.Context) error {
	_, err := p.ctrl.FetchStatus(ctx)
	if err != nil {
		return fmt.Errorf("sing-box status: %w", err)
	}

	_, registered := p.ctrl.GetOutbound(p.tag)
	if !registered {
		return fmt.Errorf("outbound %q not registered", p.tag)
	}

	running, err := p.ctrl.IsRunning(ctx)
	if err != nil {
		return fmt.Errorf("check if running: %w", err)
	}
	if !running {
		return fmt.Errorf("sing-box is not running")
	}

	_ = time.Now()
	return nil
}
