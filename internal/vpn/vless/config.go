// Package vless provides a sing-box outbound configuration for the VLESS protocol.
package vless

import (
	"encoding/json"
	"fmt"
)

// Flow types for VLESS
const (
	FlowNone        = ""
	FlowXTLSRPRX    = "xtls-rprx-vision"
	FlowXTLSRPRXUDP = "xtls-rprx-vision-udp443"
)

// Config represents the configuration for a VLESS outbound in sing-box.
type Config struct {
	// Server address (IP or hostname).
	Server string `json:"server"`
	// Server port.
	ServerPort uint16 `json:"server_port"`
	// UUID / VLESS user ID.
	UUID string `json:"uuid"`
	// VLESS flow (e.g. "xtls-rprx-vision", "xtls-rprx-vision-udp443").
	Flow string `json:"flow,omitempty"`
	// Optional packet encoding ("xudp", "packetaddr", or empty).
	PacketEncoding string `json:"packet_encoding,omitempty"`
	// Optional multiplex settings.
	Multiplex *MultiplexConfig `json:"multiplex,omitempty"`
	// TLS settings.
	TLS *TLSConfig `json:"tls,omitempty"`
	// OutboundTag for routing (auto-generated if empty).
	OutboundTag string `json:"tag,omitempty"`
}

// MultiplexConfig configures multiplexing for the connection.
type MultiplexConfig struct {
	Enabled  bool   `json:"enabled"`
	Protocol string `json:"protocol,omitempty"` // "h2mux", "smux", "yamux"
	MaxConn  int    `json:"max_connections,omitempty"`
	MinStrm  int    `json:"min_streams,omitempty"`
}

// TLSConfig configures TLS for the outbound.
type TLSConfig struct {
	Enabled    bool     `json:"enabled"`
	ServerName string   `json:"server_name,omitempty"`
	Insecure   bool     `json:"insecure,omitempty"`
	ALPN       []string `json:"alpn,omitempty"`
	MinVersion string   `json:"min_version,omitempty"`
}

// BuildOutbound produces a sing-box outbound JSON object for a VLESS connection.
func (c Config) BuildOutbound() (map[string]any, error) {
	if c.Server == "" {
		return nil, fmt.Errorf("vless: server is required")
	}
	if c.ServerPort == 0 {
		return nil, fmt.Errorf("vless: server_port is required")
	}
	if c.UUID == "" {
		return nil, fmt.Errorf("vless: uuid is required")
	}

	tag := c.OutboundTag
	if tag == "" {
		tag = fmt.Sprintf("vless-%s-%d", c.Server, c.ServerPort)
	}

	outbound := map[string]any{
		"type":        "vless",
		"tag":         tag,
		"server":      c.Server,
		"server_port": c.ServerPort,
		"uuid":        c.UUID,
	}

	if c.Flow != "" {
		outbound["flow"] = c.Flow
	}
	if c.PacketEncoding != "" {
		outbound["packet_encoding"] = c.PacketEncoding
	}
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
		outbound["tls"] = tlsMap
	}
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
	return fmt.Sprintf("vless-%s-%d", c.Server, c.ServerPort)
}

// MarshalJSON implements json.Marshaler for storage in database.
func (c Config) MarshalJSON() ([]byte, error) {
	// Use plain struct to avoid recursion
	type alias Config
	return json.Marshal(alias(c))
}

// UnmarshalJSON implements json.Unmarshaler for loading from database.
func (c *Config) UnmarshalJSON(data []byte) error {
	type alias Config
	return json.Unmarshal(data, (*alias)(c))
}
