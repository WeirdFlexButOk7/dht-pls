package main

import (
	"context"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/testground/sdk-go/network"
	"github.com/testground/sdk-go/runtime"
	tsync "github.com/testground/sdk-go/sync"
)

func runNetworkConditions(runenv *runtime.RunEnv) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// Get test parameters
	latencyMS := runenv.IntParam("latency_ms")
	jitterMS := runenv.IntParam("jitter_ms")
	bandwidthMB := runenv.IntParam("bandwidth_mb")
	packetLoss := runenv.FloatParam("packet_loss")

	runenv.RecordMessage("Starting network conditions test")
	runenv.RecordMessage("Latency: %dms, Jitter: %dms, Bandwidth: %dMbps, Loss: %.2f%%",
		latencyMS, jitterMS, bandwidthMB, packetLoss)

	// Initialize network
	netclient := network.NewClient(runenv.SyncClient, runenv.RunParams())
	netclient.MustWaitNetworkInitialized(ctx)

	// Configure network conditions
	runenv.RecordMessage("Configuring network conditions...")

	config := &network.Config{
		Network: "default",
		Enable:  true,
		Default: network.LinkShape{
			Latency:   time.Duration(latencyMS) * time.Millisecond,
			Jitter:    time.Duration(jitterMS) * time.Millisecond,
			Bandwidth: uint64(bandwidthMB) * 1024 * 1024, // Convert MB to bytes
			Loss:      float32(packetLoss),
		},
		CallbackState:  tsync.State("network-configured"),
		CallbackTarget: runenv.TestInstanceCount,
		RoutingPolicy:  network.AllowAll,
	}

	if err := netclient.ConfigureNetwork(ctx, config); err != nil {
		return fmt.Errorf("failed to configure network: %w", err)
	}

	runenv.RecordMessage("Network conditions applied")

	// Create libp2p host
	h, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"),
	)
	if err != nil {
		return fmt.Errorf("failed to create host: %w", err)
	}
	defer h.Close()

	// Create DHT
	kadDHT, err := dht.New(ctx, h, dht.Mode(dht.ModeServer))
	if err != nil {
		return fmt.Errorf("failed to create DHT: %w", err)
	}
	defer kadDHT.Close()

	if err := kadDHT.Bootstrap(ctx); err != nil {
		return fmt.Errorf("failed to bootstrap DHT: %w", err)
	}

	runenv.RecordMessage("Node ID: %s", h.ID())

	// Get sync client
	client := runenv.SyncClient()

	// Publish our address
	addrInfo := &peer.AddrInfo{
		ID:    h.ID(),
		Addrs: h.Addrs(),
	}

	ai := tsync.NewSubtree(tsync.State("addrs"))
	client.MustPublish(ctx, ai.Key(h.ID().String()), addrInfo)

	// Wait for all nodes
	client.MustSignalAndWait(ctx, tsync.State("ready"), runenv.TestInstanceCount)

	// Collect peer addresses
	peers := make(map[peer.ID]*peer.AddrInfo)
	peerCh := make(chan *peer.AddrInfo, runenv.TestInstanceCount)

	sctx, scancel := context.WithTimeout(ctx, 30*time.Second)
	client.Subscribe(sctx, ai, peerCh)
	scancel()

	for i := 0; i < runenv.TestInstanceCount; i++ {
		select {
		case peerAddr := <-peerCh:
			if peerAddr.ID != h.ID() {
				peers[peerAddr.ID] = peerAddr
			}
		case <-time.After(5 * time.Second):
			break
		}
	}

	runenv.RecordMessage("Found %d peers", len(peers))

	// Test connections under network conditions
	runenv.RecordMessage("Testing connections with network conditions...")

	connectedCount := 0
	failedCount := 0
	totalConnTime := time.Duration(0)

	for _, peerAddr := range peers {
		startTime := time.Now()

		if err := h.Connect(ctx, *peerAddr); err != nil {
			runenv.RecordMessage("Failed to connect to %s: %v", peerAddr.ID, err)
			failedCount++
			runenv.R().RecordPoint("connection_failures", 1.0)
		} else {
			connTime := time.Since(startTime)
			totalConnTime += connTime
			connectedCount++

			runenv.RecordMessage("Connected to %s in %s", peerAddr.ID, connTime)
			runenv.R().RecordPoint("connection_time_ms", float64(connTime.Milliseconds()))
		}
	}

	avgConnTime := time.Duration(0)
	if connectedCount > 0 {
		avgConnTime = totalConnTime / time.Duration(connectedCount)
	}

	runenv.RecordMessage("Connection results: %d successful, %d failed", connectedCount, failedCount)

	// Wait for all connections
	client.MustSignalAndWait(ctx, tsync.State("connected"), runenv.TestInstanceCount)

	// Test peer discovery under conditions
	runenv.RecordMessage("Testing peer discovery...")

	rd := routing.NewRoutingDiscovery(kadDHT)
	_, err = rd.Advertise(ctx, "network-test")
	if err != nil {
		runenv.RecordMessage("Failed to advertise: %v", err)
	}

	discoveryStart := time.Now()
	discoveryCtx, discoveryCancel := context.WithTimeout(ctx, 60*time.Second)
	defer discoveryCancel()

	peerChan, err := rd.FindPeers(discoveryCtx, "network-test")
	if err != nil {
		return fmt.Errorf("failed to find peers: %w", err)
	}

	discoveredCount := 0
	for p := range peerChan {
		if p.ID != h.ID() {
			discoveredCount++
			runenv.RecordMessage("Discovered peer: %s", p.ID)
		}
	}

	discoveryDuration := time.Since(discoveryStart)

	// Test message latency
	runenv.RecordMessage("Testing message latency...")

	if connectedCount > 0 {
		// Simple ping-pong test
		testMessages := 10
		var testPeer peer.ID
		for pid := range peers {
			testPeer = pid
			break
		}

		if testPeer != "" {
			totalLatency := time.Duration(0)
			successCount := 0

			for i := 0; i < testMessages; i++ {
				startTime := time.Now()

				// Open and close a stream to measure RTT
				s, err := h.NewStream(ctx, testPeer, "/test/ping/1.0.0")
				if err == nil {
					s.Close()
					latency := time.Since(startTime)
					totalLatency += latency
					successCount++
					runenv.R().RecordPoint("message_latency_ms", float64(latency.Milliseconds()))
				}

				time.Sleep(100 * time.Millisecond)
			}

			if successCount > 0 {
				avgLatency := totalLatency / time.Duration(successCount)
				runenv.RecordMessage("Average message latency: %s", avgLatency)
				runenv.R().RecordPoint("avg_message_latency_ms", float64(avgLatency.Milliseconds()))
			}
		}
	}

	// Record final metrics
	runenv.R().RecordPoint("peers_connected", float64(connectedCount))
	runenv.R().RecordPoint("peers_discovered", float64(discoveredCount))
	runenv.R().RecordPoint("discovery_duration_ms", float64(discoveryDuration.Milliseconds()))

	connectionRate := float64(connectedCount) / float64(len(peers)) * 100.0
	runenv.R().RecordPoint("connection_success_rate", connectionRate)

	if connectedCount > 0 {
		runenv.R().RecordPoint("avg_connection_time_ms", float64(avgConnTime.Milliseconds()))
	}

	runenv.RecordMessage("Network Conditions Test Summary:")
	runenv.RecordMessage("  Connections: %d/%d (%.2f%%)", connectedCount, len(peers), connectionRate)
	runenv.RecordMessage("  Avg connection time: %s", avgConnTime)
	runenv.RecordMessage("  Peers discovered: %d", discoveredCount)
	runenv.RecordMessage("  Discovery duration: %s", discoveryDuration)

	// Final sync
	client.MustSignalAndWait(ctx, tsync.State("done"), runenv.TestInstanceCount)

	runenv.RecordSuccess()
	return nil
}
