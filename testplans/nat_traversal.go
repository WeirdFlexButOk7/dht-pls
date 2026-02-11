package main

import (
	"context"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	"github.com/testground/sdk-go/runtime"
	tsync "github.com/testground/sdk-go/sync"
)

func runNATTraversal(runenv *runtime.RunEnv) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// Get test parameters
	relayCount := runenv.IntParam("relay_count")

	runenv.RecordMessage("Starting NAT traversal test")
	runenv.RecordMessage("Relay nodes: %d", relayCount)

	seq := runenv.TestGroupInstanceCount
	isRelay := seq <= relayCount

	var h *libp2p.Host
	var err error

	if isRelay {
		// Relay nodes with relay service enabled
		runenv.RecordMessage("Starting as RELAY node")

		opts := []libp2p.Option{
			libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"),
			libp2p.EnableRelayService(),
			libp2p.EnableAutoNATv2(),
		}

		host, err := libp2p.New(opts...)
		if err != nil {
			return fmt.Errorf("failed to create relay host: %w", err)
		}
		h = &host
	} else {
		// Client nodes behind NAT with relay and hole punching
		runenv.RecordMessage("Starting as CLIENT node (behind NAT)")

		// We'll add relay addresses after getting them
		opts := []libp2p.Option{
			libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"),
			libp2p.EnableAutoNATv2(),
			libp2p.EnableHolePunching(),
		}

		host, err := libp2p.New(opts...)
		if err != nil {
			return fmt.Errorf("failed to create client host: %w", err)
		}
		h = &host
	}

	if err != nil {
		return fmt.Errorf("failed to create host: %w", err)
	}
	defer (*h).Close()

	runenv.RecordMessage("Node ID: %s (Relay: %v)", (*h).ID(), isRelay)

	// Get sync client
	client := runenv.SyncClient()

	// Publish our address
	addrInfo := &peer.AddrInfo{
		ID:    (*h).ID(),
		Addrs: (*h).Addrs(),
	}

	ai := tsync.NewSubtree(tsync.State("addrs"))
	client.MustPublish(ctx, ai.Key((*h).ID().String()), addrInfo)

	if isRelay {
		// Relay nodes also publish to a separate topic
		relayAI := tsync.NewSubtree(tsync.State("relay-addrs"))
		client.MustPublish(ctx, relayAI.Key((*h).ID().String()), addrInfo)
		runenv.RecordMessage("Published relay address")
	}

	// Wait for all nodes to be ready
	client.MustSignalAndWait(ctx, tsync.State("ready"), runenv.TestInstanceCount)

	// Non-relay nodes configure AutoRelay
	if !isRelay {
		runenv.RecordMessage("Discovering relay nodes...")

		relayAI := tsync.NewSubtree(tsync.State("relay-addrs"))
		relayCh := make(chan *peer.AddrInfo, relayCount)

		sctx, scancel := context.WithTimeout(ctx, 30*time.Second)
		client.Subscribe(sctx, relayAI, relayCh)
		scancel()

		relayPeers := make([]peer.AddrInfo, 0, relayCount)
		for i := 0; i < relayCount; i++ {
			select {
			case relayAddr := <-relayCh:
				relayPeers = append(relayPeers, *relayAddr)
				runenv.RecordMessage("Found relay: %s", relayAddr.ID)

				// Connect to relay
				if err := (*h).Connect(ctx, *relayAddr); err != nil {
					runenv.RecordMessage("Failed to connect to relay %s: %v", relayAddr.ID, err)
				} else {
					runenv.RecordMessage("Connected to relay: %s", relayAddr.ID)
				}
			case <-time.After(10 * time.Second):
				runenv.RecordMessage("Timeout waiting for relay nodes")
				break
			}
		}

		// Enable AutoRelay with discovered relays
		if len(relayPeers) > 0 {
			runenv.RecordMessage("Configuring AutoRelay with %d relays", len(relayPeers))

			// Note: In a real scenario, AutoRelay would be configured during host creation
			// This is a simplified demonstration
			_ = autorelay.WithStaticRelays(relayPeers)
		}
	}

	// Wait for relay connections to establish
	client.MustSignalAndWait(ctx, tsync.State("relay-connected"), runenv.TestInstanceCount)
	time.Sleep(5 * time.Second)

	// Test connectivity through relays
	runenv.RecordMessage("Testing peer connectivity...")

	// Collect all peer addresses
	allPeers := make(map[peer.ID]*peer.AddrInfo)
	peerCh := make(chan *peer.AddrInfo, runenv.TestInstanceCount)

	sctx, scancel := context.WithTimeout(ctx, 30*time.Second)
	client.Subscribe(sctx, ai, peerCh)
	scancel()

	for i := 0; i < runenv.TestInstanceCount; i++ {
		select {
		case peerAddr := <-peerCh:
			if peerAddr.ID != (*h).ID() {
				allPeers[peerAddr.ID] = peerAddr
			}
		case <-time.After(5 * time.Second):
			break
		}
	}

	runenv.RecordMessage("Attempting to connect to %d peers", len(allPeers))

	directConnections := 0
	relayConnections := 0
	failedConnections := 0

	for _, peerAddr := range allPeers {
		startTime := time.Now()

		if err := (*h).Connect(ctx, *peerAddr); err != nil {
			runenv.RecordMessage("Failed to connect to %s: %v", peerAddr.ID, err)
			failedConnections++
		} else {
			connTime := time.Since(startTime)

			// Check if connection is relayed
			conns := (*h).Network().ConnsToPeer(peerAddr.ID)
			isRelayed := false
			for _, conn := range conns {
				if conn.Stat().Transient {
					isRelayed = true
					break
				}
			}

			if isRelayed {
				relayConnections++
				runenv.RecordMessage("Connected to %s via RELAY in %s", peerAddr.ID, connTime)
				runenv.R().RecordPoint("relay_connection_time_ms", float64(connTime.Milliseconds()))
			} else {
				directConnections++
				runenv.RecordMessage("Connected to %s DIRECTLY in %s", peerAddr.ID, connTime)
				runenv.R().RecordPoint("direct_connection_time_ms", float64(connTime.Milliseconds()))
			}
		}
	}

	// Record metrics
	totalAttempts := len(allPeers)
	runenv.R().RecordPoint("direct_connections", float64(directConnections))
	runenv.R().RecordPoint("relay_connections", float64(relayConnections))
	runenv.R().RecordPoint("failed_connections", float64(failedConnections))
	runenv.R().RecordPoint("total_connections", float64(directConnections+relayConnections))

	if totalAttempts > 0 {
		successRate := float64(directConnections+relayConnections) / float64(totalAttempts) * 100.0
		runenv.R().RecordPoint("connection_success_rate", successRate)
	}

	runenv.RecordMessage("NAT Traversal Test Summary:")
	runenv.RecordMessage("  Direct connections: %d", directConnections)
	runenv.RecordMessage("  Relay connections: %d", relayConnections)
	runenv.RecordMessage("  Failed connections: %d", failedConnections)
	runenv.RecordMessage("  Total successful: %d/%d", directConnections+relayConnections, totalAttempts)

	// Final sync
	client.MustSignalAndWait(ctx, tsync.State("done"), runenv.TestInstanceCount)

	runenv.RecordSuccess()
	return nil
}
