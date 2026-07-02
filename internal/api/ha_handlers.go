package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Traliaa/vpn-manager/internal/db"
	"github.com/Traliaa/vpn-manager/internal/routing"
	"github.com/Traliaa/vpn-manager/internal/vpn"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"
)

// HaHandlers provides REST endpoints for Home Assistant integration.
type HaHandlers struct {
	manager   *vpn.Manager
	activator *routing.Activator
	queries   *db.Queries
	logger    *zap.Logger
}

// NewHaHandlers creates a new HaHandlers instance.
func NewHaHandlers(
	manager *vpn.Manager,
	activator *routing.Activator,
	queries *db.Queries,
	logger *zap.Logger,
) *HaHandlers {
	return &HaHandlers{
		manager:   manager,
		activator: activator,
		queries:   queries,
		logger:    logger.Named("ha-handlers"),
	}
}

// haProviderStatus is a simplified provider status for HA JSON output.
type haProviderStatus struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	State         string `json:"state"`
	UptimeSeconds int64  `json:"uptime_seconds,omitempty"`
	TxBytes       int64  `json:"tx_bytes,omitempty"`
	RxBytes       int64  `json:"rx_bytes,omitempty"`
}

// haRoutingStatus represents the current routing state for HA.
type haRoutingStatus struct {
	Active      bool   `json:"active"`
	ProfileName string `json:"profile_name,omitempty"`
	ProfileID   string `json:"profile_id,omitempty"`
}

// haStatusResponse is the full status payload returned by GET /ha/status.
type haStatusResponse struct {
	Routing   haRoutingStatus    `json:"routing"`
	Providers []haProviderStatus `json:"providers"`
	Total     int                `json:"total_providers"`
	UpCount   int                `json:"up_providers"`
}

// Status returns the full VPN and routing status for Home Assistant sensors.
// GET /api/v1/ha/status
func (h *HaHandlers) Status(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	statuses := h.manager.AllStatuses(ctx)

	providers := make([]haProviderStatus, 0, len(statuses))
	upCount := 0

	for _, s := range statuses {
		uptime := int64(0)
		if s.State == vpn.StateUp && s.Uptime > 0 {
			uptime = int64(s.Uptime.Seconds())
			upCount++
		}

		providers = append(providers, haProviderStatus{
			Name:          s.Name,
			Type:          string(s.Type),
			State:         string(s.State),
			UptimeSeconds: uptime,
			TxBytes:       s.TxBytes,
			RxBytes:       s.RxBytes,
		})
	}

	routingStatus := haRoutingStatus{Active: false}
	if profile := h.activator.ActiveProfile(); profile != nil {
		routingStatus.Active = true
		routingStatus.ProfileName = profile.Name
		routingStatus.ProfileID = formatUUID(profile.ID)
	}

	resp := haStatusResponse{
		Routing:   routingStatus,
		Providers: providers,
		Total:     len(providers),
		UpCount:   upCount,
	}

	writeJSON(w, http.StatusOK, resp)
}

// Activate activates a routing profile by UUID.
// POST /api/v1/ha/activate/{id}
func (h *HaHandlers) Activate(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, "missing profile id")
		return
	}

	profileID, err := parseUUID(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile id")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if err := h.activator.Activate(ctx, profileID); err != nil {
		h.logger.Error("ha activate failed",
			zap.String("profile_id", idStr),
			zap.Error(err),
		)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "profile activated",
	})
}

// formatUUID returns the string representation of a pgtype.UUID.
func formatUUID(id pgtype.UUID) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		id.Bytes[0:4], id.Bytes[4:6], id.Bytes[6:8], id.Bytes[8:10], id.Bytes[10:16])
}
