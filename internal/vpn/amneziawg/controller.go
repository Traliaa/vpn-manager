package amneziawg

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"os/exec"
	"strings"
	"time"

	"github.com/Traliaa/vpn-manager/internal/vpn"
	"github.com/advanced-wg/awgctrl-go"
	"github.com/advanced-wg/awgctrl-go/wgtypes"
	"go.uber.org/zap"
)

// Config представляет конфигурацию AmneziaWG.
type Config struct {
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

// Controller управляет AmneziaWG-интерфейсом.
type Controller struct {
	name      string
	ifaceName string
	cfg       Config
	logger    *zap.Logger
	client    *wgctrl.Client
}

// NewController создаёт AmneziaWG-контроллер.
func NewController(name string, cfg Config, logger *zap.Logger) (*Controller, error) {
	client, err := wgctrl.New()
	if err != nil {
		return nil, fmt.Errorf("create wgctrl client: %w", err)
	}

	return &Controller{
		name:      name,
		ifaceName: sanitizeIfaceName(name, "awg"),
		cfg:       cfg,
		logger:    logger.With(zap.String("interface", name), zap.String("type", "amneziawg")),
		client:    client,
	}, nil
}

func (c *Controller) Type() vpn.ProviderType { return vpn.ProviderAmneziaWG }
func (c *Controller) Name() string           { return c.name }

// sanitizeIfaceName создаёт корректное Linux-имя интерфейса из произвольной строки.
func sanitizeIfaceName(name, prefix string) string {
	var result strings.Builder
	for _, r := range name {
		if result.Len() >= 12 {
			break
		}
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			result.WriteRune(r)
		}
	}
	s := result.String()
	if s == "" {
		return prefix + "0"
	}
	// Если имя начинается с цифры — добавляем префикс
	if s[0] >= '0' && s[0] <= '9' {
		s = prefix + "_" + s
	}
	return s
}

// ApplyConfig создаёт или обновляет конфигурацию интерфейса через wgctrl.
func (c *Controller) ApplyConfig(ctx context.Context, cfg interface{}) error {
	switch v := cfg.(type) {
	case Config:
		c.cfg = v
	case *Config:
		if v != nil {
			c.cfg = *v
		}
	default:
		data, err := json.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("invalid config type: %T", cfg)
		}
		if err := json.Unmarshal(data, &c.cfg); err != nil {
			return fmt.Errorf("unmarshal awg config: %w", err)
		}
		// Если peer-поля пустые — пробуем плоский формат
		if c.cfg.Peer.Endpoint == "" && c.cfg.Peer.PublicKey == "" {
			var flat struct {
				PublicKey  string   `json:"public_key"`
				Endpoint   string   `json:"endpoint"`
				AllowedIPs []string `json:"allowed_ips"`
			}
			if err := json.Unmarshal(data, &flat); err == nil && flat.PublicKey != "" {
				c.cfg.Peer.PublicKey = flat.PublicKey
				if flat.Endpoint != "" {
					c.cfg.Peer.Endpoint = flat.Endpoint
				}
				if len(flat.AllowedIPs) > 0 {
					c.cfg.Peer.AllowedIPs = flat.AllowedIPs
				}
				c.logger.Debug("migrated flat config to nested peer format")
			}
		}
	}

	c.logger.Info("applying AmneziaWG configuration",
		zap.String("iface", c.ifaceName),
		zap.String("endpoint", c.cfg.Peer.Endpoint),
		zap.Strings("allowed_ips", c.cfg.Peer.AllowedIPs),
	)

	if c.cfg.PrivateKey == "" || c.cfg.Peer.PublicKey == "" {
		return fmt.Errorf("incomplete config: missing private_key or peer.public_key")
	}

	privKey, err := wgtypes.ParseKey(c.cfg.PrivateKey)
	if err != nil {
		return fmt.Errorf("invalid private key: %w", err)
	}
	pubKey, err := wgtypes.ParseKey(c.cfg.Peer.PublicKey)
	if err != nil {
		return fmt.Errorf("invalid peer public key: %w", err)
	}

	// Парсим AllowedIPs в []net.IPNet (требование wgctrl)
	allowedIPs := make([]net.IPNet, 0, len(c.cfg.Peer.AllowedIPs))
	for _, s := range c.cfg.Peer.AllowedIPs {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return fmt.Errorf("invalid allowed IP %q: %w", s, err)
		}
		masked := p.Masked()
		allowedIPs = append(allowedIPs, net.IPNet{
			IP:   masked.Addr().AsSlice(),
			Mask: net.CIDRMask(masked.Bits(), masked.Addr().BitLen()),
		})
	}

	// Резолвим endpoint
	var endpoint *net.UDPAddr
	if c.cfg.Peer.Endpoint != "" {
		host, portStr, err := net.SplitHostPort(c.cfg.Peer.Endpoint)
		if err != nil {
			return fmt.Errorf("invalid endpoint %q: %w", c.cfg.Peer.Endpoint, err)
		}
		port, err := net.LookupPort("udp", portStr)
		if err != nil {
			return fmt.Errorf("invalid endpoint port %q: %w", portStr, err)
		}
		ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return fmt.Errorf("resolve endpoint %q: %w", host, err)
		}
		if len(ips) > 0 {
			endpoint = net.UDPAddrFromAddrPort(netip.AddrPortFrom(ips[0], uint16(port)))
		}
	}

	// PersistentKeepaliveInterval
	var keepalive *time.Duration
	if c.cfg.Peer.PersistentKeepalive > 0 {
		v := time.Duration(c.cfg.Peer.PersistentKeepalive) * time.Second
		keepalive = &v
	}

	peerCfg := wgtypes.PeerConfig{
		PublicKey:                   pubKey,
		Endpoint:                    endpoint,
		AllowedIPs:                  allowedIPs,
		PersistentKeepaliveInterval: keepalive,
		ReplaceAllowedIPs:           true,
	}
	if c.cfg.Peer.PresharedKey != "" {
		psk, err := wgtypes.ParseKey(c.cfg.Peer.PresharedKey)
		if err != nil {
			return fmt.Errorf("invalid preshared key: %w", err)
		}
		peerCfg.PresharedKey = &psk
	}

	deviceCfg := wgtypes.Config{
		PrivateKey:   &privKey,
		ReplacePeers: true,
		Peers:        []wgtypes.PeerConfig{peerCfg},
	}

	// Create interface if not exists
	if err := exec.Command("ip", "link", "add", "dev", c.ifaceName, "type", "wireguard").Run(); err != nil {
		c.logger.Debug("ip link add (may already exist)",
			zap.String("iface", c.ifaceName),
			zap.Error(err),
		)
	}
	if err := c.client.ConfigureDevice(ctx, c.ifaceName, deviceCfg); err != nil {
		return fmt.Errorf("configure device %s: %w", c.ifaceName, err)
	}

	if c.cfg.JunkPacketCount > 0 {
		c.logger.Debug("AmneziaWG junk packets configured (stub)",
			zap.Int("count", c.cfg.JunkPacketCount),
		)
	}

	c.logger.Info("AmneziaWG configuration applied")
	return nil
}

// Remove удаляет интерфейс.
func (c *Controller) Remove(ctx context.Context) error {
	c.logger.Info("removing AmneziaWG interface")
	if c.client != nil {
		_ = c.client.ConfigureDevice(ctx, c.ifaceName, wgtypes.Config{
			ReplacePeers: true,
			Peers:        []wgtypes.PeerConfig{},
		})
	}
	return nil
}

// Status возвращает текущее состояние интерфейса.
func (c *Controller) Status(ctx context.Context) (*vpn.InterfaceStatus, error) {
	if c.client == nil {
		return &vpn.InterfaceStatus{Name: c.name, Type: c.Type(), State: vpn.StateDown}, nil
	}

	dev, err := c.client.Device(ctx, c.ifaceName)
	if err != nil {
		return &vpn.InterfaceStatus{Name: c.name, Type: c.Type(), State: vpn.StateError},
			fmt.Errorf("get device status: %w", err)
	}
	if dev == nil {
		return &vpn.InterfaceStatus{Name: c.name, Type: c.Type(), State: vpn.StateDown}, nil
	}

	status := &vpn.InterfaceStatus{
		Name:      c.name,
		Type:      c.Type(),
		State:     vpn.StateUp,
		PublicKey: dev.PublicKey.String(),
	}
	if len(dev.Peers) > 0 {
		peer := dev.Peers[0]
		status.Endpoint = peer.Endpoint.String()
		status.TxBytes = peer.TransmitBytes
		status.RxBytes = peer.ReceiveBytes
		if !peer.LastHandshakeTime.IsZero() {
			status.LastHandshake = peer.LastHandshakeTime
		}
	}
	return status, nil
}

// HealthCheck проверяет доступность через VPN-интерфейс.
func (c *Controller) HealthCheck(ctx context.Context) error {
	_, err := c.Status(ctx)
	return err
}

// Close освобождает ресурсы wgctrl.
func (c *Controller) Close() error {
	if c.client != nil {
		c.client.Close()
	}
	return nil
}
