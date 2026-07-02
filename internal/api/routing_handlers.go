package api

import (
	"errors"
	"net/http"

	"github.com/Traliaa/vpn-manager/internal/routing"
	"github.com/jackc/pgx/v5"
)

// RoutingHandlers предоставляет HTTP-обработчики для управления маршрутизацией.
type RoutingHandlers struct {
	activator *routing.Activator
}

// NewRoutingHandlers создаёт RoutingHandlers.
func NewRoutingHandlers(activator *routing.Activator) *RoutingHandlers {
	return &RoutingHandlers{activator: activator}
}

// ActivateProfile активирует профиль маршрутизации.
// POST /api/v1/profiles/{id}/activate
func (h *RoutingHandlers) ActivateProfile(w http.ResponseWriter, r *http.Request) {
	profileID, err := parseUUID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile id")
		return
	}

	if err := h.activator.Activate(r.Context(), profileID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "profile not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to activate profile: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "activated"})
}

// DeactivateRouting деактивирует активный профиль маршрутизации.
// POST /api/v1/routing/deactivate
func (h *RoutingHandlers) DeactivateRouting(w http.ResponseWriter, r *http.Request) {
	if err := h.activator.Deactivate(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to deactivate routing: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deactivated"})
}

// RoutingStatus возвращает информацию о текущем активном профиле.
// GET /api/v1/routing/status
func (h *RoutingHandlers) RoutingStatus(w http.ResponseWriter, r *http.Request) {
	profile := h.activator.ActiveProfile()
	if profile == nil {
		writeJSON(w, http.StatusOK, map[string]any{"active": false, "profile": nil})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"active":  true,
		"profile": profile,
	})
}

// ReapplyRouting переприменяет текущий активный профиль.
// POST /api/v1/routing/reapply
func (h *RoutingHandlers) ReapplyRouting(w http.ResponseWriter, r *http.Request) {
	if err := h.activator.Reapply(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reapply routing: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "reapplied"})
}
