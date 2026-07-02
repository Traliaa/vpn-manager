// Package shadowsocks provides a sing-box outbound configuration for the Shadowsocks protocol.
package shadowsocks

import (
	"encoding/json"
	"fmt"
)

// Config represents the configuration for a Shadowsocks outbound in sing-box.
type Config struct {
	// Server address (IP or hostname).
	Server string `json:"server"`
	// Server port.
	ServerPort uint16 `json:"server_port"`
	// Encryption method (e.g. "aes-256-gcm", "chacha20-ietf-poly1305", "2022-blake3-aes-256-gcm").
	Method string `json:"method"`
	// Password or key.
	Password string `json:"password"`
	// Optional plugin (e.g. "v2ray-plugin", "obfs-server").
	Plugin string `json:"plugin,omitempty"`
	// Plugin options string.
	PluginOpts string `json:"plugin_opts,omitempty"`
	// Enable UDP (default true).
	UDP bool `json:"udp,omitempty"`
	// Optional multiplex settings.
	Multiplex *MultiplexConfig `json:"multiplex,omitempty"`
	// OutboundTag for routing (auto-generated if empty).
	OutboundTag string `json:"tag,omitempty"`
}

// MultiplexConfig configures multiplexing for Shadowsocks.
type MultiplexConfig struct {
	Enabled  bool   `json:"enabled"`
	Protocol string `json:"protocol,omitempty"`
	MaxConn  int    `json:"max_connections,omitempty"`
	MinStrm  int    `json:"min_streams,omitempty"`
}

// BuildOutbound produces a sing-box outbound JSON object for a Shadowsocks connection.
func (c Config) BuildOutbound() (map[string]any, error) {
	if c.Server == "" {
		return nil, fmt.Errorf("shadowsocks: server is required")
	}
	if c.ServerPort == 0 {
		return nil, fmt.Errorf("shadowsocks: server_port is required")
	}
	if c.Password == "" {
		return nil, fmt.Errorf("shadowsocks: password is required")
	}
	if c.Method == "" {
		return nil, fmt.Errorf("shadowsocks: method is required")
	}

	tag := c.OutboundTag
	if tag == "" {
		tag = fmt.Sprintf("ss-%s-%d", c.Server, c.ServerPort)
	}

	outbound := map[string]any{
		"type":        "shadowsocks",
		"tag":         tag,
		"server":      c.Server,
		"server_port": c.ServerPort,
		"method":      c.Method,
		"password":    c.Password,
	}

	if c.Plugin != "" {
		outbound["plugin"] = c.Plugin
	}
	if c.PluginOpts != "" {
		outbound["plugin_opts"] = c.PluginOpts
	}
	if c.UDP {
		outbound["udp"] = true
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
	return fmt.Sprintf("ss-%s-%d", c.Server, c.ServerPort)
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
