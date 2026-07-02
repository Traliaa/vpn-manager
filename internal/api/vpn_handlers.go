package api

import (
	"net/http"

	"github.com/Traliaa/vpn-manager/internal/vpn"
	"github.com/Traliaa/vpn-manager/internal/vpn/service"
)

// VpnHandlers предоставляет HTTP-обработчики для управления VPN.
type VpnHandlers struct {
	manager *vpn.Manager
	svc     *service.Service
}

// NewVpnHandlers создаёт VpnHandlers.
func NewVpnHandlers(manager *vpn.Manager, svc *service.Service) *VpnHandlers {
	return &VpnHandlers{manager: manager, svc: svc}
}

// ListInterfaces возвращает статусы всех зарегистрированных VPN-интерфейсов.
func (h *VpnHandlers) ListInterfaces(w http.ResponseWriter, r *http.Request) {
	statuses := h.manager.AllStatuses(r.Context())
	if statuses == nil {
		statuses = []*vpn.InterfaceStatus{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"interfaces": statuses})
}

// SyncProviders синхронизирует провайдеров из БД с менеджером.
func (h *VpnHandlers) SyncProviders(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.SyncProviders(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to sync providers: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "synced"})
}
