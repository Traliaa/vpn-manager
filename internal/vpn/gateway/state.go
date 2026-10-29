package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/Traliaa/vpn-manager/internal/config"
	"github.com/Traliaa/vpn-manager/internal/vpn/singbox"
	"go.uber.org/zap"
	"os"
	"sort"
)

func NewProfileManager(logger *zap.Logger, ctrl *singbox.Controller, cfg *config.Config) *Manager {
	m := NewManager(logger)
	m.profileCtrl = ctrl
	m.statePath = cfg.VPN.GatewayStatePath
	m.configureReloadHook()
	return m
}
func canonicalObjects(raw []byte, selected func(map[string]any) bool) ([]string, error) {
	var objects []map[string]any
	if err := json.Unmarshal(raw, &objects); err != nil {
		return nil, err
	}
	var out []string
	for _, o := range objects {
		if selected(o) {
			data, err := json.Marshal(o)
			if err != nil {
				return nil, err
			}
			out = append(out, string(data))
		}
	}
	sort.Strings(out)
	return out, nil
}
func nftFingerprint(raw []byte) (string, error) {
	var data struct {
		NFT []map[string]any `json:"nftables"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return "", err
	}
	var stripHandles func(any)
	stripHandles = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			delete(v, "handle")
			for _, child := range v {
				stripHandles(child)
			}
		case []any:
			for _, child := range v {
				stripHandles(child)
			}
		}
	}
	var objects []map[string]any
	for _, o := range data.NFT {
		if _, meta := o["metainfo"]; !meta {
			stripHandles(o)
			objects = append(objects, o)
		}
	}
	canonical, err := json.Marshal(objects)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(canonical)), nil
}
func (m *Manager) networkSnapshot(ctx context.Context) (profileState, error) {
	var state profileState
	raw, err := m.run(ctx, "ip", []string{"-j", "-4", "route", "show", "table", "all"}, "")
	if err != nil {
		return state, err
	}
	state.Routes, err = canonicalObjects(raw, func(o map[string]any) bool { return fmt.Sprint(o["table"]) == profileTable })
	if err != nil {
		return state, err
	}
	raw, err = m.run(ctx, "ip", []string{"-j", "-4", "rule", "show"}, "")
	if err != nil {
		return state, err
	}
	state.Rules, err = canonicalObjects(raw, func(o map[string]any) bool {
		return fmt.Sprint(o["table"]) == profileTable || fmt.Sprint(o["priority"]) == profilePriority
	})
	if err != nil {
		return state, err
	}
	raw, err = m.run(ctx, "nft", []string{"-j", "list", "table", "inet", profileNFT}, "")
	if err == errObjectMissing {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	state.NAT, err = nftFingerprint(raw)
	return state, err
}
func contained(current, saved []string) bool {
	for _, a := range current {
		found := false
		for _, b := range saved {
			if a == b {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// Restore accepts only exact objects observed when saving the private state.
// Missing objects after reboot are recreated; additions or edits are refused.
func (m *Manager) Restore(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configureReloadHook()
	return m.restoreLocked(ctx)
}
func (m *Manager) restoreLocked(ctx context.Context) (result error) {
	defer func() {
		if result != nil {
			m.lastError = result.Error()
			m.ready = false
		} else {
			m.lastError = ""
		}
	}()
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
	if err = m.validateProfile(saved.Config); err != nil {
		return err
	}
	current, err := m.networkSnapshot(ctx)
	if err != nil {
		return err
	}
	if !contained(current.Routes, saved.Routes) || !contained(current.Rules, saved.Rules) || (current.NAT != "" && current.NAT != saved.NAT) {
		return fmt.Errorf("persisted gateway objects changed; refusing automatic takeover")
	}
	m.profile = &saved.Config
	m.enabled = saved.Enabled
	m.ready = false
	if !saved.Enabled {
		if current.NAT == "" {
			if _, err = m.ensureProfileNAT(ctx, saved.Config); err != nil {
				return err
			}
		}
		m.ifaceName = saved.Config.LANInterface
		m.ready = true
		return nil
	}
	// Reconstruct the blackhole and source rule before permitting any direct
	// fallback. A missing database/profile must never turn capture into direct.
	for _, route := range profileRoutes()[:4] {
		if !routePresent(current.Routes, route) {
			if err = m.route(ctx, "add", route); err != nil {
				return err
			}
		}
	}
	if len(current.Rules) == 0 {
		if err = m.sourceRule(ctx, "add", saved.Config); err != nil {
			return err
		}
	}
	if m.profileReady == nil || !m.profileReady() {
		return fmt.Errorf("active routing profile is not ready; client remains blocked")
	}
	if current.NAT == "" {
		if _, err = m.ensureProfileNAT(ctx, saved.Config); err != nil {
			return err
		}
	}
	return m.gatewayTransaction(ctx, true, func() error {
		running, err := m.profileCtrl.IsRunning(ctx)
		if err != nil {
			return err
		}
		if !running {
			return fmt.Errorf("transparent proxy is not running")
		}
		current, err = m.networkSnapshot(ctx)
		if err != nil {
			return err
		}
		// Exact "add" operations are safe only for records missing from our saved
		// ownership snapshot. Never replace or flush the policy table.
		for _, route := range profileRoutes() {
			if routePresent(current.Routes, route) {
				continue
			}
			if err = m.route(ctx, "add", route); err != nil {
				return err
			}
		}
		if len(current.Rules) == 0 {
			if err = m.sourceRule(ctx, "add", saved.Config); err != nil {
				return err
			}
		}
		m.ifaceName = "vmtun0"
		m.ready = true
		return m.saveProfile(ctx, true, saved.Config)
	})
}
func routePresent(records []string, route []string) bool {
	typ := "unicast"
	dst := route[0]
	dev := ""
	metric := float64(0)
	start := 1
	if dst == "blackhole" || dst == "throw" {
		typ = dst
		dst = route[1]
		start = 2
	}
	for i := start; i+1 < len(route); i += 2 {
		if route[i] == "dev" {
			dev = route[i+1]
		}
		if route[i] == "metric" {
			if route[i+1] == "10" {
				metric = 10
			} else {
				metric = 32767
			}
		}
	}
	for _, raw := range records {
		var value map[string]any
		if json.Unmarshal([]byte(raw), &value) != nil {
			continue
		}
		actualType, _ := value["type"].(string)
		if actualType == "" {
			actualType = "unicast"
		}
		actualMetric, _ := value["metric"].(float64)
		actualDev, _ := value["dev"].(string)
		if value["dst"] == dst && actualType == typ && actualMetric == metric && actualDev == dev {
			return true
		}
	}
	return false
}
