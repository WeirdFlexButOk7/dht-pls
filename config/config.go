package config

import (
	"time"
)

// Config holds all configuration for the P2P network
type Config struct {
	// Network configuration
	ListenAddresses []string
	BootstrapPeers  []string

	// Connection management
	LowWater    int
	HighWater   int
	GracePeriod time.Duration

	// DHT configuration
	DHTMode string // "server", "client", "auto"

	// NAT configuration
	EnableAutoNAT   bool
	EnableRelay     bool
	EnableAutoRelay bool
	EnableHolePunch bool

	// Node identity
	PrivateKeyPath string

	// Network type
	ProtectKey string // For private networks (optional)
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig(nodeType *string) *Config {
	if(*nodeType == "bootstrap") {
		return &Config{
			ListenAddresses: []string{
				"/ip4/0.0.0.0/tcp/5090",
				"/ip4/0.0.0.0/udp/5090/quic-v1",
				"/ip6/::/tcp/5090",
				"/ip6/::/udp/5090/quic-v1",
			},
			BootstrapPeers: []string{
				// IPFS bootstrap nodes for initial connectivity
				// "/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
				// "/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
				// "/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
				// "/dnsaddr/bootstrap.libp2p.io/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
				// "/dnsaddr/va1.bootstrap.libp2p.io/p2p/12D3KooWKnDdG3iXw9eTFijk3EWSunZcFi54Zka4wmtqtt6rPxc8",
			},
			LowWater:        50,
			HighWater:       100,
			GracePeriod:     time.Minute,
			DHTMode:         "auto",
			EnableAutoNAT:   true,
			EnableRelay:     true,
			EnableAutoRelay: false,
			EnableHolePunch: false,
			PrivateKeyPath:  "",
			ProtectKey:      "",
		}
	}
	return &Config{
		ListenAddresses: []string{
			"/ip4/0.0.0.0/tcp/0",
			"/ip4/0.0.0.0/udp/0/quic-v1",
			"/ip6/::/tcp/0",
			"/ip6/::/udp/0/quic-v1",
		},
		BootstrapPeers: []string{
			// IPFS bootstrap nodes for initial connectivity
			// "/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
			// "/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
			// "/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
			// "/dnsaddr/bootstrap.libp2p.io/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
			// "/dnsaddr/va1.bootstrap.libp2p.io/p2p/12D3KooWKnDdG3iXw9eTFijk3EWSunZcFi54Zka4wmtqtt6rPxc8",
		},
		LowWater:        50,
		HighWater:       100,
		GracePeriod:     time.Minute,
		DHTMode:         "auto",
		EnableAutoNAT:   true,
		EnableRelay:     false,
		EnableAutoRelay: true,
		EnableHolePunch: true,
		PrivateKeyPath:  "",
		ProtectKey:      "",
	}
}
