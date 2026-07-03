package importer

import (
	"testing"
)

func TestToWireGuardConfig(t *testing.T) {
	input := `[Interface]
PrivateKey = eJX1z2Y3Q4R5T6U7V8W9X0Y1Z2A3B4C5D6E7F8G9H0I=
Address = 10.0.0.2/32
DNS = 1.1.1.1
MTU = 1420

[Peer]
PublicKey = TH1D+M6tPebGD4xnEwwM0hosB4MfgwYjXIVzPG+qUls=
PresharedKey = ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890abcdefgh=
Endpoint = vpn.example.com:51820
AllowedIPs = 0.0.0.0/0, ::/0
PersistentKeepalive = 25
`

	parsed, err := Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wgCfg := parsed.ToWireGuardConfig()

	if wgCfg.PrivateKey != "eJX1z2Y3Q4R5T6U7V8W9X0Y1Z2A3B4C5D6E7F8G9H0I=" {
		t.Errorf("unexpected PrivateKey: %q", wgCfg.PrivateKey)
	}
	if wgCfg.Address != "10.0.0.2/32" {
		t.Errorf("unexpected Address: %q", wgCfg.Address)
	}
	if wgCfg.DNS != "1.1.1.1" {
		t.Errorf("unexpected DNS: %q", wgCfg.DNS)
	}
	if wgCfg.MTU != 1420 {
		t.Errorf("unexpected MTU: %d", wgCfg.MTU)
	}

	if wgCfg.Peer.PublicKey != "TH1D+M6tPebGD4xnEwwM0hosB4MfgwYjXIVzPG+qUls=" {
		t.Errorf("unexpected peer PublicKey: %q", wgCfg.Peer.PublicKey)
	}
	if wgCfg.Peer.Endpoint != "vpn.example.com:51820" {
		t.Errorf("unexpected peer Endpoint: %q", wgCfg.Peer.Endpoint)
	}
	if len(wgCfg.Peer.AllowedIPs) != 2 || wgCfg.Peer.AllowedIPs[0] != "0.0.0.0/0" {
		t.Errorf("unexpected AllowedIPs: %v", wgCfg.Peer.AllowedIPs)
	}
	if wgCfg.Peer.PersistentKeepalive != 25 {
		t.Errorf("unexpected PersistentKeepalive: %d", wgCfg.Peer.PersistentKeepalive)
	}
}

func TestToAmneziaWGConfig(t *testing.T) {
	input := `[Interface]
Address = 10.92.237.3/32
PrivateKey = SOg5dR4SuxvA0o1XUt3DtQTvPZduxp3wDEChAj5E0F4=
DNS = 1.1.1.1
Jc = 3
Jmin = 50
Jmax = 100
S1 = 68
S2 = 67

[Peer]
PublicKey = TH1D+M6tPebGD4xnEwwM0hosB4MfgwYjXIVzPG+qUls=
AllowedIPs = 0.0.0.0/0
Endpoint = 185.91.127.95:32486
PersistentKeepalive = 22
`

	parsed, err := Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	awgCfg := parsed.ToAmneziaWGConfig()

	if awgCfg.PrivateKey != "SOg5dR4SuxvA0o1XUt3DtQTvPZduxp3wDEChAj5E0F4=" {
		t.Errorf("unexpected PrivateKey: %q", awgCfg.PrivateKey)
	}
	if awgCfg.Address != "10.92.237.3/32" {
		t.Errorf("unexpected Address: %q", awgCfg.Address)
	}
	if awgCfg.JunkPacketCount != 3 {
		t.Errorf("unexpected JunkPacketCount: %d", awgCfg.JunkPacketCount)
	}
	if awgCfg.JunkPacketMinSize != 50 {
		t.Errorf("unexpected JunkPacketMinSize: %d", awgCfg.JunkPacketMinSize)
	}
	if awgCfg.JunkPacketMaxSize != 100 {
		t.Errorf("unexpected JunkPacketMaxSize: %d", awgCfg.JunkPacketMaxSize)
	}
	if awgCfg.InitJunkPackets != 68 {
		t.Errorf("unexpected InitJunkPackets: %d", awgCfg.InitJunkPackets)
	}
	if awgCfg.ResponseJunkPackets != 67 {
		t.Errorf("unexpected ResponseJunkPackets: %d", awgCfg.ResponseJunkPackets)
	}

	if awgCfg.Peer.PublicKey != "TH1D+M6tPebGD4xnEwwM0hosB4MfgwYjXIVzPG+qUls=" {
		t.Errorf("unexpected peer PublicKey: %q", awgCfg.Peer.PublicKey)
	}
	if awgCfg.Peer.Endpoint != "185.91.127.95:32486" {
		t.Errorf("unexpected peer Endpoint: %q", awgCfg.Peer.Endpoint)
	}
	if len(awgCfg.Peer.AllowedIPs) != 1 || awgCfg.Peer.AllowedIPs[0] != "0.0.0.0/0" {
		t.Errorf("unexpected AllowedIPs: %v", awgCfg.Peer.AllowedIPs)
	}
}

func TestToWireGuardConfigNoPeer(t *testing.T) {
	parsed := &ParsedConfig{
		Interface: InterfaceSection{
			PrivateKey: "testkey",
			Address:    "10.0.0.1/32",
		},
	}

	wgCfg := parsed.ToWireGuardConfig()
	if wgCfg.PrivateKey != "testkey" {
		t.Errorf("unexpected PrivateKey")
	}
	// Should not panic with empty peer
	_ = wgCfg.Peer
}
