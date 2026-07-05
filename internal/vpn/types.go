// Package vpn provides abstractions for managing VPN providers and interfaces.
package vpn

import (
	"context"
	"time"
)

// ProviderType enumerates supported VPN protocols.
type ProviderType string

const (
	ProviderAmneziaWG   ProviderType = "amneziawg"
	ProviderWireGuard   ProviderType = "wireguard"
	ProviderVLESS       ProviderType = "vless"
	ProviderHysteria2   ProviderType = "hysteria2"
	ProviderTUIC        ProviderType = "tuic"
	ProviderShadowsocks ProviderType = "shadowsocks"
)

// InterfaceState represents the current state of a VPN interface.
type InterfaceState string

const (
	StateUnknown InterfaceState = "unknown"
	StateUp      InterfaceState = "up"
	StateDown    InterfaceState = "down"
	StateError   InterfaceState = "error"
)

// InterfaceStatus contains runtime information about a VPN interface.
type InterfaceStatus struct {
	Name          string         `json:"name"`
	Type          ProviderType   `json:"type"`
	State         InterfaceState `json:"state"`
	Error         string         `json:"error,omitempty"`
	PublicKey     string         `json:"public_key,omitempty"`
	LocalAddress  string         `json:"local_address,omitempty"`
	Endpoint      string         `json:"endpoint,omitempty"`
	TxBytes       int64          `json:"tx_bytes"`
	RxBytes       int64          `json:"rx_bytes"`
	LastHandshake time.Time      `json:"last_handshake,omitempty"`
	Uptime        time.Duration  `json:"uptime,omitempty"`
}

// Peer represents a VPN peer configuration.
type Peer struct {
	PublicKey           string   `json:"public_key"`
	PresharedKey        string   `json:"preshared_key,omitempty"`
	Endpoint            string   `json:"endpoint"`
	AllowedIPs          []string `json:"allowed_ips"`
	PersistentKeepalive int      `json:"persistent_keepalive,omitempty"`
}

// Provider defines the interface that all VPN provider implementations must satisfy.
type Provider interface {
	// Type returns the VPN protocol type.
	Type() ProviderType

	// Name returns a human-readable name for this provider instance.
	Name() string

	// ApplyConfig creates or updates the VPN interface with the given configuration.
	ApplyConfig(ctx context.Context, cfg interface{}) error

	// Remove tears down the VPN interface.
	Remove(ctx context.Context) error

	// Status returns the current runtime status of the interface.
	Status(ctx context.Context) (*InterfaceStatus, error)

	// HealthCheck performs a connectivity check through the VPN interface.
	// Returns nil if the connection is healthy.
	HealthCheck(ctx context.Context) error
}
