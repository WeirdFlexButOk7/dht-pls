# Decentralized P2P Network with libp2p

A fully decentralized peer-to-peer network implementation using libp2p in Go. This system provides robust P2P networking with NAT traversal, DHT-based peer discovery, and no single points of failure.

## Features

- **Fully Decentralized**: No central servers or single points of failure
- **NAT Traversal**: Automatic handling of firewalls and NATs using:
  - AutoNAT v2 for connectivity detection
  - Circuit Relay v2 for relay connections
  - DCUtR (Direct Connection Upgrade through Relay) for hole-punching
- **Peer Discovery**: Kademlia DHT for distributed peer routing
- **Security**: Multiple security transports (TLS 1.3, Noise)
- **Multiple Transports**: TCP and QUIC support
- **Connection Management**: Automatic connection pruning and peer scoring
- **Messaging Protocol**: Built-in P2P messaging with broadcasts

## Architecture

```
.
├── config/          # Configuration management
├── node/            # Core libp2p host and DHT logic
├── protocols/       # Application-level protocols (messaging)
├── utils/           # Utility functions (key management)
└── main.go          # CLI application entry point
```

## Getting Started

### Prerequisites

- Go 1.21 or higher
- Internet connection for bootstrap peers

### Installation

```bash
# Install dependencies
go mod tidy

# Build the application
go build -o dht-p2p
```

### Running a Node

**Start with default settings:**
```bash
go run main.go
```

**Start with custom port:**
```bash
go run main.go -port 4001
```

**Start with persistent identity:**
```bash
go run main.go -key node-key.bin
```

**Start in DHT server mode:**
```bash
go run main.go -dht-mode server
```

**Connect to custom bootstrap peers:**
```bash
go run main.go -bootstrap "/ip4/192.168.1.100/tcp/4001/p2p/QmPeerId,/ip4/192.168.1.101/tcp/4001/p2p/QmPeerId2"
```

**Create a private network:**
```bash
go run main.go -private "my-secret-network-key"
```

## CLI Commands

Once the node is running, you can use these interactive commands:

- `send <peer-id> <message>` - Send a message to a specific peer
- `broadcast <message>` - Broadcast a message to all connected peers
- `peers` - List all connected peers
- `info` - Show node information (peer ID, addresses, stats)
- `quit` or `exit` - Gracefully shut down the node

## How It Works

### 1. Node Initialization
- Generates or loads Ed25519 key pair for peer identity
- Configures libp2p with security transports (TLS, Noise)
- Sets up TCP and QUIC transport protocols
- Initializes connection manager for resource optimization

### 2. NAT Traversal
- **AutoNAT v2**: Automatically detects if node is behind NAT
- **Circuit Relay v2**: Allows relayed connections through public nodes
- **Hole Punching (DCUtR)**: Attempts direct connections even through NATs
- **Port Mapping**: Uses UPnP/NAT-PMP when available

### 3. Peer Discovery
- Bootstraps DHT by connecting to known peers
- Periodically refreshes routing table
- Discovers new peers through DHT queries
- Maintains connections to optimal peers

### 4. Messaging
- Custom protocol (`/dht-p2p/message/1.0.0`)
- JSON-based message serialization
- Acknowledgment system for delivery confirmation
- Broadcast support for network-wide messages

## Configuration

Key configuration options in `config/config.go`:

- **ListenAddresses**: Network interfaces and ports to listen on
- **BootstrapPeers**: Initial peers for network discovery
- **Connection Limits**: Low/high watermarks for connection management
- **DHT Mode**: Server (always reachable), Client, or Auto
- **NAT Features**: Enable/disable AutoNAT, Relay, Hole-punching

## Security Considerations

- **Transport Security**: All connections use TLS 1.3 or Noise protocol
- **Peer Identity**: Each node has a cryptographic identity (Ed25519)
- **Private Networks**: Optional shared secret for network isolation
- **Connection Limits**: Prevents resource exhaustion attacks

## Network Topology

The network is fully decentralized with no central authority:
- Any node can join by connecting to bootstrap peers
- Bootstrap peers are only used for initial discovery
- DHT ensures peer discovery continues even if bootstrap nodes fail
- Relay nodes provide connectivity for NAT-constrained peers

## Troubleshooting

**No peers connecting:**
- Check firewall settings
- Verify bootstrap peers are reachable
- Try running in DHT server mode if you have a public IP

**Behind strict NAT:**
- Enable relay: The node will automatically use relay connections
- Try running on a VPS as a relay node for your network

**Connection limits reached:**
- Adjust `LowWater` and `HighWater` in config
- Increase system file descriptor limits

## Development

**Add a new protocol:**
1. Create a new file in `protocols/`
2. Define protocol ID and message types
3. Implement stream handlers
4. Register handler in main.go

**Customize DHT behavior:**
- Modify `node/host.go`
- Adjust DHT options in `InitDHT()`

## License

MIT License - see LICENSE file for details

## Contributing

Contributions welcome! Please open an issue or submit a pull request.

## References

- [libp2p Documentation](https://docs.libp2p.io/)
- [Kademlia DHT Paper](https://pdos.csail.mit.edu/~petar/papers/maymounkov-kademlia-lncs.pdf)
- [NAT Traversal in libp2p](https://blog.ipfs.tech/2022-01-20-libp2p-hole-punching/)
