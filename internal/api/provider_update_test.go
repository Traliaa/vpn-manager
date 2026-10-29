package api

import (
	"context"
	"errors"
	"github.com/Traliaa/vpn-manager/internal/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

type providerDB struct{ value db.VpnProvider }
type errorRow struct{}

func (errorRow) Scan(...any) error { return errors.New("audit unavailable") }

type providerRow struct{ value db.VpnProvider }

func (r providerRow) Scan(dst ...any) error {
	values := []any{r.value.ID, r.value.Name, r.value.ProviderType, r.value.Config, r.value.Enabled, r.value.Priority, r.value.HealthHost, r.value.CreatedAt, r.value.UpdatedAt}
	for i, d := range dst {
		reflect.ValueOf(d).Elem().Set(reflect.ValueOf(values[i]))
	}
	return nil
}
func (d *providerDB) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	if strings.Contains(sql, "-- name: CreateAuditLog") {
		return errorRow{}
	}
	if strings.Contains(sql, "-- name: UpdateProvider") {
		d.value.Name = args[1].(string)
		d.value.ProviderType = args[2].(db.ProviderType)
		d.value.Config = args[3].(string)
		d.value.Enabled = args[4].(bool)
		d.value.Priority = args[5].(int32)
		d.value.HealthHost = args[6].(pgtype.Text)
	}
	return providerRow{d.value}
}
func (d *providerDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("unexpected query")
}
func (d *providerDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

type syncProbe struct {
	calls int
	err   error
}

func (s *syncProbe) SyncProviders(context.Context) error { s.calls++; return s.err }
func TestProviderConfigUpdatePersistsAndReconciles(t *testing.T) {
	d := &providerDB{value: db.VpnProvider{Name: "vpn", Config: `{"old":true}`, Enabled: true}}
	sync := &syncProbe{}
	h := &Handlers{q: db.New(d), svc: sync, logger: zap.NewNop()}
	req := httptest.NewRequest("PUT", "/", strings.NewReader(`{"config":{"new":true}}`))
	req.SetPathValue("id", "11111111-1111-1111-1111-111111111111")
	w := httptest.NewRecorder()
	h.UpdateProvider(w, req)
	if w.Code != 200 || d.value.Config != `{"new":true}` || sync.calls != 1 {
		t.Fatalf("code %d config %s sync %d", w.Code, d.value.Config, sync.calls)
	}
}
func TestProviderApplyFailureIsVisible(t *testing.T) {
	d := &providerDB{value: db.VpnProvider{Name: "vpn", Config: `{}`, Enabled: true}}
	sync := &syncProbe{err: errors.New("cannot start provider")}
	h := &Handlers{q: db.New(d), svc: sync, logger: zap.NewNop()}
	req := httptest.NewRequest("PUT", "/", strings.NewReader(`{"enabled":false}`))
	req.SetPathValue("id", "11111111-1111-1111-1111-111111111111")
	w := httptest.NewRecorder()
	h.UpdateProvider(w, req)
	if d.value.Enabled != true {
		t.Fatal("failed apply persisted disabled configuration")
	}
	if w.Code < 400 || sync.calls < 1 {
		t.Fatalf("failed apply reported success: %d", w.Code)
	}
}
