package amneziawg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/Traliaa/vpn-manager/internal/vpn"
	"github.com/Traliaa/vpn-manager/internal/vpn/managedprocess"
	"github.com/advanced-wg/awgctrl-go"
	"github.com/advanced-wg/awgctrl-go/wgtypes"
	"go.uber.org/zap"
)

// Config представляет конфигурацию AmneziaWG.
type Config struct {
	ListenPort          int        `json:"listen_port,omitempty"`
	TransportPadding    int        `json:"s4,omitempty"`
	H1                  string     `json:"h1,omitempty"`
	H2                  string     `json:"h2,omitempty"`
	H3                  string     `json:"h3,omitempty"`
	H4                  string     `json:"h4,omitempty"`
	I2                  string     `json:"i2,omitempty"`
	I3                  string     `json:"i3,omitempty"`
	I4                  string     `json:"i4,omitempty"`
	I5                  string     `json:"i5,omitempty"`
	PrivateKey          string     `json:"private_key"`
	Address             string     `json:"address"`
	DNS                 string     `json:"dns,omitempty"`
	MTU                 int        `json:"mtu,omitempty"`
	JunkPacketCount     int        `json:"junk_packet_count,omitempty"`
	JunkPacketMinSize   int        `json:"junk_packet_min_size,omitempty"`
	JunkPacketMaxSize   int        `json:"junk_packet_max_size,omitempty"`
	InitJunkPackets     int        `json:"init_junk_packets,omitempty"`
	ResponseJunkPackets int        `json:"response_junk_packets,omitempty"`
	TransportHeader     string     `json:"transport_header,omitempty"`
	TransportPacketLen  int        `json:"transport_packet_len,omitempty"`
	Peer                PeerConfig `json:"peer"`
}

// PeerConfig представляет конфигурацию удалённого пира.
type PeerConfig struct {
	PublicKey           string   `json:"public_key"`
	PresharedKey        string   `json:"preshared_key,omitempty"`
	Endpoint            string   `json:"endpoint"`
	AllowedIPs          []string `json:"allowed_ips"`
	PersistentKeepalive int      `json:"persistent_keepalive,omitempty"`
}

type deviceClient interface {
	ConfigureDevice(context.Context, string, wgtypes.Config) error
	Device(context.Context, string) (*wgtypes.Device, error)
	Close() error
}
type linkManager interface {
	Exists(string) (bool, error)
	Create(string) error
	Configure(string, string, int) error
	Remove(string) error
}
type serviceProcess interface {
	Start(context.Context, string, []string, func() bool) error
	Stop(context.Context) error
	Running() bool
}

// Controller serializes configuration and owns only the interface/process it creates.
type Controller struct {
	mu               sync.Mutex
	name, ifaceName  string
	cfg              Config
	logger           *zap.Logger
	client           deviceClient
	newClient        func() (deviceClient, error)
	links            linkManager
	process          serviceProcess
	owned, userspace bool
}

func NewController(name string, cfg Config, logger *zap.Logger) (*Controller, error) {
	factory := func() (deviceClient, error) { return wgctrl.New() }
	client, err := factory()
	if err != nil {
		return nil, fmt.Errorf("create AWG client: %w", err)
	}
	return &Controller{name: name, ifaceName: sanitizeIfaceName(name, "awg"), cfg: cfg, logger: logger.With(zap.String("interface", name)), client: client, newClient: factory, links: &linuxLinks{}, process: &managedprocess.Process{}}, nil
}
func (c *Controller) Type() vpn.ProviderType { return vpn.ProviderAmneziaWG }
func (c *Controller) Name() string           { return c.name }
func (c *Controller) InterfaceName() string  { return c.ifaceName }

// sanitizeIfaceName создаёт корректное Linux-имя интерфейса из произвольной строки.
func sanitizeIfaceName(name, prefix string) string {
	sum := sha256.Sum256([]byte(name))
	return prefix + "_" + hex.EncodeToString(sum[:5])
}

// ApplyConfig парсит, валидирует и сохраняет конфигурацию.
// Не создаёт и не изменяет интерфейс Linux — это делает Connect().
func (c *Controller) ApplyConfig(ctx context.Context, cfg interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.owned {
		return fmt.Errorf("disconnect provider before changing configuration")
	}
	next := c.cfg
	switch v := cfg.(type) {
	case Config:
		next = v
	case *Config:
		if v == nil {
			return fmt.Errorf("nil AmneziaWG config")
		}
		next = *v
	default:
		data, err := json.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("invalid config type: %T", cfg)
		}
		if err := json.Unmarshal(data, &next); err != nil {
			return fmt.Errorf("unmarshal awg config: %w", err)
		}
		// Если peer-поля пустые — пробуем плоский формат
		if next.Peer.Endpoint == "" && next.Peer.PublicKey == "" {
			var flat struct {
				PublicKey  string   `json:"public_key"`
				Endpoint   string   `json:"endpoint"`
				AllowedIPs []string `json:"allowed_ips"`
			}
			if err := json.Unmarshal(data, &flat); err == nil && flat.PublicKey != "" {
				next.Peer.PublicKey = flat.PublicKey
				if flat.Endpoint != "" {
					next.Peer.Endpoint = flat.Endpoint
				}
				if len(flat.AllowedIPs) > 0 {
					next.Peer.AllowedIPs = flat.AllowedIPs
				}
				c.logger.Debug("migrated flat config to nested peer format")
			}
		}
	}

	if next.PrivateKey == "" || next.Peer.PublicKey == "" {
		return fmt.Errorf("incomplete config: missing private_key or peer.public_key")
	}

	if err := validateConfig(next); err != nil {
		return err
	}
	c.cfg = next
	c.logger.Info("config validated",
		zap.String("iface", c.ifaceName),
		zap.String("endpoint", next.Peer.Endpoint),
		zap.Strings("allowed_ips", next.Peer.AllowedIPs),
	)
	return nil
}

// Connect validates completely before performing any network mutation.
func (c *Controller) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connectLocked(ctx)
}
func (c *Controller) connectLocked(ctx context.Context) (retErr error) {
	if err := validateConfig(c.cfg); err != nil {
		return err
	}
	deviceCfg, err := buildDeviceConfig(ctx, c.cfg)
	if err != nil {
		return err
	}
	if c.owned {
		exists, err := c.links.Exists(c.ifaceName)
		if err != nil {
			return err
		}
		if exists && (!c.userspace || c.process.Running()) {
			return nil
		}
		if err := c.disconnectLocked(ctx); err != nil {
			return err
		}
	}
	exists, err := c.links.Exists(c.ifaceName)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("interface %s already exists and is not owned by this controller", c.ifaceName)
	}
	if c.client == nil {
		c.client, err = c.newClient()
		if err != nil {
			return fmt.Errorf("create AWG client: %w", err)
		}
	}
	success := false
	defer func() {
		if !success {
			cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			retErr = errors.Join(retErr, c.disconnectLocked(cleanup))
		}
	}()
	if err := c.links.Create(c.ifaceName); err != nil {
		if !errors.Is(err, syscall.EOPNOTSUPP) && !errors.Is(err, syscall.EPROTONOSUPPORT) && !errors.Is(err, syscall.ENOENT) {
			return fmt.Errorf("create AWG interface: %w", err)
		}
		c.userspace = true
		binary := os.Getenv("AMNEZIAWG_GO_BINARY")
		if binary == "" {
			binary = "/usr/local/bin/amneziawg-go"
		}
		if err := c.process.Start(ctx, binary, []string{"-f", c.ifaceName}, func() bool { ok, err := c.links.Exists(c.ifaceName); return err == nil && ok }); err != nil {
			return fmt.Errorf("start userspace AWG: %w", err)
		}
	}
	c.owned = true
	if err := c.client.ConfigureDevice(ctx, c.ifaceName, deviceCfg); err != nil {
		return fmt.Errorf("configure AWG device: %w", err)
	}
	mtu := c.cfg.MTU
	if mtu == 0 {
		mtu = 1420
	}
	if err := c.links.Configure(c.ifaceName, c.cfg.Address, mtu); err != nil {
		return err
	}
	success = true
	c.logger.Info("AmneziaWG interface configured", zap.Bool("userspace", c.userspace))
	return nil
}
func buildDeviceConfig(ctx context.Context, cfg Config) (wgtypes.Config, error) {
	if err := validateConfig(cfg); err != nil {
		return wgtypes.Config{}, err
	}
	private, _ := wgtypes.ParseKey(cfg.PrivateKey)
	public, _ := wgtypes.ParseKey(cfg.Peer.PublicKey)
	allowed := make([]net.IPNet, 0, len(cfg.Peer.AllowedIPs))
	for _, raw := range cfg.Peer.AllowedIPs {
		p, _ := netip.ParsePrefix(raw)
		p = p.Masked()
		allowed = append(allowed, net.IPNet{IP: p.Addr().AsSlice(), Mask: net.CIDRMask(p.Bits(), p.Addr().BitLen())})
	}
	host, port, _ := net.SplitHostPort(cfg.Peer.Endpoint)
	number, _ := strconv.Atoi(port)
	resolve, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupNetIP(resolve, "ip", host)
	if err != nil {
		return wgtypes.Config{}, fmt.Errorf("resolve VPN endpoint: %w", err)
	}
	if len(ips) == 0 {
		return wgtypes.Config{}, fmt.Errorf("VPN endpoint has no addresses")
	}
	keepalive := time.Duration(cfg.Peer.PersistentKeepalive) * time.Second
	peer := wgtypes.PeerConfig{PublicKey: public, Endpoint: net.UDPAddrFromAddrPort(netip.AddrPortFrom(ips[0], uint16(number))), AllowedIPs: allowed, ReplaceAllowedIPs: true, PersistentKeepaliveInterval: &keepalive}
	if cfg.Peer.PresharedKey != "" {
		psk, _ := wgtypes.ParseKey(cfg.Peer.PresharedKey)
		peer.PresharedKey = &psk
	}
	result := wgtypes.Config{PrivateKey: &private, ListenPort: &cfg.ListenPort, ReplacePeers: true, Peers: []wgtypes.PeerConfig{peer}, Jc: &cfg.JunkPacketCount, Jmin: &cfg.JunkPacketMinSize, Jmax: &cfg.JunkPacketMaxSize, S1: &cfg.InitJunkPackets, S2: &cfg.ResponseJunkPackets, S3: &cfg.TransportPacketLen, S4: &cfg.TransportPadding}
	for _, field := range []struct {
		value  string
		target **string
	}{{cfg.H1, &result.H1}, {cfg.H2, &result.H2}, {cfg.H3, &result.H3}, {cfg.H4, &result.H4}, {cfg.TransportHeader, &result.I1}, {cfg.I2, &result.I2}, {cfg.I3, &result.I3}, {cfg.I4, &result.I4}, {cfg.I5, &result.I5}} {
		if field.value != "" {
			v := field.value
			*field.target = &v
		}
	}
	return result, nil
}
func (c *Controller) Disconnect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.disconnectLocked(ctx)
}
func (c *Controller) disconnectLocked(ctx context.Context) error {
	var errs []error
	if c.userspace {
		if err := c.process.Stop(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if c.owned {
		if err := c.links.Remove(c.ifaceName); err != nil {
			errs = append(errs, err)
		} else {
			c.owned = false
		}
	}
	c.userspace = false
	if c.client != nil {
		if err := c.client.Close(); err != nil {
			errs = append(errs, err)
		}
		c.client = nil
	}
	return errors.Join(errs...)
}
func (c *Controller) Remove(ctx context.Context) error { return c.Disconnect(ctx) }
func (c *Controller) Status(ctx context.Context) (*vpn.InterfaceStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	status := &vpn.InterfaceStatus{Name: c.name, Type: c.Type(), State: vpn.StateDown, LocalAddress: c.cfg.Address}
	if !c.owned || c.client == nil {
		return status, nil
	}
	if c.userspace && !c.process.Running() {
		status.State = vpn.StateError
		status.Error = "userspace VPN process exited"
		return status, nil
	}
	exists, err := c.links.Exists(c.ifaceName)
	if err != nil {
		return status, err
	}
	if !exists {
		return status, nil
	}
	dev, err := c.client.Device(ctx, c.ifaceName)
	if err != nil {
		return status, fmt.Errorf("read VPN status: %w", err)
	}
	if dev == nil {
		return status, nil
	}
	status.PublicKey = dev.PublicKey.String()
	status.State = vpn.StateUnknown
	if len(dev.Peers) > 0 {
		peer := dev.Peers[0]
		if peer.Endpoint != nil {
			status.Endpoint = peer.Endpoint.String()
		}
		status.TxBytes = peer.TransmitBytes
		status.RxBytes = peer.ReceiveBytes
		status.LastHandshake = peer.LastHandshakeTime
		if !peer.LastHandshakeTime.IsZero() && time.Since(peer.LastHandshakeTime) >= 0 && time.Since(peer.LastHandshakeTime) <= 3*time.Minute {
			status.State = vpn.StateUp
		} else {
			status.Error = "waiting for a recent VPN handshake"
		}
	}
	return status, nil
}
func (c *Controller) HealthCheck(ctx context.Context) error {
	c.mu.Lock()
	if c.owned && c.userspace && !c.process.Running() {
		if err := c.connectLocked(ctx); err != nil {
			c.mu.Unlock()
			return err
		}
	}
	c.mu.Unlock()
	s, err := c.Status(ctx)
	if err != nil {
		return err
	}
	if s == nil || s.State != vpn.StateUp {
		return fmt.Errorf("VPN is not ready")
	}
	return nil
}
func (c *Controller) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return c.Disconnect(ctx)
}
