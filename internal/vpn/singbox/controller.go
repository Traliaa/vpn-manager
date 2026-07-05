// Package singbox manages the sing-box VPN client as a backend for
// multiple VPN protocol outbounds (VLESS, Hysteria2, TUIC, Shadowsocks).
//
// It handles config generation, file management, process reload, and
// status monitoring via the sing-box REST API.
package singbox

import (
	"context"
	"encoding/json"
	"fmt"
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
		_ = c.loadBaseTemplate(cfg.ConfigPath)
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

	return c.setOutboundsLocked(ctx, outbounds)
}

// AddOutbound adds or replaces an outbound by its Tag, then applies the config.
func (c *Controller) AddOutbound(ctx context.Context, ob Outbound) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if tag already exists — replace it
	tag := ob.Tag()
	for i, existing := range c.activeOutbounds {
		if existing.Tag() == tag {
			c.activeOutbounds[i] = ob
			return c.applyConfigLocked(ctx)
		}
	}

	// Append new outbound
	c.activeOutbounds = append(c.activeOutbounds, ob)
	return c.applyConfigLocked(ctx)
}

// RemoveOutbound removes an outbound by tag, then applies the config.
func (c *Controller) RemoveOutbound(ctx context.Context, tag string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	idx := -1
	for i, ob := range c.activeOutbounds {
		if ob.Tag() == tag {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil // not found, nothing to remove
	}

	c.activeOutbounds = append(c.activeOutbounds[:idx], c.activeOutbounds[idx+1:]...)
	return c.applyConfigLocked(ctx)
}

// GetOutbound finds an outbound by tag.
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

// ListOutbounds returns all currently active outbounds.
func (c *Controller) ListOutbounds() []Outbound {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]Outbound, len(c.activeOutbounds))
	copy(result, c.activeOutbounds)
	return result
}

// setOutboundsLocked replaces all outbounds without acquiring the lock (caller must hold mu).
func (c *Controller) setOutboundsLocked(ctx context.Context, outbounds []Outbound) error {
	c.activeOutbounds = make([]Outbound, len(outbounds))
	copy(c.activeOutbounds, outbounds)
	return c.applyConfigLocked(ctx)
}

// applyConfigLocked writes the current config and reloads sing-box (caller must hold mu).
func (c *Controller) applyConfigLocked(ctx context.Context) error {
	// Build outbound JSON
	outboundList := make([]map[string]any, 0, len(c.activeOutbounds))
	for _, ob := range c.activeOutbounds {
		built, err := ob.BuildOutbound()
		if err != nil {
			return fmt.Errorf("build outbound %q: %w", ob.Tag(), err)
		}
		outboundList = append(outboundList, built)
	}

	// Build full config
	cfg := c.buildConfig(outboundList)
	configBytes, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	// Ensure the config directory exists
	configDir := filepath.Dir(c.configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("create config dir %s: %w", configDir, err)
	}

	// Backup current config first
	if err := c.backupConfig(); err != nil {
		c.logger.Warn("config backup failed (non-fatal)", zap.Error(err))
	}

	// Write to disk
	if err := os.WriteFile(c.configPath, configBytes, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	c.logger.Info("sing-box config written",
		zap.String("path", c.configPath),
		zap.Int("outbounds", len(outboundList)),
	)
	return nil
}

// Reload signals sing-box to reload its configuration.
// In Docker, sends SIGHUP to reload, or starts sing-box if not running.
func (c *Controller) Reload(ctx context.Context) error {
	// Try SIGHUP (graceful reload)
	if err := exec.CommandContext(ctx, "pkill", "-SIGHUP", "sing-box").Run(); err == nil {
		c.logger.Info("sing-box reloaded via SIGHUP")
		return nil
	}

	// Not running — start it in background
	startCmd := exec.Command(c.binaryPath, "run", "-c", c.configPath)
	startCmd.Stdout = nil
	startCmd.Stderr = nil
	if err := startCmd.Start(); err != nil {
		return fmt.Errorf("start sing-box: %w", err)
	}

	c.logger.Info("sing-box started in background")
	return nil
}

// Stop stops the sing-box process.
func (c *Controller) Stop(ctx context.Context) error {
	if err := exec.CommandContext(ctx, "pkill", "sing-box").Run(); err != nil {
		return fmt.Errorf("stop sing-box: %w", err)
	}
	c.logger.Info("sing-box stopped")
	return nil
}

// Start starts the sing-box process in background.
func (c *Controller) Start(ctx context.Context) error {
	startCmd := exec.Command(c.binaryPath, "run", "-c", c.configPath)
	startCmd.Stdout = nil
	startCmd.Stderr = nil
	if err := startCmd.Start(); err != nil {
		return fmt.Errorf("start sing-box: %w", err)
	}
	c.logger.Info("sing-box started in background")
	return nil
}

// IsRunning checks if sing-box process is running.
func (c *Controller) IsRunning(ctx context.Context) (bool, error) {
	if err := exec.CommandContext(ctx, "pgrep", "sing-box").Run(); err != nil {
		return false, nil
	}
	return true, nil
}

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
	cmd := exec.CommandContext(ctx, c.binaryPath, "check", "-c", c.configPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("config validation: %w\n%s", err, string(out))
	}
	c.logger.Debug("sing-box config validated")
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

// backupConfig creates a timestamped backup of the current config.
func (c *Controller) backupConfig() error {
	data, err := os.ReadFile(c.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	backupPath := fmt.Sprintf("%s.bak.%d", c.configPath, time.Now().Unix())
	return os.WriteFile(backupPath, data, 0644)
}
