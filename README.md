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

## Getting Started

### Prerequisites

- Go 1.21 or higher

### Installation

The repository consists of two branches:
- `peer` - For running peer nodes
- `bootstrap` - For running bootstrap nodes

#### For Peer Nodes

```bash
# Clone the repository and checkout the peer branch
git clone https://github.com/WeirdFlexButOk7/Decentralised-P2P-Network.git
cd Decentralised-P2P-Network
git checkout peer

# Install dependencies
go mod tidy
```

#### For Bootstrap Nodes

```bash
# Clone the repository and checkout the bootstrap branch
git clone https://github.com/WeirdFlexButOk7/Decentralised-P2P-Network.git
cd Decentralised-P2P-Network
git checkout bootstrap

# Install dependencies
go mod tidy
```

## Running the Network

### Bootstrap Nodes

Bootstrap nodes serve as initial entry points for the network. 

**Note:** The bootstrap nodes listed below are not currently running. If you want to set up your own bootstrap nodes:

1. Clone the repository and checkout the `bootstrap` branch
2. Create Azure VMs (or any cloud VMs) with public IP addresses
3. Configure inbound rules for TCP and QUIC connections on any port
4. Generate and keep a fixed key file (e.g., `hi.dat`, `hi2.dat`, `hi3.dat`) for each node
4. Run the bootstrap node commands below on each VM and note down each multiaddrs

Here are example commands for running three bootstrap nodes that was initially implemented:

**Bootstrap Node 1 (VM1):**
```bash
GOLOG_LOG_LEVEL="dcutr=debug,autorelay=debug,relay=debug,autonat=debug,relaysvc=debug" go run . --key=hi.dat --type=bootstrap
```

**Bootstrap Node 2 (VM2):**
```bash
GOLOG_LOG_LEVEL="dcutr=debug,autorelay=debug,relay=debug,autonat=debug,relaysvc=debug" go run . --key=hi2.dat --type=bootstrap
```

**Bootstrap Node 3 (VM3):**
```bash
GOLOG_LOG_LEVEL="dcutr=debug,autorelay=debug,relay=debug,autonat=debug,relaysvc=debug" go run . --key=hi3.dat --type=bootstrap
```

**Example Bootstrap Node Multiaddrs Observed (when running):**
- VM1: `/ip4/20.17.98.81/tcp/5090/p2p/12D3KooWAkBihGFKPyM2vfT3JmHMJ6RjdxxuZpJc9babUfoZAudX`
- VM2: `/ip4/20.17.97.234/tcp/5090/p2p/12D3KooWMXyuQoy9rbWHcu2YxfnySTWNPBHmE4fcmtzqimPgmeXz`
- VM3: `/ip4/20.189.122.207/tcp/5090/p2p/12D3KooWAytAc3tzXZo56LXddE2Cq3FVhWz2RpE6nZoN7LmGhq6p`

### Peer Nodes

Connect peer nodes to the network using the bootstrap nodes (replace with your own bootstrap node multiaddrs if you've set up your own):

```bash
go run . --bootstrap=/ip4/20.17.98.81/tcp/5090/p2p/12D3KooWAkBihGFKPyM2vfT3JmHMJ6RjdxxuZpJc9babUfoZAudX,/ip4/20.17.97.234/tcp/5090/p2p/12D3KooWMXyuQoy9rbWHcu2YxfnySTWNPBHmE4fcmtzqimPgmeXz,/ip4/20.189.122.207/tcp/5090/p2p/12D3KooWAytAc3tzXZo56LXddE2Cq3FVhWz2RpE6nZoN7LmGhq6p
```

## CLI Commands

Once the node is running, you can use these interactive commands:

- `send <peer-id> <message>` - Send a direct message to a specific peer
- `broadcast <message>` - Broadcast a message to all connected peers
- `peers` - List all currently connected peers
- `info` - Show node information (peer ID, addresses, connection stats)
- `type` - Display connection information for all connected peers (peer ID, direction, multiaddr)
- `knownpeers` - Discover and list all peers found via DHT peer discovery
- `quit` or `exit` - Gracefully shut down the node
