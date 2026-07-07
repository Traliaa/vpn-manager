package amneziawg

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/Traliaa/vpn-manager/internal/vpn"
	"github.com/advanced-wg/awgctrl-go"
	"github.com/advanced-wg/awgctrl-go/wgtypes"
	"github.com/vishvananda/netlink"
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
func (c *Controller) InterfaceName() string  { return c.ifaceName }

// sanitizeIfaceName создаёт корректное Linux-имя интерфейса из произвольной строки.
func sanitizeIfaceName(name, prefix string) string {
	var result []rune
	for _, r := range name {
		if len(result) >= 12 {
			break
		}
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			result = append(result, r)
		}
	}
	s := string(result)
	if s == "" {
		return prefix + "0"
	}
	if s[0] >= '0' && s[0] <= '9' {
		s = prefix + "_" + s
	}
	return s
}

// ApplyConfig парсит, валидирует и сохраняет конфигурацию.
// Не создаёт и не изменяет интерфейс Linux — это делает Connect().
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

	if c.cfg.PrivateKey == "" || c.cfg.Peer.PublicKey == "" {
		return fmt.Errorf("incomplete config: missing private_key or peer.public_key")
	}

	c.logger.Info("config validated",
		zap.String("iface", c.ifaceName),
		zap.String("endpoint", c.cfg.Peer.Endpoint),
		zap.Strings("allowed_ips", c.cfg.Peer.AllowedIPs),
	)
	return nil
}

// Connect создаёт интерфейс, применяет конфиг, поднимает его и настраивает маршруты.
func (c *Controller) Connect(ctx context.Context) error {
	c.logger.Info("connecting AmneziaWG",
		zap.String("iface", c.ifaceName),
		zap.String("endpoint", c.cfg.Peer.Endpoint),
	)

	// 0. Если интерфейс уже существует — удаляем
	if existing, err := netlink.LinkByName(c.ifaceName); err == nil {
		c.logger.Debug("removing existing interface", zap.String("iface", c.ifaceName))
		if err := netlink.LinkDel(existing); err != nil {
			return fmt.Errorf("remove existing interface %s: %w", c.ifaceName, err)
		}
	}

	// 1. Создаём WireGuard-интерфейс
	wgLink := &netlink.Wireguard{LinkAttrs: netlink.NewLinkAttrs()}
	wgLink.Name = c.ifaceName
	if err := netlink.LinkAdd(wgLink); err != nil {
		return fmt.Errorf("create interface %s: %w", c.ifaceName, err)
	}
	c.logger.Debug("interface created", zap.String("iface", c.ifaceName))

	// 2. Применяем конфигурацию WG (ключи, пиры)
	pubKey, err := wgtypes.ParseKey(c.cfg.Peer.PublicKey)
	if err != nil {
		return fmt.Errorf("invalid peer public key: %w", err)
	}
	privKey, err := wgtypes.ParseKey(c.cfg.PrivateKey)
	if err != nil {
		return fmt.Errorf("invalid private key: %w", err)
	}

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
	if err := c.client.ConfigureDevice(ctx, c.ifaceName, deviceCfg); err != nil {
		return fmt.Errorf("configure device %s: %w", c.ifaceName, err)
	}
	c.logger.Debug("device configured", zap.String("iface", c.ifaceName))

	// 3. Устанавливаем MTU (если задан)
	if c.cfg.MTU > 0 {
		link, err := netlink.LinkByName(c.ifaceName)
		if err != nil {
			return fmt.Errorf("find interface %s: %w", c.ifaceName, err)
		}
		if err := netlink.LinkSetMTU(link, c.cfg.MTU); err != nil {
			return fmt.Errorf("set MTU on %s: %w", c.ifaceName, err)
		}
		c.logger.Debug("MTU set", zap.String("iface", c.ifaceName), zap.Int("mtu", c.cfg.MTU))
	}

	// 4. Назначаем IP-адрес (если задан)
	if c.cfg.Address != "" {
		link, err := netlink.LinkByName(c.ifaceName)
		if err != nil {
			return fmt.Errorf("find interface %s: %w", c.ifaceName, err)
		}
		prefix, err := netip.ParsePrefix(c.cfg.Address)
		if err != nil {
			return fmt.Errorf("invalid address %q: %w", c.cfg.Address, err)
		}
		addr := &netlink.Addr{
			IPNet: &net.IPNet{
				IP:   prefix.Addr().AsSlice(),
				Mask: net.CIDRMask(prefix.Bits(), prefix.Addr().BitLen()),
			},
		}
		if err := netlink.AddrAdd(link, addr); err != nil {
			return fmt.Errorf("add address %s to %s: %w", c.cfg.Address, c.ifaceName, err)
		}
		c.logger.Debug("IP address assigned", zap.String("iface", c.ifaceName), zap.String("addr", c.cfg.Address))
	}

	// 5. Поднимаем интерфейс
	link, err := netlink.LinkByName(c.ifaceName)
	if err != nil {
		return fmt.Errorf("find interface %s: %w", c.ifaceName, err)
	}
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("bring up interface %s: %w", c.ifaceName, err)
	}
	c.logger.Debug("interface is up", zap.String("iface", c.ifaceName))

	// 6. Добавляем маршруты (если нужны)
	// Для каждого allowed_ip добавляем маршрут через интерфейс
	for _, s := range c.cfg.Peer.AllowedIPs {
		if s == "0.0.0.0/0" || s == "::/0" {
			continue // default route — не добавляем, управляется маршрутизацией
		}
		prefix, err := netip.ParsePrefix(s)
		if err != nil {
			c.logger.Warn("skip invalid route", zap.String("route", s), zap.Error(err))
			continue
		}
		dst := &net.IPNet{
			IP:   prefix.Addr().AsSlice(),
			Mask: net.CIDRMask(prefix.Bits(), prefix.Addr().BitLen()),
		}
		route := &netlink.Route{
			Dst:       dst,
			LinkIndex: link.Attrs().Index,
		}
		if err := netlink.RouteAdd(route); err != nil {
			// Может уже существовать — не фатально
			c.logger.Debug("route add", zap.String("route", s), zap.Error(err))
		}
	}

	c.logger.Info("AmneziaWG connected",
		zap.String("iface", c.ifaceName),
	)
	return nil
}

// Disconnect удаляет интерфейс и очищает конфигурацию.
func (c *Controller) Disconnect(ctx context.Context) error {
	c.logger.Info("disconnecting AmneziaWG", zap.String("iface", c.ifaceName))

	if existing, err := netlink.LinkByName(c.ifaceName); err == nil {
		if err := netlink.LinkDel(existing); err != nil {
			return fmt.Errorf("remove interface %s: %w", c.ifaceName, err)
		}
		c.logger.Info("interface removed", zap.String("iface", c.ifaceName))
	} else {
		c.logger.Debug("interface not found, nothing to remove", zap.String("iface", c.ifaceName))
	}

	if c.client != nil {
		c.client.Close()
		c.client = nil
	}
	return nil
}

// Remove — бэкап-совместимость.
// Deprecated: используйте Disconnect.
func (c *Controller) Remove(ctx context.Context) error {
	return c.Disconnect(ctx)
}

// Status возвращает текущее состояние интерфейса.
func (c *Controller) Status(ctx context.Context) (*vpn.InterfaceStatus, error) {
	// Проверяем, существует ли интерфейс через netlink
	if _, err := netlink.LinkByName(c.ifaceName); err != nil {
		return &vpn.InterfaceStatus{
			Name:  c.name,
			Type:  c.Type(),
			State: vpn.StateDown,
		}, nil
	}

	if c.client == nil {
		return &vpn.InterfaceStatus{Name: c.name, Type: c.Type(), State: vpn.StateDown}, nil
	}

	dev, err := c.client.Device(ctx, c.ifaceName)
	if err != nil {
		return &vpn.InterfaceStatus{
			Name:  c.name,
			Type:  c.Type(),
			State: vpn.StateError,
		}, fmt.Errorf("get device status: %w", err)
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
