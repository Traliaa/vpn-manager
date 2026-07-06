package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Traliaa/vpn-manager/internal/db"
	"github.com/Traliaa/vpn-manager/internal/vpn/service"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type Handlers struct {
	q      *db.Queries
	pool   *pgxpool.Pool
	logger *zap.Logger
	svc    *service.Service
}

func NewHandlers(q *db.Queries, pool *pgxpool.Pool, logger *zap.Logger, svc *service.Service) *Handlers {
	return &Handlers{q: q, pool: pool, logger: logger, svc: svc}
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func parseUUID(s string) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := id.Scan(s)
	return id, err
}

// ========================================================================
// Providers
// ========================================================================

type createProviderRequest struct {
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"`
	Config       any    `json:"config"`
	Enabled      *bool  `json:"enabled,omitempty"`
	Priority     *int32 `json:"priority,omitempty"`
	HealthHost   string `json:"health_host,omitempty"`
}

func (h *Handlers) ListProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := h.q.ListProviders(r.Context())
	if err != nil {
		h.logger.Error("list providers", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to list providers")
		return
	}
	if providers == nil {
		providers = []db.VpnProvider{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": providers})
}

func (h *Handlers) CreateProvider(w http.ResponseWriter, r *http.Request) {
	var req createProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.ProviderType == "" {
		writeError(w, http.StatusBadRequest, "name and provider_type are required")
		return
	}

	cfgStr := "{}"
	if req.Config != nil {
		data, err := json.Marshal(req.Config)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid config")
			return
			cfgStr = string(data)
		}
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	priority := int32(100)
	if req.Priority != nil {
		priority = *req.Priority
	}

	provider, err := h.q.CreateProvider(r.Context(), db.CreateProviderParams{
		Name:         req.Name,
		ProviderType: db.ProviderType(req.ProviderType),
		Config:       cfgStr,
		Enabled:      enabled,
		Priority:     priority,
		HealthHost:   pgtype.Text{String: req.HealthHost, Valid: req.HealthHost != ""},
	})
	if err != nil {
		h.logger.Error("create provider", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to create provider")
		return
	}

	h.audit(r.Context(), "create", "vpn_providers", provider.ID, map[string]any{
		"name": req.Name, "type": req.ProviderType,
	})
	writeJSON(w, http.StatusCreated, provider)
}

func (h *Handlers) GetProvider(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid provider id")
		return
	}

	provider, err := h.q.GetProvider(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "provider not found")
			return
		}
		h.logger.Error("get provider", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to get provider")
		return
	}
	writeJSON(w, http.StatusOK, provider)
}

func (h *Handlers) UpdateProvider(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid provider id")
		return
	}

	var req struct {
		Name         string `json:"name,omitempty"`
		ProviderType string `json:"provider_type,omitempty"`
		Config       any    `json:"config,omitempty"`
		Enabled      *bool  `json:"enabled,omitempty"`
		Priority     *int32 `json:"priority,omitempty"`
		HealthHost   string `json:"health_host,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	current, err := h.q.GetProvider(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "provider not found")
			return
		}
		h.logger.Error("get provider for update", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to get provider")
		return
	}

	name := current.Name
	if req.Name != "" {
		name = req.Name
	}
	ptype := current.ProviderType
	if req.ProviderType != "" {
		ptype = db.ProviderType(req.ProviderType)
	}
	cfgStr := current.Config
	if req.Config != nil {
		data, err := json.Marshal(req.Config)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid config")
			return
			cfgStr = string(data)
		}
	}
	enabled := current.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	priority := current.Priority
	if req.Priority != nil {
		priority = *req.Priority
	}
	healthHost := current.HealthHost
	if req.HealthHost != "" {
		healthHost = pgtype.Text{String: req.HealthHost, Valid: true}
	}

	provider, err := h.q.UpdateProvider(r.Context(), db.UpdateProviderParams{
		ID: id, Name: name, ProviderType: ptype,
		Config: cfgStr, Enabled: enabled, Priority: priority,
		HealthHost: healthHost,
	})
	if err != nil {
		h.logger.Error("update provider", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to update provider")
		return
	}

	h.audit(r.Context(), "update", "vpn_providers", id, nil)
	if enabled && !current.Enabled {
		if err := h.svc.SyncProviders(r.Context()); err != nil {
			h.logger.Warn("sync after enable", zap.Error(err))
		}
	}
	writeJSON(w, http.StatusOK, provider)
}

func (h *Handlers) DeleteProvider(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid provider id")
		return
	}

	if err := h.q.DeleteProvider(r.Context(), id); err != nil {
		h.logger.Error("delete provider", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to delete provider")
		return
	}

	h.audit(r.Context(), "delete", "vpn_providers", id, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ========================================================================
// Profiles
// ========================================================================

func (h *Handlers) ListProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.q.ListProfiles(r.Context())
	if err != nil {
		h.logger.Error("list profiles", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to list profiles")
		return
	}
	if profiles == nil {
		profiles = []db.Profile{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": profiles})
}

func (h *Handlers) CreateProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
		IsDefault   bool   `json:"is_default,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	if req.IsDefault {
		_ = h.q.ResetDefaultProfile(r.Context())
	}

	profile, err := h.q.CreateProfile(r.Context(), db.CreateProfileParams{
		Name:        req.Name,
		Description: pgtype.Text{String: req.Description, Valid: req.Description != ""},
		IsDefault:   req.IsDefault,
	})
	if err != nil {
		h.logger.Error("create profile", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to create profile")
		return
	}

	h.audit(r.Context(), "create", "profiles", profile.ID, nil)
	writeJSON(w, http.StatusCreated, profile)
}

func (h *Handlers) GetProfile(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile id")
		return
	}

	profile, err := h.q.GetProfile(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "profile not found")
			return
		}
		h.logger.Error("get profile", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to get profile")
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (h *Handlers) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile id")
		return
	}

	var req struct {
		Name        string `json:"name,omitempty"`
		Description string `json:"description,omitempty"`
		IsDefault   *bool  `json:"is_default,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	current, err := h.q.GetProfile(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "profile not found")
			return
		}
		h.logger.Error("get profile for update", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to get profile")
		return
	}

	name := current.Name
	if req.Name != "" {
		name = req.Name
	}
	desc := current.Description
	if req.Description != "" {
		desc = pgtype.Text{String: req.Description, Valid: true}
	}
	isDefault := current.IsDefault
	if req.IsDefault != nil && *req.IsDefault {
		_ = h.q.ResetDefaultProfile(r.Context())
		isDefault = true
	} else if req.IsDefault != nil && !*req.IsDefault {
		isDefault = false
	}

	profile, err := h.q.UpdateProfile(r.Context(), db.UpdateProfileParams{
		ID: id, Name: name, Description: desc, IsDefault: isDefault,
	})
	if err != nil {
		h.logger.Error("update profile", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to update profile")
		return
	}

	h.audit(r.Context(), "update", "profiles", id, nil)
	writeJSON(w, http.StatusOK, profile)
}

func (h *Handlers) DeleteProfile(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile id")
		return
	}

	if err := h.q.DeleteProfile(r.Context(), id); err != nil {
		h.logger.Error("delete profile", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to delete profile")
		return
	}

	h.audit(r.Context(), "delete", "profiles", id, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ========================================================================
// Rules
// ========================================================================

func (h *Handlers) ListRules(w http.ResponseWriter, r *http.Request) {
	profileID, err := parseUUID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile id")
		return
	}

	rules, err := h.q.ListRulesByProfile(r.Context(), profileID)
	if err != nil {
		h.logger.Error("list rules", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to list rules")
		return
	}
	if rules == nil {
		rules = []db.RoutingRule{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"rules": rules})
}

func (h *Handlers) CreateRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProfileID   string `json:"profile_id"`
		ProviderID  string `json:"provider_id,omitempty"`
		RuleType    string `json:"rule_type"`
		Value       string `json:"value"`
		Enabled     *bool  `json:"enabled,omitempty"`
		Priority    *int32 `json:"priority,omitempty"`
		Description string `json:"description,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ProfileID == "" || req.RuleType == "" || req.Value == "" {
		writeError(w, http.StatusBadRequest, "profile_id, rule_type, and value are required")
		return
	}

	profileUUID, err := parseUUID(req.ProfileID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile_id")
		return
	}
	var providerUUID pgtype.UUID
	if req.ProviderID != "" {
		providerUUID, err = parseUUID(req.ProviderID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid provider_id")
			return
		}
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	priority := int32(500)
	if req.Priority != nil {
		priority = *req.Priority
	}

	rule, err := h.q.CreateRule(r.Context(), db.CreateRuleParams{
		ProfileID: profileUUID, ProviderID: providerUUID,
		RuleType: db.RuleType(req.RuleType), Value: req.Value,
		Enabled: enabled, Priority: priority,
		Description: pgtype.Text{String: req.Description, Valid: req.Description != ""},
	})
	if err != nil {
		h.logger.Error("create rule", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to create rule")
		return
	}

	h.audit(r.Context(), "create", "routing_rules", rule.ID, nil)
	writeJSON(w, http.StatusCreated, rule)
}

func (h *Handlers) GetRule(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid rule id")
		return
	}

	rule, err := h.q.GetRule(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "rule not found")
			return
		}
		h.logger.Error("get rule", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to get rule")
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (h *Handlers) UpdateRule(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid rule id")
		return
	}

	var req struct {
		ProviderID  string `json:"provider_id,omitempty"`
		RuleType    string `json:"rule_type,omitempty"`
		Value       string `json:"value,omitempty"`
		Enabled     *bool  `json:"enabled,omitempty"`
		Priority    *int32 `json:"priority,omitempty"`
		Description string `json:"description,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	current, err := h.q.GetRule(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "rule not found")
			return
		}
		h.logger.Error("get rule for update", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to get rule")
		return
	}

	providerID := current.ProviderID
	if req.ProviderID != "" {
		providerID, err = parseUUID(req.ProviderID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid provider_id")
			return
		}
	}
	ruleType := current.RuleType
	if req.RuleType != "" {
		ruleType = db.RuleType(req.RuleType)
	}
	value := current.Value
	if req.Value != "" {
		value = req.Value
	}
	enabled := current.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	priority := current.Priority
	if req.Priority != nil {
		priority = *req.Priority
	}
	desc := current.Description
	if req.Description != "" {
		desc = pgtype.Text{String: req.Description, Valid: true}
	}

	rule, err := h.q.UpdateRule(r.Context(), db.UpdateRuleParams{
		ID: id, ProviderID: providerID, RuleType: ruleType,
		Value: value, Enabled: enabled, Priority: priority,
		Description: desc,
	})
	if err != nil {
		h.logger.Error("update rule", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to update rule")
		return
	}

	h.audit(r.Context(), "update", "routing_rules", id, nil)
	writeJSON(w, http.StatusOK, rule)
}

func (h *Handlers) DeleteRule(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid rule id")
		return
	}

	if err := h.q.DeleteRule(r.Context(), id); err != nil {
		h.logger.Error("delete rule", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to delete rule")
		return
	}

	h.audit(r.Context(), "delete", "routing_rules", id, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ========================================================================
// Interfaces
// ========================================================================

func (h *Handlers) ListInterfaces(w http.ResponseWriter, r *http.Request) {
	ifaces, err := h.q.ListInterfaces(r.Context())
	if err != nil {
		h.logger.Error("list interfaces", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to list interfaces")
		return
	}
	if ifaces == nil {
		ifaces = []db.Interface{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"interfaces": ifaces})
}

func (h *Handlers) GetInterface(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "interface name is required")
		return
	}

	iface, err := h.q.GetInterfaceByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "interface not found")
			return
		}
		h.logger.Error("get interface", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to get interface")
		return
	}
	writeJSON(w, http.StatusOK, iface)
}

// ========================================================================
// Health Checks
// ========================================================================

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	err := h.pool.Ping(ctx)
	status := http.StatusOK
	resp := map[string]any{"status": "ok", "database": "connected"}
	if err != nil {
		status = http.StatusServiceUnavailable
		resp["status"] = "degraded"
		resp["database"] = err.Error()
	}
	writeJSON(w, status, resp)
}

func (h *Handlers) ListHealthChecks(w http.ResponseWriter, r *http.Request) {
	providerID, err := parseUUID(r.PathValue("providerId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid provider id")
		return
	}

	checks, err := h.q.ListHealthChecks(r.Context(), db.ListHealthChecksParams{
		ProviderID: providerID,
		Limit:      50,
	})
	if err != nil {
		h.logger.Error("list health checks", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to list health checks")
		return
	}
	if checks == nil {
		checks = []db.HealthCheck{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"health_checks": checks})
}

// ========================================================================
// Audit log helper
// ========================================================================

func (h *Handlers) audit(ctx context.Context, action, entityType string, entityID pgtype.UUID, payload any) {
	var payloadBytes []byte
	if payload != nil {
		var err error
		payloadBytes, err = json.Marshal(payload)
		if err != nil {
			h.logger.Warn("failed to marshal audit payload", zap.Error(err))
			return
		}
	}

	if _, err := h.q.CreateAuditLog(ctx, db.CreateAuditLogParams{
		Action: action, EntityType: entityType,
		EntityID: entityID, Payload: payloadBytes,
	}); err != nil {
		h.logger.Warn("failed to write audit log", zap.Error(err))
	}
}
