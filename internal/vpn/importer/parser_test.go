package importer

import (
	"testing"
)

func TestParseWireGuard(t *testing.T) {
	input := `[Interface]
PrivateKey = eJX1z2Y3Q4R5T6U7V8W9X0Y1Z2A3B4C5D6E7F8G9H0I=
Address = 10.0.0.2/32
DNS = 1.1.1.1
MTU = 1420
ListenPort = 51820

[Peer]
PublicKey = TH1D+M6tPebGD4xnEwwM0hosB4MfgwYjXIVzPG+qUls=
PresharedKey = ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890abcdefgh=
Endpoint = vpn.example.com:51820
AllowedIPs = 0.0.0.0/0, ::/0
PersistentKeepalive = 25
`

	cfg, err := Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.IsAmneziaWG {
		t.Error("expected false for WireGuard config")
	}

	if cfg.Interface.PrivateKey != "eJX1z2Y3Q4R5T6U7V8W9X0Y1Z2A3B4C5D6E7F8G9H0I=" {
		t.Errorf("unexpected PrivateKey: %q", cfg.Interface.PrivateKey)
	}
	if cfg.Interface.Address != "10.0.0.2/32" {
		t.Errorf("unexpected Address: %q", cfg.Interface.Address)
	}
	if cfg.Interface.DNS != "1.1.1.1" {
		t.Errorf("unexpected DNS: %q", cfg.Interface.DNS)
	}
	if cfg.Interface.MTU != 1420 {
		t.Errorf("unexpected MTU: %d", cfg.Interface.MTU)
	}

	if len(cfg.Peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(cfg.Peers))
	}
	peer := cfg.Peers[0]
	if peer.PublicKey != "TH1D+M6tPebGD4xnEwwM0hosB4MfgwYjXIVzPG+qUls=" {
		t.Errorf("unexpected PublicKey: %q", peer.PublicKey)
	}
	if peer.Endpoint != "vpn.example.com:51820" {
		t.Errorf("unexpected Endpoint: %q", peer.Endpoint)
	}
	if peer.PresharedKey != "ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890abcdefgh=" {
		t.Errorf("unexpected PresharedKey: %q", peer.PresharedKey)
	}
	if len(peer.AllowedIPs) != 2 || peer.AllowedIPs[0] != "0.0.0.0/0" || peer.AllowedIPs[1] != "::/0" {
		t.Errorf("unexpected AllowedIPs: %v", peer.AllowedIPs)
	}
	if peer.PersistentKeepalive != 25 {
		t.Errorf("unexpected PersistentKeepalive: %d", peer.PersistentKeepalive)
	}
}

func TestParseAmneziaWG(t *testing.T) {
	input := `[Interface]
Address = 10.92.237.3/32
PrivateKey = SOg5dR4SuxvA0o1XUt3DtQTvPZduxp3wDEChAj5E0F4=
DNS = 1.1.1.1
Jc = 3
Jmin = 50
Jmax = 100
S1 = 68
S2 = 67
S3 = 4
S4 = 9
H1 = 485277783-485343318
H2 = 1630401762-1630467297
H3 = 1182268304-1182333839
H4 = 713909618-713975153
i1 = test-initial-packet-data

[Peer]
PublicKey = TH1D+M6tPebGD4xnEwwM0hosB4MfgwYjXIVzPG+qUls=
AllowedIPs = 0.0.0.0/0
Endpoint = 185.91.127.95:32486
PersistentKeepalive = 22
`

	cfg, err := Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.IsAmneziaWG {
		t.Error("expected true for AmneziaWG config")
	}

	if cfg.Interface.PrivateKey != "SOg5dR4SuxvA0o1XUt3DtQTvPZduxp3wDEChAj5E0F4=" {
		t.Errorf("unexpected PrivateKey: %q", cfg.Interface.PrivateKey)
	}
	if cfg.Interface.Address != "10.92.237.3/32" {
		t.Errorf("unexpected Address: %q", cfg.Interface.Address)
	}
	if cfg.Interface.JunkPacketCount != 3 {
		t.Errorf("unexpected JunkPacketCount: %d", cfg.Interface.JunkPacketCount)
	}
	if cfg.Interface.JunkPacketMinSize != 50 {
		t.Errorf("unexpected JunkPacketMinSize: %d", cfg.Interface.JunkPacketMinSize)
	}
	if cfg.Interface.JunkPacketMaxSize != 100 {
		t.Errorf("unexpected JunkPacketMaxSize: %d", cfg.Interface.JunkPacketMaxSize)
	}
	if cfg.Interface.InitJunkPackets != 68 {
		t.Errorf("unexpected InitJunkPackets: %d", cfg.Interface.InitJunkPackets)
	}
	if cfg.Interface.H1 != "485277783-485343318" {
		t.Errorf("unexpected H1: %q", cfg.Interface.H1)
	}
	if cfg.Interface.I1 != "test-initial-packet-data" {
		t.Errorf("unexpected I1: %q", cfg.Interface.I1)
	}

	if len(cfg.Peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(cfg.Peers))
	}
	peer := cfg.Peers[0]
	if peer.PublicKey != "TH1D+M6tPebGD4xnEwwM0hosB4MfgwYjXIVzPG+qUls=" {
		t.Errorf("unexpected PublicKey: %q", peer.PublicKey)
	}
	if peer.Endpoint != "185.91.127.95:32486" {
		t.Errorf("unexpected Endpoint: %q", peer.Endpoint)
	}
	if len(peer.AllowedIPs) != 1 || peer.AllowedIPs[0] != "0.0.0.0/0" {
		t.Errorf("unexpected AllowedIPs: %v", peer.AllowedIPs)
	}
	if peer.PersistentKeepalive != 22 {
		t.Errorf("unexpected PersistentKeepalive: %d", peer.PersistentKeepalive)
	}
}

func TestParseMinimalConfig(t *testing.T) {
	input := `[Interface]
PrivateKey = testkey
Address = 10.0.0.1/32

[Peer]
PublicKey = peerkey
Endpoint = 1.2.3.4:51820
AllowedIPs = 0.0.0.0/0
`

	cfg, err := Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Interface.PrivateKey != "testkey" {
		t.Errorf("unexpected PrivateKey: %q", cfg.Interface.PrivateKey)
	}
	if len(cfg.Peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(cfg.Peers))
	}
	if cfg.Peers[0].Endpoint != "1.2.3.4:51820" {
		t.Errorf("unexpected Endpoint: %q", cfg.Peers[0].Endpoint)
	}
}

func TestParseOnlyInterface(t *testing.T) {
	input := `[Interface]
PrivateKey = testkey
Address = 10.0.0.1/32
`

	cfg, err := Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Interface.PrivateKey != "testkey" {
		t.Errorf("unexpected PrivateKey")
	}
	if len(cfg.Peers) != 0 {
		t.Errorf("expected 0 peers, got %d", len(cfg.Peers))
	}
}

func TestParseEmpty(t *testing.T) {
	cfg, err := Parse("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Interface.PrivateKey != "" {
		t.Errorf("expected empty PrivateKey")
	}
	if cfg.IsAmneziaWG {
		t.Error("expected false for empty config")
	}
}

func TestParseComments(t *testing.T) {
	input := `# This is a comment
[Interface]
# Another comment
PrivateKey = key1
Address = 10.0.0.1/32

[Peer]
# Peer comment
PublicKey = key2
# Another peer comment
Endpoint = example.com:51820
AllowedIPs = 0.0.0.0/0
`

	cfg, err := Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Interface.PrivateKey != "key1" {
		t.Errorf("unexpected PrivateKey: %q", cfg.Interface.PrivateKey)
	}
	if len(cfg.Peers) != 1 || cfg.Peers[0].PublicKey != "key2" {
		t.Errorf("unexpected peer PublicKey: %q", cfg.Peers[0].PublicKey)
	}
}

func TestProviderNameGeneration(t *testing.T) {
	input := `[Interface]
PrivateKey = key
Address = 10.0.0.1/32

[Peer]
PublicKey = peerkey
Endpoint = my-vpn.example.com:51820
AllowedIPs = 0.0.0.0/0
`

	cfg, err := Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ProviderName != "my-vpn.example.com" {
		t.Errorf("expected provider name 'my-vpn.example.com', got %q", cfg.ProviderName)
	}
}

func TestProviderNameFromIPv4(t *testing.T) {
	input := `[Interface]
PrivateKey = key
Address = 10.0.0.1/32

[Peer]
PublicKey = peerkey
Endpoint = 192.168.1.1:51820
AllowedIPs = 0.0.0.0/0
`

	cfg, err := Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ProviderName != "192.168.1.1" {
		t.Errorf("expected provider name '192.168.1.1', got %q", cfg.ProviderName)
	}
}

func TestParseCaseInsensitive(t *testing.T) {
	input := `[Interface]
privatekey = mykey
address = 10.0.0.1/32
dns = 8.8.8.8

[Peer]
publickey = peerkey
endpoint = vpn.test.com:51820
allowedips = 0.0.0.0/0
`

	cfg, err := Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Interface.PrivateKey != "mykey" {
		t.Errorf("unexpected PrivateKey: %q", cfg.Interface.PrivateKey)
	}
	if cfg.Interface.Address != "10.0.0.1/32" {
		t.Errorf("unexpected Address")
	}
}

func TestAutoDetectWireGuard(t *testing.T) {
	// Only basic WireGuard fields — should not be detected as AmneziaWG
	input := `[Interface]
PrivateKey = key
Address = 10.0.0.1/32

[Peer]
PublicKey = peerkey
Endpoint = 1.2.3.4:51820
AllowedIPs = 0.0.0.0/0
`

	cfg, err := Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.IsAmneziaWG {
		t.Error("config without Amnezia fields should not be detected as AmneziaWG")
	}
}

func TestAutoDetectAmnezia(t *testing.T) {
	// Only one Amnezia field — should still be detected
	input := `[Interface]
PrivateKey = key
Address = 10.0.0.1/32
Jc = 5
Jmin = 10

[Peer]
PublicKey = peerkey
Endpoint = 1.2.3.4:51820
AllowedIPs = 0.0.0.0/0
`

	cfg, err := Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.IsAmneziaWG {
		t.Error("config with Jc and Jmin should be detected as AmneziaWG")
	}
}
