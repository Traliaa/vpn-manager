package importer

import (
	"encoding/json"
	"testing"
)

func TestToWireGuardConfig(t *testing.T) {
	input := `[Interface]
PrivateKey = AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=
Address = 10.0.0.2/32
DNS = 1.1.1.1
MTU = 1420

[Peer]
PublicKey = AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI=
PresharedKey = AwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwM=
Endpoint = vpn.example.com:51820
AllowedIPs = 0.0.0.0/0, ::/0
PersistentKeepalive = 25
`

	parsed, err := Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wgCfg := parsed.ToWireGuardConfig()

	if wgCfg.PrivateKey != "AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=" {
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

	if wgCfg.Peer.PublicKey != "AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI=" {
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
PrivateKey = AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=
DNS = 1.1.1.1
Jc = 3
Jmin = 50
Jmax = 100
S1 = 68
S2 = 67

[Peer]
PublicKey = AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI=
AllowedIPs = 0.0.0.0/0
Endpoint = 192.0.2.22:51820
PersistentKeepalive = 22
`

	parsed, err := Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	awgCfg := parsed.ToAmneziaWGConfig()

	if awgCfg.PrivateKey != "AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=" {
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

	if awgCfg.Peer.PublicKey != "AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI=" {
		t.Errorf("unexpected peer PublicKey: %q", awgCfg.Peer.PublicKey)
	}
	if awgCfg.Peer.Endpoint != "192.0.2.22:51820" {
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

// Regression: import used to drop H1-H4 and S4, producing a tunnel that never handshakes.
func TestToAmneziaWGPreservesObfuscation(t *testing.T) {
	parsed, err := Parse(`[Interface]
PrivateKey = AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=
Address = 10.0.0.2/32
Jc = 3
Jmin = 50
Jmax = 100
S1 = 105
S2 = 97
S3 = 4
S4 = 9
H1 = 100-200
H2 = 300-400
H3 = 500
H4 = 600
I1 = <b 0x0102>
I2 = <b 0x0304>
I3 = <b 0x0506>
I4 = <b 0x0708>
I5 = <b 0x090a>
[Peer]
PublicKey = AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI=
Endpoint = 192.0.2.1:51820
AllowedIPs = 0.0.0.0/0
`)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(parsed.ToAmneziaWGConfig())
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{"h1": "100-200", "h2": "300-400", "h3": "500", "h4": "600", "s4": float64(9), "i2": "<b 0x0304>", "i3": "<b 0x0506>", "i4": "<b 0x0708>", "i5": "<b 0x090a>"} {
		if got[key] != want {
			t.Errorf("imported %s = %v, want %v", key, got[key], want)
		}
	}
}
