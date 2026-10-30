package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

func (m *Manager) configureReloadHook() {
	if ctrl, ok := m.profileCtrl.(interface {
		SetReloadHook(func(context.Context, bool) error)
	}); ok {
		ctrl.SetReloadHook(m.restoreTUNRoute)
	}
}

// Called under the sing-box lock, so it deliberately never takes Manager.mu or
// reads mutable runtime flags. Atomic private disk state proves route ownership.
func (m *Manager) restoreTUNRoute(ctx context.Context, active bool) error {
	if !active {
		return nil
	}
	raw, err := os.ReadFile(m.statePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var saved profileState
	if err = json.Unmarshal(raw, &saved); err != nil {
		return err
	}
	if !saved.Enabled {
		return nil
	}
	current, err := m.networkSnapshot(ctx)
	if err != nil {
		return err
	}
	if !contained(current.Routes, saved.Routes) || !contained(current.Rules, saved.Rules) || current.NAT != saved.NAT {
		return fmt.Errorf("gateway objects changed during proxy reload")
	}
	primary := []string{"default", "dev", "vmtun0", "metric", "10"}
	if routePresent(current.Routes, primary) {
		return nil
	}
	return m.route(ctx, "add", primary)
}

func (m *Manager) gatewayTransaction(ctx context.Context, enabled bool, stage func() error) error {
	if ctrl, ok := m.profileCtrl.(interface {
		GatewayTransaction(context.Context, bool, func() error) error
	}); ok {
		return ctrl.GatewayTransaction(ctx, enabled, stage)
	}
	if err := m.profileCtrl.SetGateway(ctx, enabled); err != nil {
		return err
	}
	return stage()
}
