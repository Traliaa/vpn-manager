// Package routing manages profile activation and route rule generation for sing-box.
package routing

import (
	"context"
	"fmt"
	"sync"

	"github.com/Traliaa/vpn-manager/internal/db"
	"github.com/Traliaa/vpn-manager/internal/vpn"
	"github.com/Traliaa/vpn-manager/internal/vpn/singbox"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"
)

// Activator manages the activation of routing profiles.
// It loads profile rules, converts them to sing-box route rules,
// and applies the configuration through the sing-box Controller.
type Activator struct {
	sbCtrl  *singbox.Controller
	queries *db.Queries
	manager *vpn.Manager
	logger  *zap.Logger

	applyMu         sync.Mutex
	mu              sync.RWMutex
	activeProfile   *db.Profile
	activeProfileID pgtype.UUID
}

// NewActivator creates a new profile activator.
func NewActivator(
	sbCtrl *singbox.Controller,
	queries *db.Queries,
	manager *vpn.Manager,
	logger *zap.Logger,
) *Activator {
	return &Activator{
		sbCtrl:  sbCtrl,
		queries: queries,
		manager: manager,
		logger:  logger.Named("routing-activator"),
	}
}

// Activate activates a routing profile.
// It loads the profile rules, builds a route config, and applies it via sing-box.
func (a *Activator) Activate(ctx context.Context, profileID pgtype.UUID) error {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	a.logger.Info("activating profile", zap.Stringer("profile_id", profileID))

	profile, err := a.queries.GetProfile(ctx, profileID)
	if err != nil {
		return fmt.Errorf("get profile: %w", err)
	}

	rc, err := a.buildRouteConfig(ctx, profileID)
	if err != nil {
		return fmt.Errorf("build route config: %w", err)
	}

	outbounds := a.collectOutbounds(ctx)

	if err := a.sbCtrl.GenerateConfigWithRouting(ctx, outbounds, rc); err != nil {
		return fmt.Errorf("generate config with routing: %w", err)
	}

	if err := a.sbCtrl.Reload(ctx); err != nil {
		return fmt.Errorf("reload sing-box: %w", err)
	}

	a.mu.Lock()
	a.activeProfile = &profile
	a.activeProfileID = profileID
	a.mu.Unlock()

	a.logger.Info("profile activated",
		zap.String("name", profile.Name),
		zap.Int("rules", len(rc.Rules)),
	)
	return nil
}

// Deactivate deactivates the active profile, resetting routing to direct.
func (a *Activator) Deactivate(ctx context.Context) error {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	a.mu.RLock()
	hasActive := a.activeProfile != nil
	a.mu.RUnlock()

	if !hasActive {
		return nil
	}

	rc := singbox.RouteConfig{
		Rules:               nil,
		Final:               singbox.DefaultFinal,
		AutoDetectInterface: singbox.AutoDetectEnabled,
	}

	outbounds := a.collectOutbounds(ctx)

	if err := a.sbCtrl.GenerateConfigWithRouting(ctx, outbounds, rc); err != nil {
		return fmt.Errorf("deactivate routing: %w", err)
	}

	if err := a.sbCtrl.Reload(ctx); err != nil {
		return fmt.Errorf("reload sing-box after deactivate: %w", err)
	}

	a.mu.Lock()
	a.activeProfile = nil
	a.activeProfileID = pgtype.UUID{}
	a.mu.Unlock()

	a.logger.Info("routing deactivated — all traffic goes direct")
	return nil
}

// ActiveProfile returns the active profile or nil.
func (a *Activator) ActiveProfile() *db.Profile {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.activeProfile
}

// ActiveProfileID returns the ID of the active profile.
func (a *Activator) ActiveProfileID() pgtype.UUID {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.activeProfileID
}

// buildRouteConfig builds a singbox.RouteConfig from profile rules.
func (a *Activator) buildRouteConfig(ctx context.Context, profileID pgtype.UUID) (singbox.RouteConfig, error) {
	rules, err := a.queries.ListEnabledRulesByProfile(ctx, profileID)
	if err != nil {
		return singbox.RouteConfig{}, fmt.Errorf("list rules: %w", err)
	}

	var routeRules []singbox.RouteRule
	for _, rule := range rules {
		r, err := a.buildRouteRule(ctx, rule)
		if err != nil {
			return singbox.RouteConfig{}, fmt.Errorf("rule %s: %w", rule.ID, err)
		}
		routeRules = append(routeRules, r)
	}

	return singbox.RouteConfig{
		Rules:               routeRules,
		Final:               singbox.DefaultFinal,
		AutoDetectInterface: singbox.AutoDetectEnabled,
	}, nil
}

// buildRouteRule converts a single DB rule to a singbox.RouteRule.
func (a *Activator) buildRouteRule(ctx context.Context, rule db.ListEnabledRulesByProfileRow) (singbox.RouteRule, error) {
	outboundTag, err := a.outboundTagForProvider(ctx, rule.ProviderID)
	if err != nil {
		return singbox.RouteRule{}, fmt.Errorf("outbound tag for provider: %w", err)
	}

	r, err := matchRule(rule.RuleType, rule.Value)
	if err != nil {
		return r, err
	}
	r.OutboundTag = outboundTag

	return r, nil
}

// loadResolvedIPs loads resolved IP addresses for a rule and returns them as CIDR strings.
func (a *Activator) loadResolvedIPs(ctx context.Context, ruleID pgtype.UUID) ([]string, error) {
	routes, err := a.queries.ListResolvedRoutesByRule(ctx, ruleID)
	if err != nil {
		return nil, fmt.Errorf("list resolved routes: %w", err)
	}

	ips := make([]string, 0, len(routes))
	for _, route := range routes {
		prefix := 32
		if route.Ip.Is6() {
			prefix = 128
		}
		ips = append(ips, fmt.Sprintf("%s/%d", route.Ip.String(), prefix))
	}
	return ips, nil
}

// outboundTagForProvider determines the outbound tag for a provider by its ID.
func (a *Activator) outboundTagForProvider(ctx context.Context, providerID pgtype.UUID) (string, error) {
	provider, err := a.queries.GetProvider(ctx, providerID)
	if err != nil {
		return "", fmt.Errorf("get provider: %w", err)
	}

	if tag, ok := a.manager.OutboundTag(provider.Name); ok {
		return tag, nil
	}

	return "", fmt.Errorf("provider %q has no configured outbound", provider.Name)
}

// collectOutbounds collects all sing-box outbounds from the controller.
func (a *Activator) collectOutbounds(ctx context.Context) []singbox.Outbound {
	return a.sbCtrl.ListOutbounds()
}

// ActivateDefaultProfile finds and activates the default profile.
func (a *Activator) ActivateDefaultProfile(ctx context.Context) error {
	profile, err := a.queries.GetDefaultProfile(ctx)
	if err != nil {
		return fmt.Errorf("get default profile: %w", err)
	}

	return a.Activate(ctx, profile.ID)
}

// Reapply re-applies the active profile (after rule/provider changes).
func (a *Activator) Reapply(ctx context.Context) error {
	a.mu.RLock()
	pid := a.activeProfileID
	a.mu.RUnlock()

	if pid == (pgtype.UUID{}) {
		return nil
	}

	return a.Activate(ctx, pid)
}
