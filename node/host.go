package node

import (
	"context"
	"fmt"
	"sync"

	"dht-p2p/config"
	"dht-p2p/utils"

	"github.com/libp2p/go-libp2p"
  "github.com/libp2p/go-libp2p/p2p/host/autorelay"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"

	"github.com/multiformats/go-multiaddr"
)

// Node represents a P2P node in the network
type Node struct {
	Host       host.Host
	DHT        *dht.IpfsDHT
	Config     *config.Config
	ctx        context.Context
	cancel     context.CancelFunc
	closeMutex sync.Mutex
}

// mustCreateConnManager creates a connection manager or panics
func mustCreateConnManager(cfg *config.Config) *connmgr.BasicConnMgr {
	cm, err := connmgr.NewConnManager(
		cfg.LowWater,
		cfg.HighWater,
		connmgr.WithGracePeriod(cfg.GracePeriod),
	)
	if err != nil {
		panic(fmt.Sprintf("failed to create connection manager: %v", err))
	}
	return cm
}

// NewNode creates and initializes a new P2P node
func NewNode(ctx context.Context, cfg *config.Config) (*Node, error) {
	nodeCtx, cancel := context.WithCancel(ctx)

	// Load or generate node identity
	priv, err := utils.LoadOrGenerateKey(cfg.PrivateKeyPath)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to load/generate key: %w", err)
	}

	// Build libp2p options
	opts := []libp2p.Option{
		libp2p.Identity(priv),
		libp2p.ListenAddrStrings(cfg.ListenAddresses...),

		// Security transports (multiple for flexibility)
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		libp2p.Security(noise.ID, noise.New),

		// Transport protocols
		libp2p.DefaultTransports,

		// Connection manager for resource management
		libp2p.ConnectionManager(mustCreateConnManager(cfg)),

	}

	if cfg.EnableAutoRelay {
		var relays []peer.AddrInfo

		for _, addr := range cfg.BootstrapPeers {
			ai, err := peer.AddrInfoFromString(addr)
			if err != nil {
				continue
			}
			relays = append(relays, *ai)
		}

		opts = append(opts,
			libp2p.EnableAutoRelay(
				autorelay.WithStaticRelays(relays),
			),
		)

	}

	// Add private network protection if configured
	if cfg.ProtectKey != "" {
		opts = append(opts, libp2p.PrivateNetwork([]byte(cfg.ProtectKey)))
	}

	// Enable AutoNAT if configured
	if cfg.EnableAutoNAT {
		opts = append(opts, libp2p.EnableAutoNATv2())
	}

	// Enable hole punching if configured
	if cfg.EnableHolePunch {
		opts = append(opts, libp2p.EnableHolePunching())
	}

	// Enable relay service if configured
	if cfg.EnableRelay {
		opts = append(opts, libp2p.EnableRelayService())
	}

	// Create the libp2p host
	h, err := libp2p.New(opts...)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	node := &Node{
		Host:   h,
		Config: cfg,
		ctx:    nodeCtx,
		cancel: cancel,
	}

	return node, nil
}

// InitDHT initializes the Kademlia DHT for peer discovery
func (n *Node) InitDHT(ctx context.Context) error {
	var mode dht.ModeOpt
	switch n.Config.DHTMode {
	case "server":
		mode = dht.ModeServer
	case "client":
		mode = dht.ModeClient
	default:
		mode = dht.ModeAuto
	}

	// Create DHT with proper options
	kadDHT, err := dht.New(ctx, n.Host,
		dht.Mode(mode),
		dht.BootstrapPeers(n.parseBootstrapPeers()...),
		dht.ProtocolPrefix("/dht-p2p"),
	)
	if err != nil {
		return fmt.Errorf("failed to create DHT: %w", err)
	}

	n.DHT = kadDHT

	// Bootstrap the DHT
	if err := kadDHT.Bootstrap(ctx); err != nil {
		return fmt.Errorf("failed to bootstrap DHT: %w", err)
	}

	return nil
}

// parseBootstrapPeers converts string addresses to peer.AddrInfo
func (n *Node) parseBootstrapPeers() []peer.AddrInfo {
	var peers []peer.AddrInfo
	for _, addrStr := range n.Config.BootstrapPeers {
		addr, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			continue
		}

		peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			continue
		}

		peers = append(peers, *peerInfo)
	}
	return peers
}

// Connect connects to bootstrap peers and starts peer discovery
func (n *Node) Connect(ctx context.Context) error {
	// Connect to bootstrap peers
	var wg sync.WaitGroup
	peers := n.parseBootstrapPeers()

	for _, peerInfo := range peers {
		wg.Add(1)
		go func(pi peer.AddrInfo) {
			defer wg.Done()
			if err := n.Host.Connect(ctx, pi); err != nil {
				// Log error but don't fail - we only need some bootstrap nodes
				fmt.Printf("Failed to connect to bootstrap peer %s: %v\n", pi.ID, err)
			} else {
				fmt.Printf("Connected to bootstrap peer: %s\n", pi.ID)
			}
		}(peerInfo)
	}

	wg.Wait()
	return nil
}

// RoutingDiscovery returns the routing discovery interface
func (n *Node) RoutingDiscovery() routing.Routing {
	return n.DHT
}

// GetAddresses returns all listening addresses of the node
func (n *Node) GetAddresses() []string {
	var addrs []string
	for _, addr := range n.Host.Addrs() {
		addrs = append(addrs, fmt.Sprintf("%s/p2p/%s", addr, n.Host.ID()))
	}
	return addrs
}

// SetStreamHandler sets a handler for a specific protocol
func (n *Node) SetStreamHandler(protocolID string, handler network.StreamHandler) {
	n.Host.SetStreamHandler(protocol.ID(protocolID), handler)
}

// Close shuts down the node gracefully
func (n *Node) Close() error {
	n.closeMutex.Lock()
	defer n.closeMutex.Unlock()

	// Close DHT if initialized
	if n.DHT != nil {
		if err := n.DHT.Close(); err != nil {
			fmt.Printf("Error closing DHT: %v\n", err)
		}
	}

	// Close the host
	if err := n.Host.Close(); err != nil {
		return fmt.Errorf("failed to close host: %w", err)
	}

	n.cancel()
	return nil
}
