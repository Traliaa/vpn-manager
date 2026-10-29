// Package singbox manages the sing-box VPN client as a backend for
// multiple VPN protocol outbounds (VLESS, Hysteria2, TUIC, Shadowsocks).
//
// It handles config generation, file management, process reload, and
// status monitoring via the sing-box REST API.
package singbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Traliaa/vpn-manager/internal/vpn/managedprocess"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Outbound defines the interface each protocol-specific config must satisfy
// to produce a sing-box outbound JSON fragment.
type Outbound interface {
	// Tag returns a unique tag for this outbound (e.g. "vless-main").
	Tag() string
	// BuildOutbound returns the full outbound JSON object for sing-box.
	BuildOutbound() (map[string]any, error)
}

// Controller manages the sing-box instance: config generation, writing,
// reload, and status queries.
type Controller struct {
	mu sync.RWMutex

	configPath   string
	binaryPath   string
	serviceName  string
	apiBaseURL   string
	apiEnabled   bool
	baseTemplate map[string]any

	activeOutbounds []Outbound
	logger          *zap.Logger
	process         managedprocess.Process
	rollback        *configSnapshot
}

// Config holds the settings for the sing-box Controller.
type Config struct {
	ConfigPath  string // path to sing-box config file (e.g. /etc/sing-box/config.json)
	BinaryPath  string // path to sing-box binary (e.g. /usr/local/bin/sing-box)
	ServiceName string // systemd service name (e.g. "sing-box")
	APIBaseURL  string // e.g. "http://127.0.0.1:9090"
	APIEnabled  bool   // whether sing-box was started with --api
	BaseConfig  string // optional path to a base template JSON file
}

// NewController creates a new sing-box Controller.
func NewController(cfg Config, logger *zap.Logger) (*Controller, error) {
	if cfg.ConfigPath == "" {
		cfg.ConfigPath = "/etc/sing-box/config.json"
	}
	if cfg.BinaryPath == "" {
		cfg.BinaryPath = "sing-box"
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = "sing-box"
	}
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = "http://127.0.0.1:9090"
	}

	c := &Controller{
		configPath:      cfg.ConfigPath,
		binaryPath:      cfg.BinaryPath,
		serviceName:     cfg.ServiceName,
		apiBaseURL:      strings.TrimRight(cfg.APIBaseURL, "/"),
		apiEnabled:      cfg.APIEnabled,
		baseTemplate:    make(map[string]any),
		activeOutbounds: make([]Outbound, 0),
		logger:          logger.Named("singbox-controller"),
	}

	// Try to load existing config as base template
	if cfg.BaseConfig != "" {
		if err := c.loadBaseTemplate(cfg.BaseConfig); err != nil {
			return nil, fmt.Errorf("load base template: %w", err)
		}
	} else {
		// Load current config if it exists
		if err := c.loadBaseTemplate(cfg.ConfigPath); err != nil {
			return nil, fmt.Errorf("load existing sing-box config: %w", err)
		}
	}

	return c, nil
}

// loadBaseTemplate reads a config file and extracts the non-outbound sections
// to use as a template for future config generation.
func (c *Controller) loadBaseTemplate(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // not an error, we'll use defaults
		}
		return fmt.Errorf("read config: %w", err)
	}

	var full map[string]any
	if err := json.Unmarshal(data, &full); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	// Reserve persisted provider targets as blocked until provider sync rebuilds
	// each real outbound. This lets startup validate multi-provider profiles
	// without temporarily sending a pending VPN target directly.
	if values, ok := full["outbounds"].([]any); ok {
		seen := map[string]bool{}
		for _, raw := range values {
			value, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid persisted outbound")
			}
			tag, ok := value["tag"].(string)
			if !ok || tag == "" {
				return fmt.Errorf("persisted outbound has no tag")
			}
			if seen[tag] {
				return fmt.Errorf("duplicate persisted outbound tag")
			}
			seen[tag] = true
			if tag != "direct" && tag != "block" {
				c.activeOutbounds = append(c.activeOutbounds, BlockedOutbound(tag))
			}
		}
	}
	// Keep everything except outbounds (we manage those)
	for k, v := range full {
		if k != "outbounds" {
			c.baseTemplate[k] = v
		}
	}
	return nil
}

// SetOutbounds replaces the active outbounds and regenerates the config.
func (c *Controller) SetOutbounds(ctx context.Context, outbounds []Outbound) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.updateLocked(ctx, outbounds, c.baseTemplate)
}
func (c *Controller) AddOutbound(ctx context.Context, ob Outbound) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	next := append([]Outbound(nil), c.activeOutbounds...)
	found := false
	for i, old := range next {
		if old.Tag() == ob.Tag() {
			next[i] = ob
			found = true
			break
		}
	}
	if !found {
		next = append(next, ob)
	}
	return c.updateLocked(ctx, next, c.baseTemplate)
}
func (c *Controller) RemoveOutbound(ctx context.Context, tag string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	next := make([]Outbound, 0, len(c.activeOutbounds))
	for _, ob := range c.activeOutbounds {
		if ob.Tag() == tag {
			next = append(next, BlockedOutbound(tag))
		} else {
			next = append(next, ob)
		}
	}
	return c.updateLocked(ctx, next, c.baseTemplate)
}
func (c *Controller) GetOutbound(tag string) (Outbound, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, ob := range c.activeOutbounds {
		if ob.Tag() == tag {
			return ob, true
		}
	}
	return nil, false
}
func (c *Controller) ListOutbounds() []Outbound {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]Outbound(nil), c.activeOutbounds...)
}

// Reload affects only the process created by this controller. A failed restart
// restores the last accepted configuration and its runtime when one was running.
func (c *Controller) Reload(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reloadLocked(ctx)
}
func (c *Controller) reloadLocked(ctx context.Context) error {
	if err := c.validatePath(ctx, c.configPath); err != nil {
		if c.rollback != nil {
			restoreErr := c.restoreLocked(c.rollback)
			if restoreErr == nil {
				c.rollback = nil
			}
			return errors.Join(err, restoreErr)
		}
		return err
	}
	wasRunning := c.process.Running()
	if err := c.process.Stop(ctx); err != nil {
		return err
	}
	if err := c.process.Start(ctx, c.binaryPath, []string{"run", "-c", c.configPath}, nil); err != nil {
		original := err
		if c.rollback != nil {
			if restoreErr := c.restoreLocked(c.rollback); restoreErr != nil {
				return errors.Join(original, restoreErr)
			}
			c.rollback = nil
			if wasRunning {
				recovery, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				return errors.Join(original, c.process.Start(recovery, c.binaryPath, []string{"run", "-c", c.configPath}, nil))
			}
		}
		return original
	}
	c.rollback = nil
	return nil
}
func (c *Controller) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.process.Stop(ctx)
}
func (c *Controller) Start(ctx context.Context) error {
	if c.process.Running() {
		return nil
	}
	return c.Reload(ctx)
}
func (c *Controller) IsRunning(ctx context.Context) (bool, error) { return c.process.Running(), nil }

// Status returns the current runtime info from sing-box API if enabled.
type Status struct {
	Outbounds     []OutboundStatus `json:"outbounds,omitempty"`
	MemoryUsage   int64            `json:"memory_usage_bytes,omitempty"`
	UptimeSeconds int64            `json:"uptime_seconds,omitempty"`
	Running       bool             `json:"running"`
}

// OutboundStatus represents per-outbound runtime data from sing-box API.
type OutboundStatus struct {
	Tag       string `json:"tag"`
	Type      string `json:"type"`
	Connected bool   `json:"connected"`
	RxBytes   int64  `json:"rx_bytes"`
	TxBytes   int64  `json:"tx_bytes"`
}

// FetchStatus queries the sing-box API for runtime stats.
// Returns nil if API is not enabled or unreachable.
func (c *Controller) FetchStatus(ctx context.Context) (*Status, error) {
	status := &Status{}

	running, err := c.IsRunning(ctx)
	if err != nil {
		return nil, err
	}
	status.Running = running
	if !running {
		return status, nil
	}

	if !c.apiEnabled {
		status.Running = true
		return status, nil
	}

	// Try to fetch group stats from sing-box API
	// sing-box exposes: GET /groups/{tag} (for balancers)
	// GET /outbounds/{tag} for per-outbound stats
	status.Running = true

	// Get uptime and memory from debug endpoint if available
	_ = c.fetchAPIData(ctx, "/debug", status)

	return status, nil
}

// fetchAPIData is a generic helper to call sing-box API endpoints.
func (c *Controller) fetchAPIData(ctx context.Context, path string, target any) error {
	apiURL, err := url.JoinPath(c.apiBaseURL, path)
	if err != nil {
		return fmt.Errorf("build api url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("api request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("api status %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

// ValidateConfig runs `sing-box check` to validate the generated config.
func (c *Controller) ValidateConfig(ctx context.Context) error {
	return c.validatePath(ctx, c.configPath)
}
func (c *Controller) validatePath(ctx context.Context, path string) error {
	if err := exec.CommandContext(ctx, c.binaryPath, "check", "-c", path).Run(); err != nil {
		return fmt.Errorf("sing-box configuration rejected: %w", err)
	}
	return nil
}

// buildConfig assembles the full sing-box config from template + outbounds.
func (c *Controller) buildConfig(outbounds []map[string]any) map[string]any {
	cfg := make(map[string]any)

	// Copy base template
	for k, v := range c.baseTemplate {
		cfg[k] = v
	}

	// Set outbounds
	cfg["outbounds"] = outbounds

	// Ensure default fields exist
	c.ensureDefaults(cfg)

	return cfg
}

// ensureDefaults adds sensible defaults for fields that might be missing.
func (c *Controller) ensureDefaults(cfg map[string]any) {
	if _, ok := cfg["log"]; !ok {
		cfg["log"] = map[string]any{
			"level":     "info",
			"output":    filepath.Join(filepath.Dir(c.configPath), "sing-box.log"),
			"timestamp": true,
		}
	}
}
