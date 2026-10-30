package singbox

import (
	"context"
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
	base := cloneTemplate(c.baseTemplate)
	base["route"] = routeTemplate(rc)
	return c.updateLocked(ctx, c.activeOutbounds, base)
}
func (c *Controller) ClearRouteConfig(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	base := cloneTemplate(c.baseTemplate)
	delete(base, "route")
	return c.updateLocked(ctx, c.activeOutbounds, base)
}
func (c *Controller) GenerateConfigWithRouting(ctx context.Context, outbounds []Outbound, rc RouteConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	base := cloneTemplate(c.baseTemplate)
	base["route"] = routeTemplate(rc)
	return c.updateLocked(ctx, outbounds, base)
}
func routeTemplate(rc RouteConfig) map[string]any {
	return map[string]any{"rules": rc.Rules, "final": rc.Final, "auto_detect_interface": rc.AutoDetectInterface}
}
