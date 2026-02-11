package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/testground/sdk-go/runtime"
	tsync "github.com/testground/sdk-go/sync"
)

const ScalabilityProtocol = protocol.ID("/test/scalability/1.0.0")

func runScalability(runenv *runtime.RunEnv) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Get test parameters
	rampUpDuration := runenv.DurationParam("ramp_up_duration")
	steadyDuration := runenv.DurationParam("steady_duration")
	messagesPerNode := runenv.IntParam("messages_per_node")

	runenv.RecordMessage("Starting scalability test")
	runenv.RecordMessage("Instances: %d, Ramp-up: %s, Steady: %s, Messages/node: %d",
		runenv.TestInstanceCount, rampUpDuration, steadyDuration, messagesPerNode)

	seq := runenv.TestGroupInstanceCount

	// Staggered start for ramp-up
	startDelay := time.Duration(seq-1) * (rampUpDuration / time.Duration(runenv.TestInstanceCount))
	runenv.RecordMessage("Node %d will start in %s", seq, startDelay)
	time.Sleep(startDelay)

	startTime := time.Now()

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

	// Message tracking
	var (
		receivedCount int
		sentCount     int
		mu            sync.Mutex
	)

	// Set up message handler
	h.SetStreamHandler(ScalabilityProtocol, func(s network.Stream) {
		defer s.Close()
		mu.Lock()
		receivedCount++
		mu.Unlock()
	})

	runenv.RecordMessage("Node %d started: %s", seq, h.ID())

	initDuration := time.Since(startTime)
	runenv.R().RecordPoint("init_duration_ms", float64(initDuration.Milliseconds()))

	// Get sync client
	client := runenv.SyncClient()

	// Publish our address
	addrInfo := &peer.AddrInfo{
		ID:    h.ID(),
		Addrs: h.Addrs(),
	}

	ai := tsync.NewSubtree(tsync.State("addrs"))
	client.MustPublish(ctx, ai.Key(h.ID().String()), addrInfo)

	// Determine bootstrap nodes (first 10% of nodes)
	bootstrapCount := runenv.TestInstanceCount / 10
	if bootstrapCount < 2 {
		bootstrapCount = 2
	}
	isBootstrap := seq <= bootstrapCount

	if isBootstrap {
		bootstrapAI := tsync.NewSubtree(tsync.State("bootstrap-addrs"))
		client.MustPublish(ctx, bootstrapAI.Key(h.ID().String()), addrInfo)
		runenv.RecordMessage("Published as bootstrap node")
	}

	// Wait for enough nodes to be ready (not all, to test dynamic joining)
	minReady := runenv.TestInstanceCount / 2
	client.MustSignalEntry(ctx, tsync.State("ready"))

	runenv.RecordMessage("Waiting for at least %d nodes to be ready...", minReady)
	for {
		count := client.SignalCount(ctx, tsync.State("ready"))
		if count >= minReady {
			break
		}
		time.Sleep(time.Second)
	}

	// Connect to bootstrap nodes
	if !isBootstrap {
		runenv.RecordMessage("Connecting to bootstrap nodes...")

		bootstrapAI := tsync.NewSubtree(tsync.State("bootstrap-addrs"))
		bootstrapCh := make(chan *peer.AddrInfo, bootstrapCount)

		sctx, scancel := context.WithTimeout(ctx, 30*time.Second)
		client.Subscribe(sctx, bootstrapAI, bootstrapCh)
		scancel()

		connected := 0
		for i := 0; i < bootstrapCount; i++ {
			select {
			case bootstrapAddr := <-bootstrapCh:
				if err := h.Connect(ctx, *bootstrapAddr); err != nil {
					runenv.RecordMessage("Failed to connect to bootstrap %s: %v", bootstrapAddr.ID, err)
				} else {
					connected++
					runenv.RecordMessage("Connected to bootstrap: %s", bootstrapAddr.ID)
				}
			case <-time.After(5 * time.Second):
				break
			}
		}

		runenv.RecordMessage("Connected to %d/%d bootstrap nodes", connected, bootstrapCount)
	}

	// Wait for ramp-up to complete
	remainingRampUp := rampUpDuration - time.Since(startTime)
	if remainingRampUp > 0 {
		runenv.RecordMessage("Waiting for ramp-up to complete: %s", remainingRampUp)
		time.Sleep(remainingRampUp)
	}

	client.MustSignalEntry(ctx, tsync.State("ramp-up-complete"))

	// Record metrics during ramp-up
	connCount := len(h.Network().Peers())
	runenv.R().RecordPoint("connections_after_rampup", float64(connCount))
	runenv.R().RecordPoint("dht_size_after_rampup", float64(kadDHT.RoutingTable().Size()))

	runenv.RecordMessage("Ramp-up complete. Connections: %d, DHT size: %d",
		connCount, kadDHT.RoutingTable().Size())

	// Steady state: collect peer addresses and send messages
	runenv.RecordMessage("Entering steady state...")

	steadyStart := time.Now()

	// Get all available peer addresses
	peers := make([]peer.ID, 0)
	peerCh := make(chan *peer.AddrInfo, runenv.TestInstanceCount)

	sctx, scancel := context.WithTimeout(ctx, 30*time.Second)
	client.Subscribe(sctx, ai, peerCh)
	scancel()

	discoveryStart := time.Now()
	for i := 0; i < runenv.TestInstanceCount; i++ {
		select {
		case peerAddr := <-peerCh:
			if peerAddr.ID != h.ID() {
				// Try to connect
				if err := h.Connect(ctx, *peerAddr); err == nil {
					peers = append(peers, peerAddr.ID)
				}
			}
		case <-time.After(2 * time.Second):
			break
		}

		if len(peers) >= 20 { // Limit connections per node for scalability
			break
		}
	}
	discoveryDuration := time.Since(discoveryStart)

	runenv.RecordMessage("Discovered and connected to %d peers in %s", len(peers), discoveryDuration)
	runenv.R().RecordPoint("peer_discovery_duration_ms", float64(discoveryDuration.Milliseconds()))
	runenv.R().RecordPoint("discovered_peers", float64(len(peers)))

	// Send messages during steady state
	if len(peers) > 0 {
		runenv.RecordMessage("Sending %d messages...", messagesPerNode)

		messageInterval := steadyDuration / time.Duration(messagesPerNode)
		ticker := time.NewTicker(messageInterval)
		defer ticker.Stop()

		messageSent := 0
		messageFailed := 0

		for i := 0; i < messagesPerNode; i++ {
			// Select a random peer
			targetPeer := peers[i%len(peers)]

			sendStart := time.Now()
			s, err := h.NewStream(ctx, targetPeer, ScalabilityProtocol)
			sendDuration := time.Since(sendStart)

			if err != nil {
				messageFailed++
			} else {
				s.Close()
				messageSent++
				mu.Lock()
				sentCount++
				mu.Unlock()
				runenv.R().RecordPoint("message_send_duration_ms", float64(sendDuration.Milliseconds()))
			}

			if messageSent%100 == 0 && messageSent > 0 {
				runenv.RecordMessage("Sent %d messages", messageSent)
			}

			<-ticker.C
		}

		runenv.RecordMessage("Finished sending messages: %d sent, %d failed", messageSent, messageFailed)
	}

	steadyElapsed := time.Since(steadyStart)
	if steadyElapsed < steadyDuration {
		remainingSteady := steadyDuration - steadyElapsed
		runenv.RecordMessage("Waiting for steady state to complete: %s", remainingSteady)
		time.Sleep(remainingSteady)
	}

	// Wait for all messages to propagate
	time.Sleep(5 * time.Second)

	// Final metrics
	mu.Lock()
	finalReceived := receivedCount
	finalSent := sentCount
	mu.Unlock()

	finalConnections := len(h.Network().Peers())
	finalDHTSize := kadDHT.RoutingTable().Size()

	runenv.R().RecordPoint("final_connections", float64(finalConnections))
	runenv.R().RecordPoint("final_dht_size", float64(finalDHTSize))
	runenv.R().RecordPoint("messages_sent", float64(finalSent))
	runenv.R().RecordPoint("messages_received", float64(finalReceived))

	totalDuration := time.Since(startTime)
	runenv.R().RecordPoint("total_duration_ms", float64(totalDuration.Milliseconds()))

	runenv.RecordMessage("Scalability Test Summary:")
	runenv.RecordMessage("  Node: %d/%d", seq, runenv.TestInstanceCount)
	runenv.RecordMessage("  Total duration: %s", totalDuration)
	runenv.RecordMessage("  Final connections: %d", finalConnections)
	runenv.RecordMessage("  DHT routing table size: %d", finalDHTSize)
	runenv.RecordMessage("  Messages sent: %d", finalSent)
	runenv.RecordMessage("  Messages received: %d", finalReceived)

	// Final sync
	client.MustSignalAndWait(ctx, tsync.State("done"), runenv.TestInstanceCount)

	runenv.RecordSuccess()
	return nil
}
