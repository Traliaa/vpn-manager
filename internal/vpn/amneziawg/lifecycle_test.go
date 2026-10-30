package amneziawg

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"time"

	"github.com/advanced-wg/awgctrl-go/wgtypes"
	"go.uber.org/zap"
)

type fakeLinks struct {
	exists                  bool
	creates, removes        int
	createErr, configureErr error
}

func (l *fakeLinks) Exists(string) (bool, error) { return l.exists, nil }
func (l *fakeLinks) Create(string) error {
	l.creates++
	if l.createErr != nil {
		return l.createErr
	}
	l.exists = true
	return nil
}
func (l *fakeLinks) Configure(string, string, int) error { return l.configureErr }
func (l *fakeLinks) Remove(string) error                 { l.removes++; l.exists = false; return nil }

type fakeDevice struct {
	config       wgtypes.Config
	configureErr error
	device       *wgtypes.Device
}

func (d *fakeDevice) ConfigureDevice(_ context.Context, _ string, c wgtypes.Config) error {
	d.config = c
	return d.configureErr
}
func (d *fakeDevice) Device(context.Context, string) (*wgtypes.Device, error) { return d.device, nil }
func (d *fakeDevice) Close() error                                            { return nil }

type fakeProcess struct {
	alive         bool
	starts, stops int
	links         *fakeLinks
}

func (p *fakeProcess) Start(_ context.Context, _ string, _ []string, ready func() bool) error {
	p.starts++
	p.alive = true
	p.links.exists = true
	if !ready() {
		return errors.New("not ready")
	}
	return nil
}
func (p *fakeProcess) Stop(context.Context) error {
	p.stops++
	p.alive = false
	p.links.exists = false
	return nil
}
func (p *fakeProcess) Running() bool { return p.alive }
func controllerFixture(t *testing.T) (*Controller, *fakeLinks, *fakeDevice, *fakeProcess) {
	t.Helper()
	l := &fakeLinks{}
	d := &fakeDevice{device: &wgtypes.Device{Peers: []wgtypes.Peer{{LastHandshakeTime: time.Now()}}}}
	p := &fakeProcess{links: l}
	c, err := NewController("test userspace", validTestConfig(), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	c.client.Close()
	c.client = d
	c.links = l
	c.process = p
	c.newClient = func() (deviceClient, error) { return d, nil }
	t.Cleanup(func() { c.Close() })
	return c, l, d, p
}
func TestUserspaceFallbackAndFullDeviceConfig(t *testing.T) {
	c, l, d, p := controllerFixture(t)
	l.createErr = syscall.EOPNOTSUPP
	cfg := validTestConfig()
	cfg.H1 = "100-200"
	cfg.H2 = "300"
	cfg.H3 = "400"
	cfg.H4 = "500"
	cfg.InitJunkPackets = 105
	cfg.ResponseJunkPackets = 97
	cfg.JunkPacketCount = 3
	cfg.TransportHeader = "<b 0x0102>"
	if err := c.ApplyConfig(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if err := c.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.starts != 1 || !p.alive {
		t.Fatal("userspace process not started")
	}
	if d.config.H1 == nil || *d.config.H1 != "100-200" || d.config.Jc == nil || *d.config.Jc != 3 || d.config.S1 == nil || *d.config.S1 != 105 || d.config.I1 == nil || *d.config.I1 != "<b 0x0102>" {
		t.Fatal("obfuscation omitted from device configuration")
	}
	if err := c.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.starts != 1 {
		t.Fatal("repeated Connect spawned duplicate process")
	}
	p.alive = false
	s, err := c.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.State == "up" {
		t.Fatal("dead userspace process reported up")
	}
}
func TestConfigureFailureCleansOnlyOwnedResources(t *testing.T) {
	c, l, d, p := controllerFixture(t)
	l.createErr = syscall.EOPNOTSUPP
	d.configureErr = errors.New("configuration failed")
	if err := c.Connect(context.Background()); err == nil {
		t.Fatal("configuration failure ignored")
	}
	if p.alive || l.exists {
		t.Fatal("partially configured interface/process leaked")
	}
}
func TestMalformedConfigDoesNotTouchNetwork(t *testing.T) {
	c, l, _, p := controllerFixture(t)
	c.cfg.PrivateKey = "invalid"
	if err := c.Connect(context.Background()); err == nil {
		t.Fatal("invalid config connected")
	}
	if l.creates != 0 || l.removes != 0 || p.starts != 0 {
		t.Fatal("invalid config modified network")
	}
}
func TestExistingForeignInterfaceIsNotRemoved(t *testing.T) {
	c, l, _, p := controllerFixture(t)
	l.exists = true
	if err := c.Connect(context.Background()); err == nil {
		t.Fatal("existing unowned interface accepted")
	}
	if l.removes != 0 || p.starts != 0 {
		t.Fatal("foreign interface modified")
	}
}
func TestDisconnectCanReconnect(t *testing.T) {
	c, l, _, p := controllerFixture(t)
	l.createErr = syscall.EOPNOTSUPP
	for i := 0; i < 2; i++ {
		if err := c.Connect(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := c.Disconnect(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if p.starts != 2 || p.alive || l.exists {
		t.Fatal("reconnect/cleanup failed")
	}
}

func TestMissingOrStaleHandshakeIsNotHealthy(t *testing.T) {
	for _, when := range []time.Time{time.Time{}, time.Now().Add(-10 * time.Minute)} {
		c, _, d, _ := controllerFixture(t)
		d.device.Peers[0].LastHandshakeTime = when
		if err := c.Connect(context.Background()); err != nil {
			t.Fatal(err)
		}
		s, err := c.Status(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if s.State == "up" {
			t.Fatal("missing or stale handshake reported as ready")
		}
		if err := c.HealthCheck(context.Background()); err == nil {
			t.Fatal("unhealthy tunnel passed health check")
		}
	}
}

func TestHealthCheckRecoversOwnedDeadUserspaceButNotDisconnectedProvider(t *testing.T) {
	c, l, _, p := controllerFixture(t)
	l.createErr = syscall.EOPNOTSUPP
	ctx := context.Background()
	if err := c.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	p.alive = false
	l.exists = false
	if err := c.HealthCheck(ctx); err != nil {
		t.Fatal(err)
	}
	if p.starts != 2 {
		t.Fatal("dead owned userspace not restarted")
	}
	if err := c.Disconnect(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.HealthCheck(ctx); err == nil {
		t.Fatal("disconnected provider reported ready")
	}
	if p.starts != 2 {
		t.Fatal("health check resurrected disconnected provider")
	}
}
