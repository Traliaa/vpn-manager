package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// Expansion is deliberately one-way. Changing LAN or replacing an existing
// router scope requires disabling/releasing it first, with router coordination.
func (m *Manager) expandScopeLocked(ctx context.Context, cfg ProfileConfig) error {
	old := *m.profile
	prefixErr := m.validateRouterScope(cfg)
	if prefixErr != nil {
		return prefixErr
	}
	if old.LANInterface != cfg.LANInterface {
		return fmt.Errorf("scope expansion must preserve LAN interface")
	}
	if !m.ready {
		return fmt.Errorf("recover pilot gateway before expanding scope")
	}
	// The previous pilot must belong to this connected LAN.
	if old.ClientIP != "" && (!subnetContains(cfg.ClientSubnet, old.ClientIP) || old.ClientIP == cfg.RouterIP || old.ClientIP == cfg.ManagerIP) {
		return fmt.Errorf("pilot client must belong to the new LAN scope")
	}
	if old.ClientIP == "" && !routerNATUpgrade(old, cfg) {
		return fmt.Errorf("only enabling router NAT compatibility is supported in place")
	}
	added := ruleDifference(orderedSourceRules(cfg, "add"), sourceRules(old))
	removed := ruleDifference(orderedSourceRules(old, "del"), sourceRules(cfg))
	raw, err := os.ReadFile(m.statePath)
	if err != nil {
		return err
	}
	var saved profileState
	if err = json.Unmarshal(raw, &saved); err != nil {
		return err
	}
	current, err := m.networkSnapshot(ctx)
	if err != nil {
		return err
	}
	if !saved.Enabled || saved.Config != old || !contained(current.Routes, saved.Routes) || !contained(saved.Routes, current.Routes) || !contained(current.Rules, saved.Rules) || !contained(saved.Rules, current.Rules) || current.NAT != saved.NAT {
		return fmt.Errorf("gateway objects changed; refusing scope expansion")
	}
	return m.gatewayTransaction(ctx, true, func() (result error) {
		newRules, oldRemoved, natReplaced := false, false, false
		defer func() {
			if result == nil {
				return
			}
			recovery, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var cleanup []error
			// Keep interception while restoring the old source and NAT, then remove
			// broader rules. A failed rollback is visible and never reported ready.
			if oldRemoved {
				cleanup = append(cleanup, m.changeSourceRules(recovery, "add", reversedRules(removed)))
			}
			if natReplaced {
				_, e := m.run(recovery, "nft", []string{"-f", "-"}, "delete table inet "+profileNFT+"\n"+profileNATBatch(old))
				cleanup = append(cleanup, e)
			}
			if newRules {
				cleanup = append(cleanup, m.changeSourceRules(recovery, "del", reversedRules(added)))
			}
			if e := errors.Join(cleanup...); e != nil {
				m.ready = false
				result = errors.Join(result, fmt.Errorf("scope rollback incomplete: %w", e))
			}
		}()
		if err := m.changeSourceRules(ctx, "add", added); err != nil {
			return err
		}
		newRules = true
		if _, err := m.run(ctx, "nft", []string{"-f", "-"}, "delete table inet "+profileNFT+"\n"+profileNATBatch(cfg)); err != nil {
			return err
		}
		natReplaced = true
		if err := m.changeSourceRules(ctx, "del", removed); err != nil {
			return err
		}
		oldRemoved = true
		if err := m.saveProfile(ctx, true, cfg); err != nil {
			return err
		}
		m.profile = &cfg
		m.ready = true
		return nil
	})
}

func reversedRules(rules [][]string) [][]string {
	out := make([][]string, len(rules))
	for i, r := range rules {
		out[len(rules)-1-i] = r
	}
	return out
}
