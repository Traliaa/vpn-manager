package api

import (
	"encoding/json"
	"net/http"
	"net/netip"

	"github.com/Traliaa/vpn-manager/internal/vpn"
	"github.com/Traliaa/vpn-manager/internal/vpn/gateway"
)

// GatewayHandlers provides HTTP handlers for gateway mode management.
type GatewayHandlers struct {
	gw      *gateway.Manager
	manager *vpn.Manager
}

// NewGatewayHandlers creates GatewayHandlers.
func NewGatewayHandlers(gw *gateway.Manager, manager *vpn.Manager) *GatewayHandlers {
	return &GatewayHandlers{gw: gw, manager: manager}
}

// enableRequest is the JSON body for enabling gateway mode.
type enableRequest struct {
	ProviderName string `json:"provider_name"`
	Mode         string `json:"mode"`
	ClientIP     string `json:"client_ip"`
	LANInterface string `json:"lan_interface"`
}

// Enable activates gateway mode through the specified provider.
// POST /api/v1/gateway/enable
func (h *GatewayHandlers) Enable(w http.ResponseWriter, r *http.Request) {
	var req enableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Mode != "" && req.Mode != "profile" && req.Mode != "nat" {
		writeError(w, http.StatusBadRequest, "unknown gateway mode")
		return
	}
	if req.Mode == "profile" {
		ip, err := netip.ParseAddr(req.ClientIP)
		if req.ProviderName != "" || err != nil || !ip.Is4() || !ip.IsPrivate() || req.LANInterface == "" {
			writeError(w, http.StatusBadRequest, "profile mode requires one private client_ip and lan_interface, without provider_name")
			return
		}
		if err = h.gw.EnableProfile(r.Context(), gateway.ProfileConfig{ClientIP: req.ClientIP, LANInterface: req.LANInterface}); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to enable profile gateway: "+err.Error())
			return
		}
		h.Status(w, r)
		return
	}
	if req.ClientIP != "" || req.LANInterface != "" {
		writeError(w, http.StatusBadRequest, "client scope requires profile mode")
		return
	}
	if req.ProviderName == "" {
		writeError(w, http.StatusBadRequest, "provider_name is required")
		return
	}

	// Find provider in manager
	providers := h.manager.List()
	var provider vpn.Provider
	var ifaceName string
	for _, p := range providers {
		if p.Name() == req.ProviderName {
			provider = p
			ifaceName = p.InterfaceName()
			break
		}
	}
	if ifaceName == "" || provider == nil {
		writeError(w, http.StatusNotFound, "provider not found or has no interface")
		return
	}

	// Проверяем, что провайдер действительно подключён
	if status, err := provider.Status(r.Context()); err != nil || status == nil || status.State != vpn.StateUp {
		errMsg := "provider is not connected"
		if err != nil {
			errMsg += ": " + err.Error()
		} else if status != nil {
			errMsg += " (state: " + string(status.State) + ")"
		}
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}

	if err := h.gw.Enable(r.Context(), ifaceName); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to enable gateway: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "enabled",
		"interface": ifaceName,
	})
}

// Disable deactivates gateway mode.
// POST /api/v1/gateway/disable
func (h *GatewayHandlers) Disable(w http.ResponseWriter, r *http.Request) {
	if err := h.gw.Disable(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to disable gateway: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "disabled"})
}

// Status returns the current gateway mode state.
// GET /api/v1/gateway/status
func (h *GatewayHandlers) Status(w http.ResponseWriter, r *http.Request) {
	status := h.gw.Status()
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":       status.Enabled,
		"interface":     status.Interface,
		"client_ip":     status.ClientIP,
		"lan_interface": status.LANInterface,
		"ready":         status.Ready,
		"error":         status.Error,
		"mode": func() string {
			if status.ClientIP != "" {
				return "profile"
			}
			return "nat"
		}(),
	})
}

func (h *GatewayHandlers) Release(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RouterPolicyRemoved bool `json:"router_policy_removed"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || !req.RouterPolicyRemoved {
		writeError(w, http.StatusBadRequest, "confirm router_policy_removed before releasing direct fallback")
		return
	}
	if err := h.gw.Release(r.Context()); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	h.Status(w, r)
}
