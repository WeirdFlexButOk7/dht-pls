# Quick Start Guide

## Build and Run

### Build the application:
```bash
go build -o dht-p2p.exe .
```

### Run your first node:
```bash
.\dht-p2p.exe
```

### Run with a specific port:
```bash
.\dht-p2p.exe -port 4001
```

### Run with persistent identity:
```bash
.\dht-p2p.exe -key my-node-key.bin
```

## Connecting Multiple Nodes

### Terminal 1 (First Node):
```bash
.\dht-p2p.exe -port 4001 -key node1.bin
```
Copy one of the multiaddresses that appears (e.g., `/ip4/127.0.0.1/tcp/4001/p2p/QmXXXXXX...`)

### Terminal 2 (Second Node):
```bash
.\dht-p2p.exe -port 4002 -key node2.bin -bootstrap "/ip4/127.0.0.1/tcp/4001/p2p/QmXXXXXX..."
```

## CLI Commands

Once running, try these commands:
- `peers` - List connected peers
- `info` - Show node information
- `broadcast Hello everyone!` - Send a message to all peers
- `send <peer-id> Hello!` - Send a message to a specific peer
- `quit` - Exit

## Example Session

```
> info
Peer ID: 12D3KooWA1B2C3D4E5F6...
Addresses:
   /ip4/127.0.0.1/tcp/4001/p2p/12D3KooWA1B2C3D4E5F6...
   /ip4/192.168.1.100/tcp/4001/p2p/12D3KooWA1B2C3D4E5F6...
DHT Mode: auto
Connected Peers: 3

> peers
Connected peers: 3
   - 12D3KooWXYZ...
   - 12D3KooWABC...
   - 12D3KooWDEF...

> broadcast Hello network!
✅ Message broadcasted

> quit
Shutting down...
```

## Network Features

✅ **NAT Traversal**: Automatically handles firewalls using relay and hole-punching  
✅ **Peer Discovery**: Finds peers automatically through Kademlia DHT  
✅ **Security**: All connections use TLS 1.3 or Noise encryption  
✅ **Resilience**: No single point of failure, fully decentralized  

## Troubleshooting

**Can't connect to peers?**
- Check your firewall settings
- Try running one node in server mode: `-dht-mode server`
- Make sure bootstrap addresses are correct

**Node not listening on expected port?**
- Port 0 means random port assignment (default)
- Specify `-port <number>` to use a specific port

## Next Steps

- Modify `protocols/messaging.go` to add custom protocols
- Adjust connection limits in `config/config.go`
- Add your application logic on top of the P2P network

See [README.md](README.md) for full documentation.
