package amneziawg

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"github.com/advanced-wg/awgctrl-go/wgtypes"
)

func validateConfig(cfg Config) error {
	for _, key := range []string{cfg.PrivateKey, cfg.Peer.PublicKey} {
		if k, err := wgtypes.ParseKey(key); err != nil || k == (wgtypes.Key{}) {
			return fmt.Errorf("invalid or empty VPN key")
		}
	}
	if cfg.Peer.PresharedKey != "" {
		if _, err := wgtypes.ParseKey(cfg.Peer.PresharedKey); err != nil {
			return fmt.Errorf("invalid preshared key")
		}
	}
	p, err := netip.ParsePrefix(cfg.Address)
	if err != nil || !p.Addr().IsGlobalUnicast() {
		return fmt.Errorf("invalid interface address")
	}
	host, port, err := net.SplitHostPort(cfg.Peer.Endpoint)
	n, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || host == "" || n < 1 || n > 65535 || strings.ContainsAny(host, "\r\n\x00 ") {
		return fmt.Errorf("invalid peer endpoint")
	}
	if len(cfg.Peer.AllowedIPs) == 0 {
		return fmt.Errorf("allowed IPs are required")
	}
	for _, prefix := range cfg.Peer.AllowedIPs {
		if _, err := netip.ParsePrefix(prefix); err != nil {
			return fmt.Errorf("invalid allowed IP prefix")
		}
	}
	if cfg.Peer.PersistentKeepalive < 0 || cfg.Peer.PersistentKeepalive > 65535 || cfg.ListenPort < 0 || cfg.ListenPort > 65535 {
		return fmt.Errorf("invalid port or keepalive")
	}
	if cfg.MTU != 0 && (cfg.MTU < 576 || cfg.MTU > 65535) {
		return fmt.Errorf("invalid MTU")
	}
	for _, value := range []int{cfg.JunkPacketCount, cfg.JunkPacketMinSize, cfg.JunkPacketMaxSize, cfg.InitJunkPackets, cfg.ResponseJunkPackets, cfg.TransportPacketLen, cfg.TransportPadding} {
		if value < 0 || value > 65535 {
			return fmt.Errorf("invalid AmneziaWG padding parameter")
		}
	}
	if cfg.JunkPacketMaxSize < cfg.JunkPacketMinSize {
		return fmt.Errorf("junk packet maximum must not be less than minimum")
	}
	for _, header := range []string{cfg.H1, cfg.H2, cfg.H3, cfg.H4} {
		if header == "" {
			continue
		}
		parts := strings.Split(header, "-")
		if len(parts) > 2 {
			return fmt.Errorf("invalid magic header range")
		}
		var first uint64
		for i, part := range parts {
			value, err := strconv.ParseUint(part, 10, 32)
			if err != nil || (i == 1 && value < first) {
				return fmt.Errorf("invalid magic header range")
			}
			first = value
		}
	}
	for _, signature := range []string{cfg.TransportHeader, cfg.I2, cfg.I3, cfg.I4, cfg.I5} {
		if strings.ContainsAny(signature, "\r\n\x00") {
			return fmt.Errorf("invalid packet signature")
		}
	}
	return nil
}
