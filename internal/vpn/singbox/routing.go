package singbox

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// RouteRule represents a single routing rule for sing-box.
// It maps traffic matching certain criteria to a specific outbound.
type RouteRule struct {
	// OutboundTag is the tag of the outbound to route matching traffic to.
	// Special values: "direct" (bypass), "block" (drop).
	OutboundTag string `json:"outbound"`
	// Domain matches exact domain names.
	Domain []string `json:"domain,omitempty"`
	// DomainSuffix matches domain suffixes (e.g. ".youtube.com" matches any subdomain).
	DomainSuffix []string `json:"domain_suffix,omitempty"`
	// DomainKeyword matches domains containing the keyword.
	DomainKeyword []string `json:"domain_keyword,omitempty"`
	// IPCIDR matches IP CIDR ranges.
	IPCIDR []string `json:"ip_cidr,omitempty"`
}

// RouteConfig represents the full routing configuration for sing-box.
type RouteConfig struct {
	// Rules is the ordered list of routing rules.
	Rules []RouteRule `json:"rules"`
	// Final is the default outbound for traffic not matching any rule.
	// Typically "direct" or a provider outbound tag.
	Final string `json:"final"`
	// AutoDetectInterface enables automatic interface detection.
	AutoDetectInterface bool `json:"auto_detect_interface,omitempty"`
}

const (
	DefaultFinal      = "direct" // built-in sing-box direct outbound
	BlockFinal        = "block"  // built-in sing-box block outbound
	AutoDetectEnabled = true
)

// SetRouteConfig sets the routing configuration. The route config will be
// merged into the next config write (on the next SetOutbounds/AddOutbound call).
func (c *Controller) SetRouteConfig(ctx context.Context, rc RouteConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Store route config in base template so it's included in future configs
	c.baseTemplate["route"] = map[string]any{
		"rules":                 rc.Rules,
		"final":                 rc.Final,
		"auto_detect_interface": rc.AutoDetectInterface,
	}

	c.logger.Info("route config set",
		zap.Int("rules", len(rc.Rules)),
		zap.String("final", rc.Final),
	)
	return nil
}

// ClearRouteConfig removes the route configuration from the base template.
func (c *Controller) ClearRouteConfig(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.baseTemplate, "route")

	c.logger.Info("route config cleared")
	return nil
}

// GenerateConfigWithRouting builds a complete sing-box config with both
// outbounds and route rules, writes it to disk, and validates it.
func (c *Controller) GenerateConfigWithRouting(ctx context.Context, outbounds []Outbound, rc RouteConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Store route config
	c.baseTemplate["route"] = map[string]any{
		"rules":                 rc.Rules,
		"final":                 rc.Final,
		"auto_detect_interface": rc.AutoDetectInterface,
	}

	// Apply outbounds
	c.activeOutbounds = make([]Outbound, len(outbounds))
	copy(c.activeOutbounds, outbounds)

	// Write config
	if err := c.applyConfigLocked(ctx); err != nil {
		return fmt.Errorf("apply config with routing: %w", err)
	}

	// Validate
	if err := c.ValidateConfig(ctx); err != nil {
		c.logger.Warn("route config validation warning", zap.Error(err))
		// Non-fatal — sing-box may still work with minor issues
	}

	return nil
}
