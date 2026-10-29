package api

import (
	"github.com/Traliaa/vpn-manager/internal/vpn/gateway"
	"go.uber.org/zap"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGatewayRejectsAmbiguousMode(t *testing.T) {
	h := NewGatewayHandlers(gateway.NewManager(zap.NewNop()), nil)
	for _, body := range []string{`{"mode":"profile","provider_name":"vpn","client_ip":"192.168.1.71","lan_interface":"eth0"}`, `{"mode":"other"}`, `{"provider_name":"vpn","client_ip":"192.168.1.71"}`, `{"mode":"profile","client_ip":"192.168.1.0/24","lan_interface":"eth0"}`} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		h.Enable(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("body %s: %d %s", body, w.Code, w.Body.String())
		}
	}
}
func TestGatewayStatusCompatibleFields(t *testing.T) {
	h := NewGatewayHandlers(gateway.NewManager(zap.NewNop()), nil)
	w := httptest.NewRecorder()
	h.Status(w, httptest.NewRequest("GET", "/", nil))
	for _, field := range []string{`"enabled"`, `"interface"`, `"mode"`, `"ready"`} {
		if !strings.Contains(w.Body.String(), field) {
			t.Fatalf("missing %s", field)
		}
	}
}
