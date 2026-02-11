# Testground Test Plans for Decentralised P2P Network

This directory contains comprehensive Testground test plans for testing the P2P network at scale.

## Overview

The test plans are designed to thoroughly test various aspects of the P2P network including:

1. **Peer Discovery** - Test DHT-based peer discovery mechanisms
2. **Direct Messaging** - Test point-to-point message delivery
3. **Broadcast** - Test message propagation across all peers
4. **DHT Performance** - Benchmark DHT operations (store, retrieve, lookup)
5. **Connection Stress** - Test connection churn and reconnection resilience
6. **Network Conditions** - Test under various network constraints (latency, jitter, packet loss)
7. **NAT Traversal** - Test relay and hole-punching capabilities
8. **Scalability** - Test with large numbers of nodes (100-1000+)

## Prerequisites

1. **Install Testground**

```bash
git clone https://github.com/testground/testground.git
cd testground
make install
```

2. **Start Testground Daemon**

```bash
testground daemon
```

## Installation

Import the test plan into Testground:

```bash
# From the repository root
cd Decentralised-P2P-Network

# Import the test plan
testground plan import --from ./testplans/p2p-network --name p2p-network
```

## Running Tests

### Quick Start

Run a basic peer discovery test with 5 nodes:

```bash
testground run single \
  --plan=p2p-network \
  --testcase=peer-discovery \
  --builder=docker:go \
  --runner=local:docker \
  --instances=5
```

### Test Cases

#### 1. Peer Discovery

Tests the DHT-based peer discovery mechanism.

```bash
testground run single \
  --plan=p2p-network \
  --testcase=peer-discovery \
  --builder=docker:go \
  --runner=local:docker \
  --instances=10 \
  --test-param bootstrap_count=2 \
  --test-param target_peers=5 \
  --test-param discovery_timeout=60s
```

**Parameters:**
- `bootstrap_count`: Number of bootstrap nodes (default: 1)
- `target_peers`: Target number of peers to discover (default: 3)
- `discovery_timeout`: Timeout for peer discovery (default: 60s)

#### 2. Direct Messaging

Tests point-to-point message delivery between peers.

```bash
testground run single \
  --plan=p2p-network \
  --testcase=direct-messaging \
  --builder=docker:go \
  --runner=local:docker \
  --instances=5 \
  --test-param message_count=100 \
  --test-param message_size=1024 \
  --test-param concurrent_sends=5
```

**Parameters:**
- `message_count`: Number of messages to send (default: 10)
- `message_size`: Size of each message in bytes (default: 1024)
- `concurrent_sends`: Number of concurrent senders (default: 1)

#### 3. Broadcast Test

Tests message broadcasting to all connected peers.

```bash
testground run single \
  --plan=p2p-network \
  --testcase=broadcast-test \
  --builder=docker:go \
  --runner=local:docker \
  --instances=10 \
  --test-param broadcast_count=5 \
  --test-param broadcast_interval=2s
```

**Parameters:**
- `broadcast_count`: Number of broadcasts per node (default: 5)
- `broadcast_interval`: Interval between broadcasts (default: 2s)

#### 4. DHT Performance

Benchmarks DHT operations including store, retrieve, and peer lookups.

```bash
testground run single \
  --plan=p2p-network \
  --testcase=dht-performance \
  --builder=docker:go \
  --runner=local:docker \
  --instances=20 \
  --test-param lookup_count=10 \
  --test-param store_count=5
```

**Parameters:**
- `lookup_count`: Number of DHT lookups (default: 10)
- `store_count`: Number of values to store (default: 5)

#### 5. Connection Stress

Tests network resilience under connection churn.

```bash
testground run single \
  --plan=p2p-network \
  --testcase=connection-stress \
  --builder=docker:go \
  --runner=local:docker \
  --instances=50 \
  --test-param connection_churn=20 \
  --test-param churn_interval=30s \
  --test-param test_duration=5m
```

**Parameters:**
- `connection_churn`: Percentage of peers to disconnect/reconnect (default: 20)
- `churn_interval`: Interval for connection churn (default: 30s)
- `test_duration`: Total test duration (default: 5m)

#### 6. Network Conditions

Tests performance under various network conditions.

```bash
testground run single \
  --plan=p2p-network \
  --testcase=network-conditions \
  --builder=docker:go \
  --runner=local:docker \
  --instances=10 \
  --test-param latency_ms=100 \
  --test-param jitter_ms=10 \
  --test-param bandwidth_mb=10 \
  --test-param packet_loss=0.5
```

**Parameters:**
- `latency_ms`: Network latency in milliseconds (default: 100)
- `jitter_ms`: Network jitter in milliseconds (default: 10)
- `bandwidth_mb`: Bandwidth limit in Mbps (default: 10)
- `packet_loss`: Packet loss percentage (default: 0.0)

#### 7. NAT Traversal

Tests relay and hole-punching capabilities.

```bash
testground run single \
  --plan=p2p-network \
  --testcase=nat-traversal \
  --builder=docker:go \
  --runner=local:docker \
  --instances=10 \
  --test-param relay_count=2 \
  --test-param nat_type=symmetric
```

**Parameters:**
- `relay_count`: Number of relay nodes (default: 2)
- `nat_type`: Type of NAT simulation (default: "symmetric")

#### 8. Scalability

Tests the network with a large number of nodes.

```bash
testground run single \
  --plan=p2p-network \
  --testcase=scalability \
  --builder=docker:go \
  --runner=local:docker \
  --instances=100 \
  --test-param ramp_up_duration=2m \
  --test-param steady_duration=5m \
  --test-param messages_per_node=50
```

**Parameters:**
- `ramp_up_duration`: Time to ramp up all nodes (default: 2m)
- `steady_duration`: Duration to maintain steady state (default: 5m)
- `messages_per_node`: Messages each node should send (default: 10)

## Large-Scale Testing

For testing with more than 50 instances, use the Kubernetes runner:

1. **Set up a Kubernetes cluster** (see Testground docs)

2. **Run large-scale test:**

```bash
testground run single \
  --plan=p2p-network \
  --testcase=scalability \
  --builder=docker:go \
  --runner=cluster:k8s \
  --instances=500
```

## Viewing Results

After running a test, Testground will output a directory with results:

```bash
# Results are typically in ~/.testground/outputs/
ls -la ~/.testground/outputs/

# View metrics
cat ~/.testground/outputs/<run-id>/run-events.out
```

## Metrics Collected

Each test case collects various metrics including:

- **Peer Discovery:** peers_discovered, discovery_duration_ms, connection_time
- **Messaging:** messages_sent, messages_received, latency_ms, success_rate
- **DHT:** lookup_duration, store_duration, routing_table_size
- **Connections:** active_connections, reconnect_success_rate
- **Network:** message_latency_ms, connection_success_rate

## Troubleshooting

### Docker Issues

If you encounter Docker-related errors:

```bash
# Ensure Docker is running
docker ps

# Clean up old Testground containers
docker system prune -a
```

### Module Issues

If you get module dependency errors:

```bash
cd testplans/p2p-network
go mod tidy
go mod download
```

### Memory Issues

For large-scale tests, increase Docker memory:

- Docker Desktop: Settings → Resources → Memory (recommend 8GB+)

## Integration with CI/CD

You can integrate these tests into your CI pipeline:

```yaml
# Example GitHub Actions workflow
- name: Run Testground Tests
  run: |
    testground run single \
      --plan=p2p-network \
      --testcase=peer-discovery \
      --instances=5 \
      --collect
```

## Further Reading

- [Testground Documentation](https://docs.testground.ai/)
- [libp2p Test Plans](https://github.com/libp2p/test-plans)
- [Writing Test Plans](https://docs.testground.ai/v/master/writing-test-plans)

## Contributing

To add new test cases:

1. Create a new `.go` file in the test plan directory
2. Implement the test case function
3. Add the test case to `manifest.toml`
4. Update `main.go` to include the new test case
5. Update this README with usage instructions

## License

Same as the parent project.
