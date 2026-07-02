// Package tuic provides a sing-box outbound configuration for the TUIC protocol.
package tuic

import (
	"encoding/json"
	"fmt"
)

// Config represents the configuration for a TUIC outbound in sing-box.
type Config struct {
	// Server address (IP or hostname).
	Server string `json:"server"`
	// Server port.
	ServerPort uint16 `json:"server_port"`
	// TUIC token (v4) or UUID (v5) for authentication.
	Token string `json:"token,omitempty"`
	// UDP relay mode ("native", "quic").
	UDPRelayMode string `json:"udp_relay_mode,omitempty"`
	// Congestion control algorithm ("cubic", "bbr", "new_reno").
	CongestionControl string `json:"congestion_control,omitempty"`
	// Heartbeat interval in seconds.
	HeartbeatInterval int `json:"heartbeat_interval,omitempty"`
	// Disable sniffing for this outbound.
	DisableSniffing bool `json:"disable_sniffing,omitempty"`
	// Zero RTT handshake.
	ZeroRTTHandshake bool `json:"zero_rtt_handshake,omitempty"`
	// Multiplex settings.
	Multiplex *MultiplexConfig `json:"multiplex,omitempty"`
	// TLS settings.
	TLS *TLSConfig `json:"tls,omitempty"`
	// OutboundTag for routing (auto-generated if empty).
	OutboundTag string `json:"tag,omitempty"`
}

// MultiplexConfig configures multiplexing for TUIC.
type MultiplexConfig struct {
	Enabled  bool   `json:"enabled"`
	Protocol string `json:"protocol,omitempty"`
	MaxConn  int    `json:"max_connections,omitempty"`
	MinStrm  int    `json:"min_streams,omitempty"`
}

// TLSConfig configures TLS for TUIC outbound.
type TLSConfig struct {
	Enabled     bool     `json:"enabled"`
	ServerName  string   `json:"server_name,omitempty"`
	Insecure    bool     `json:"insecure,omitempty"`
	ALPN        []string `json:"alpn,omitempty"`
	MinVersion  string   `json:"min_version,omitempty"`
	Certificate []byte   `json:"certificate,omitempty"`
}

// BuildOutbound produces a sing-box outbound JSON object for a TUIC connection.
func (c Config) BuildOutbound() (map[string]any, error) {
	if c.Server == "" {
		return nil, fmt.Errorf("tuic: server is required")
	}
	if c.ServerPort == 0 {
		return nil, fmt.Errorf("tuic: server_port is required")
	}
	if c.Token == "" {
		return nil, fmt.Errorf("tuic: token is required")
	}

	tag := c.OutboundTag
	if tag == "" {
		tag = fmt.Sprintf("tuic-%s-%d", c.Server, c.ServerPort)
	}

	outbound := map[string]any{
		"type":        "tuic",
		"tag":         tag,
		"server":      c.Server,
		"server_port": c.ServerPort,
		"token":       c.Token,
	}

	if c.UDPRelayMode != "" {
		outbound["udp_relay_mode"] = c.UDPRelayMode
	}
	if c.CongestionControl != "" {
		outbound["congestion_control"] = c.CongestionControl
	}
	if c.HeartbeatInterval > 0 {
		outbound["heartbeat_interval"] = c.HeartbeatInterval
	}
	if c.DisableSniffing {
		outbound["disable_sniffing"] = true
	}
	if c.ZeroRTTHandshake {
		outbound["zero_rtt_handshake"] = true
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

	// TLS
	if c.TLS != nil && c.TLS.Enabled {
		tlsMap := map[string]any{
			"enabled": true,
		}
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
	} else {
		outbound["tls"] = map[string]any{
			"enabled": false,
		}
	}

	return outbound, nil
}

// Tag implements the singbox.Outbound interface.
func (c Config) Tag() string {
	if c.OutboundTag != "" {
		return c.OutboundTag
	}
	return fmt.Sprintf("tuic-%s-%d", c.Server, c.ServerPort)
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
