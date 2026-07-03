package importer

import (
	"github.com/Traliaa/vpn-manager/internal/vpn/amneziawg"
	"github.com/Traliaa/vpn-manager/internal/vpn/wireguard"
)

// ToWireGuardConfig converts the parsed config to a wireguard.Config.
func (p *ParsedConfig) ToWireGuardConfig() wireguard.Config {
	cfg := wireguard.Config{
		PrivateKey: p.Interface.PrivateKey,
		Address:    p.Interface.Address,
		DNS:        p.Interface.DNS,
		MTU:        p.Interface.MTU,
	}

	if len(p.Peers) > 0 {
		peer := p.Peers[0]
		cfg.Peer = wireguard.PeerConfig{
			PublicKey:           peer.PublicKey,
			PresharedKey:        peer.PresharedKey,
			Endpoint:            peer.Endpoint,
			AllowedIPs:          peer.AllowedIPs,
			PersistentKeepalive: peer.PersistentKeepalive,
		}
	}

	return cfg
}

// ToAmneziaWGConfig converts the parsed config to an amneziawg.Config.
func (p *ParsedConfig) ToAmneziaWGConfig() amneziawg.Config {
	cfg := amneziawg.Config{
		PrivateKey:          p.Interface.PrivateKey,
		Address:             p.Interface.Address,
		DNS:                 p.Interface.DNS,
		MTU:                 p.Interface.MTU,
		JunkPacketCount:     p.Interface.JunkPacketCount,
		JunkPacketMinSize:   p.Interface.JunkPacketMinSize,
		JunkPacketMaxSize:   p.Interface.JunkPacketMaxSize,
		InitJunkPackets:     p.Interface.InitJunkPackets,
		ResponseJunkPackets: p.Interface.ResponseJunkPkts,
		TransportHeader:     p.Interface.TransportHeader,
		TransportPacketLen:  p.Interface.TransportPktLen,
	}

	if len(p.Peers) > 0 {
		peer := p.Peers[0]
		cfg.Peer = amneziawg.PeerConfig{
			PublicKey:           peer.PublicKey,
			PresharedKey:        peer.PresharedKey,
			Endpoint:            peer.Endpoint,
			AllowedIPs:          peer.AllowedIPs,
			PersistentKeepalive: peer.PersistentKeepalive,
		}
	}

	return cfg
}
