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

func TestGatewayRouterContract(t *testing.T) {
	h := NewGatewayHandlers(gateway.NewManager(zap.NewNop()), nil)
	for _, tc := range []struct {
		body string
		code int
	}{
		{`{"mode":"router","client_subnet":"192.168.1.0/24","router_ip":"192.168.1.1","manager_ip":"192.168.1.40","lan_interface":"eth0","router_nat":true}`, 500},
		{`{"mode":"profile","client_ip":"192.168.1.71","lan_interface":"eth0","router_nat":true}`, 400},
		{`{"mode":"router","client_subnet":"192.168.1.0/24","client_ip":"192.168.1.71","router_ip":"192.168.1.1","manager_ip":"192.168.1.40","lan_interface":"eth0"}`, 400},
		{`{"mode":"profile","client_ip":"192.168.1.71","client_subnet":"192.168.1.0/24","lan_interface":"eth0"}`, 400},
		{`{"mode":"nat","provider_name":"vpn","client_subnet":"192.168.1.0/24"}`, 400},
		{`{"mode":"router","client_subnet":"0.0.0.0/0","router_ip":"192.168.1.1","manager_ip":"192.168.1.40","lan_interface":"eth0"}`, 400},
	} {
		w := httptest.NewRecorder()
		h.Enable(w, httptest.NewRequest("POST", "/", strings.NewReader(tc.body)))
		if w.Code != tc.code {
			t.Errorf("%s got %d want %d: %s", tc.body, w.Code, tc.code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	h.Status(w, httptest.NewRequest("GET", "/", nil))
	for _, field := range []string{`"client_subnet"`, `"router_ip"`, `"manager_ip"`, `"router_nat"`} {
		if !strings.Contains(w.Body.String(), field) {
			t.Errorf("status missing %s", field)
		}
	}
}
