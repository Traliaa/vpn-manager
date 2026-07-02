// Package hysteria2 provides a sing-box outbound configuration for the Hysteria2 protocol.
package hysteria2

import (
	"encoding/json"
	"fmt"
)

// Config represents the configuration for a Hysteria2 outbound in sing-box.
type Config struct {
	// Server address (IP or hostname).
	Server string `json:"server"`
	// Server port.
	ServerPort uint16 `json:"server_port"`
	// Authentication password.
	Password string `json:"password,omitempty"`
	// Upload bandwidth (Mbps).
	UpMbps int `json:"up_mbps,omitempty"`
	// Download bandwidth (Mbps).
	DownMbps int `json:"down_mbps,omitempty"`
	// Optional obfuscation settings.
	Obfs *ObfsConfig `json:"obfs,omitempty"`
	// TLS settings.
	TLS *TLSConfig `json:"tls,omitempty"`
	// Optional multiplex settings.
	Multiplex *MultiplexConfig `json:"multiplex,omitempty"`
	// OutboundTag for routing (auto-generated if empty).
	OutboundTag string `json:"tag,omitempty"`
}

// ObfsConfig configures Hysteria2 obfuscation.
type ObfsConfig struct {
	Type     string `json:"type"` // "salamander"
	Password string `json:"password,omitempty"`
}

// TLSConfig configures TLS for Hysteria2 outbound.
type TLSConfig struct {
	Enabled     bool     `json:"enabled"`
	ServerName  string   `json:"server_name,omitempty"`
	Insecure    bool     `json:"insecure,omitempty"`
	ALPN        []string `json:"alpn,omitempty"`
	MinVersion  string   `json:"min_version,omitempty"`
	Certificate []byte   `json:"certificate,omitempty"` // Custom CA cert (PEM)
}

// MultiplexConfig configures Hysteria2 multiplexing.
type MultiplexConfig struct {
	Enabled  bool   `json:"enabled"`
	Protocol string `json:"protocol,omitempty"` // "smux"
	MaxConn  int    `json:"max_connections,omitempty"`
	MinStrm  int    `json:"min_streams,omitempty"`
}

// BuildOutbound produces a sing-box outbound JSON object for a Hysteria2 connection.
func (c Config) BuildOutbound() (map[string]any, error) {
	if c.Server == "" {
		return nil, fmt.Errorf("hysteria2: server is required")
	}
	if c.ServerPort == 0 {
		return nil, fmt.Errorf("hysteria2: server_port is required")
	}

	tag := c.OutboundTag
	if tag == "" {
		tag = fmt.Sprintf("hy2-%s-%d", c.Server, c.ServerPort)
	}

	outbound := map[string]any{
		"type":        "hysteria2",
		"tag":         tag,
		"server":      c.Server,
		"server_port": c.ServerPort,
	}

	if c.Password != "" {
		outbound["password"] = c.Password
	}
	if c.UpMbps > 0 {
		outbound["up_mbps"] = c.UpMbps
	}
	if c.DownMbps > 0 {
		outbound["down_mbps"] = c.DownMbps
	}

	// Obfuscation
	if c.Obfs != nil {
		obfsMap := map[string]any{
			"type": c.Obfs.Type,
		}
		if c.Obfs.Password != "" {
			obfsMap["password"] = c.Obfs.Password
		}
		outbound["obfs"] = obfsMap
	}

	// TLS
	if c.TLS != nil && c.TLS.Enabled {
		tlsMap := map[string]any{}
		if c.TLS.ServerName != "" {
			tlsMap["server_name"] = c.TLS.ServerName
		}
		if c.TLS.Insecure {
			tlsMap["insecure"] = true
		}
		if len(c.TLS.ALPN) > 0 {
			tlsMap["alpn"] = c.TLS.ALPN
		}
		if c.TLS.MinVersion != "" {
			tlsMap["min_version"] = c.TLS.MinVersion
		}
		if len(c.TLS.Certificate) > 0 {
			tlsMap["certificate"] = string(c.TLS.Certificate)
		}
		outbound["tls"] = tlsMap
	}

	// Multiplex
	if c.Multiplex != nil && c.Multiplex.Enabled {
		muxMap := map[string]any{
			"enabled": true,
		}
		if c.Multiplex.Protocol != "" {
			muxMap["protocol"] = c.Multiplex.Protocol
		}
		if c.Multiplex.MaxConn > 0 {
			muxMap["max_connections"] = c.Multiplex.MaxConn
		}
		if c.Multiplex.MinStrm > 0 {
			muxMap["min_streams"] = c.Multiplex.MinStrm
		}
		outbound["multiplex"] = muxMap
	}

	return outbound, nil
}

// Tag implements the singbox.Outbound interface.
func (c Config) Tag() string {
	if c.OutboundTag != "" {
		return c.OutboundTag
	}
	return fmt.Sprintf("hy2-%s-%d", c.Server, c.ServerPort)
}

// MarshalJSON implements json.Marshaler for storage in database.
func (c Config) MarshalJSON() ([]byte, error) {
	type alias Config
	return json.Marshal(alias(c))
}

// UnmarshalJSON implements json.Unmarshaler for loading from database.
func (c *Config) UnmarshalJSON(data []byte) error {
	type alias Config
	return json.Unmarshal(data, (*alias)(c))
}
