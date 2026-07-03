// Package importer parses WireGuard/AmneziaWG .conf files and converts them
// to internal config structures for provider creation.
package importer

import (
	"fmt"
	"strconv"
	"strings"
)

// DetectionThreshold is the minimum number of AmneziaWG-specific fields
// that must be present to classify a config as AmneziaWG.
const DetectionThreshold = 2

// amneziaIndicators lists field names that indicate AmneziaWG.
var amneziaIndicators = []string{"jc", "jmin", "jmax", "s1", "s2", "s3", "s4", "h1", "h2", "h3", "h4", "i1"}

// ParsedConfig holds the structured result of parsing a .conf file.
type ParsedConfig struct {
	ProviderName string // suggested provider name (derived from Endpoint or filename)
	IsAmneziaWG  bool   // whether this is an AmneziaWG config
	Interface    InterfaceSection
	Peers        []PeerSection // typically one peer, but could be multiple
}

// InterfaceSection holds parsed [Interface] fields.
type InterfaceSection struct {
	PrivateKey string
	Address    string
	DNS        string
	MTU        int
	ListenPort int

	// AmneziaWG-specific fields
	JunkPacketCount   int
	JunkPacketMinSize int
	JunkPacketMaxSize int
	InitJunkPackets   int
	ResponseJunkPkts  int
	TransportHeader   string
	TransportPktLen   int
	H1                string
	H2                string
	H3                string
	H4                string
	I1                string

	// Raw holds any unrecognised fields from [Interface]
	Raw map[string]string
}

// PeerSection holds parsed [Peer] fields.
type PeerSection struct {
	PublicKey           string
	PresharedKey        string
	Endpoint            string
	AllowedIPs          []string
	PersistentKeepalive int

	// Raw holds any unrecognised fields from [Peer]
	Raw map[string]string
}

// Parse parses a WireGuard-style .conf text and returns a ParsedConfig.
func Parse(input string) (*ParsedConfig, error) {
	cfg := &ParsedConfig{
		Interface: InterfaceSection{Raw: make(map[string]string)},
	}
	lines := strings.Split(input, "\n")

	var currentSection string
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Section header
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}

		// Key = Value (or Key Value)
		key, value := splitKeyValue(line)
		if key == "" {
			continue
		}
		key = strings.ToLower(key)

		switch currentSection {
		case "interface":
			cfg.Interface.Raw[key] = value
		case "peer":
			if len(cfg.Peers) == 0 {
				cfg.Peers = append(cfg.Peers, PeerSection{Raw: make(map[string]string)})
			}
			cfg.Peers[0].Raw[key] = value
		}
	}

	// Check for AmneziaWG indicators
	indicatorCount := 0
	for _, ind := range amneziaIndicators {
		if _, ok := cfg.Interface.Raw[ind]; ok {
			indicatorCount++
		}
	}
	cfg.IsAmneziaWG = indicatorCount >= DetectionThreshold

	// Populate typed fields from raw maps
	populateInterface(cfg)
	for i := range cfg.Peers {
		populatePeer(&cfg.Peers[i])
	}

	// Auto-generate provider name from endpoint
	cfg.ProviderName = generateName(cfg)

	return cfg, nil
}

// splitKeyValue splits "Key = Value" or "Key Value" lines.
func splitKeyValue(line string) (string, string) {
	// Try "Key = Value" first
	if idx := strings.Index(line, "="); idx >= 0 {
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		return key, val
	}
	// Try "Key Value" (space-separated)
	if idx := strings.Index(line, " "); idx >= 0 {
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		return key, val
	}
	return line, ""
}

// populateInterface copies known fields from Raw to typed fields.
func populateInterface(cfg *ParsedConfig) {
	m := cfg.Interface.Raw

	cfg.Interface.PrivateKey = m["privatekey"]
	cfg.Interface.Address = m["address"]
	cfg.Interface.DNS = m["dns"]
	cfg.Interface.MTU, _ = strconv.Atoi(m["mtu"])
	cfg.Interface.ListenPort, _ = strconv.Atoi(m["listenport"])

	if cfg.IsAmneziaWG {
		cfg.Interface.JunkPacketCount, _ = strconv.Atoi(m["jc"])
		cfg.Interface.JunkPacketMinSize, _ = strconv.Atoi(m["jmin"])
		cfg.Interface.JunkPacketMaxSize, _ = strconv.Atoi(m["jmax"])
		cfg.Interface.InitJunkPackets, _ = strconv.Atoi(m["s1"])
		cfg.Interface.ResponseJunkPkts, _ = strconv.Atoi(m["s2"])
		cfg.Interface.TransportPktLen, _ = strconv.Atoi(m["s3"])
		// S4 doesn't map directly, store in Raw
		cfg.Interface.TransportHeader = m["i1"]
		cfg.Interface.H1 = m["h1"]
		cfg.Interface.H2 = m["h2"]
		cfg.Interface.H3 = m["h3"]
		cfg.Interface.H4 = m["h4"]
		cfg.Interface.I1 = m["i1"]
	}
}

// populatePeer copies known fields from Raw to typed fields.
func populatePeer(p *PeerSection) {
	m := p.Raw

	p.PublicKey = m["publickey"]
	p.PresharedKey = m["presharedkey"]
	p.Endpoint = m["endpoint"]
	p.PersistentKeepalive, _ = strconv.Atoi(m["persistentkeepalive"])

	// AllowedIPs may be comma-separated or multi-line
	allowedRaw := m["allowedips"]
	if allowedRaw != "" {
		parts := strings.Split(allowedRaw, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				p.AllowedIPs = append(p.AllowedIPs, part)
			}
		}
	}
}

// generateName creates a provider name from endpoint hostname.
func generateName(cfg *ParsedConfig) string {
	for _, p := range cfg.Peers {
		if p.Endpoint != "" {
			host, _, err := netSplitHostPort(p.Endpoint)
			if err == nil && host != "" {
				return host
			}
		}
	}
	return ""
}

// netSplitHostPort splits host:port, handling IPv6 addresses.
func netSplitHostPort(endpoint string) (string, string, error) {
	// Quick check for IPv6
	if strings.HasPrefix(endpoint, "[") {
		// [::1]:port
		closeBracket := strings.LastIndex(endpoint, "]")
		if closeBracket < 0 {
			return "", "", fmt.Errorf("invalid endpoint: %s", endpoint)
		}
		host := endpoint[1:closeBracket]
		port := ""
		if len(endpoint) > closeBracket+1 && endpoint[closeBracket+1] == ':' {
			port = endpoint[closeBracket+2:]
		}
		return host, port, nil
	}

	// IPv4 or hostname
	colon := strings.LastIndex(endpoint, ":")
	if colon < 0 {
		return endpoint, "", nil
	}
	return endpoint[:colon], endpoint[colon+1:], nil
}
