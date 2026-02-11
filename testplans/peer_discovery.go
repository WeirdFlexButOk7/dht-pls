package main

import (
	"context"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/sdk-go/sync"
)

func runPeerDiscovery(runenv *runtime.RunEnv) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Get test parameters
	bootstrapCount := runenv.IntParam("bootstrap_count")
	discoveryTimeout := runenv.DurationParam("discovery_timeout")
	targetPeers := runenv.IntParam("target_peers")

	runenv.RecordMessage("Starting peer discovery test")
	runenv.RecordMessage("Bootstrap nodes: %d, Target peers: %d, Timeout: %s",
		bootstrapCount, targetPeers, discoveryTimeout)

	// Determine if this instance is a bootstrap node
	seq := runenv.TestGroupInstanceCount
	isBootstrap := seq <= bootstrapCount

	// Create libp2p host
	h, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"),
	)
	if err != nil {
		return fmt.Errorf("failed to create libp2p host: %w", err)
	}
	defer h.Close()

	runenv.RecordMessage("Node ID: %s (Bootstrap: %v)", h.ID(), isBootstrap)

	// Initialize DHT
	var kadDHT *dht.IpfsDHT
	if isBootstrap {
		kadDHT, err = dht.New(ctx, h, dht.Mode(dht.ModeServer))
	} else {
		kadDHT, err = dht.New(ctx, h, dht.Mode(dht.ModeClient))
	}
	if err != nil {
		return fmt.Errorf("failed to create DHT: %w", err)
	}
	defer kadDHT.Close()

	// Bootstrap the DHT
	if err := kadDHT.Bootstrap(ctx); err != nil {
		return fmt.Errorf("failed to bootstrap DHT: %w", err)
	}

	// Get sync client for coordination
	client := runenv.SyncClient()

	// Publish our address
	addrInfo := &peer.AddrInfo{
		ID:    h.ID(),
		Addrs: h.Addrs(),
	}

	ai := sync.NewSubtree(sync.State("addrs"))
	if isBootstrap {
		// Bootstrap nodes publish their addresses
		client.MustPublish(ctx, ai.Key(h.ID().String()), addrInfo)
		runenv.RecordMessage("Published bootstrap address")
	}

	// Wait for all nodes to be ready
	client.MustSignalAndWait(ctx, sync.State("ready"), runenv.TestInstanceCount)

	// Non-bootstrap nodes connect to bootstrap nodes
	if !isBootstrap {
		runenv.RecordMessage("Retrieving bootstrap addresses")

		// Wait for bootstrap nodes to publish
		time.Sleep(2 * time.Second)

		bootstrapCh := make(chan *peer.AddrInfo, bootstrapCount)

		// Subscribe to bootstrap addresses
		for i := 1; i <= bootstrapCount; i++ {
			sctx, scancel := context.WithTimeout(ctx, 10*time.Second)
			err := client.Subscribe(sctx, ai, bootstrapCh)
			scancel()
			if err != nil {
				runenv.RecordMessage("Warning: failed to subscribe to bootstrap addresses: %v", err)
			}
		}

		// Connect to bootstrap nodes
		connectedBootstrap := 0
		timeout := time.After(30 * time.Second)
	bootstrapLoop:
		for {
			select {
			case bootstrapAddr := <-bootstrapCh:
				if bootstrapAddr.ID == h.ID() {
					continue
				}
				h.Peerstore().AddAddrs(bootstrapAddr.ID, bootstrapAddr.Addrs, peerstore.PermanentAddrTTL)
				if err := h.Connect(ctx, *bootstrapAddr); err != nil {
					runenv.RecordMessage("Failed to connect to bootstrap %s: %v", bootstrapAddr.ID, err)
				} else {
					connectedBootstrap++
					runenv.RecordMessage("Connected to bootstrap node: %s", bootstrapAddr.ID)
				}
				if connectedBootstrap >= bootstrapCount {
					break bootstrapLoop
				}
			case <-timeout:
				runenv.RecordMessage("Bootstrap connection timeout, connected to %d/%d", connectedBootstrap, bootstrapCount)
				break bootstrapLoop
			}
		}
	}

	// Signal bootstrap complete
	client.MustSignalAndWait(ctx, sync.State("bootstrap-complete"), runenv.TestInstanceCount)

	// Start peer discovery
	runenv.RecordMessage("Starting peer discovery via DHT")
	rd := routing.NewRoutingDiscovery(kadDHT)

	// Advertise ourselves
	_, err = rd.Advertise(ctx, "p2p-network-test")
	if err != nil {
		runenv.RecordMessage("Failed to advertise: %v", err)
	}

	// Discover peers
	discoveredPeers := make(map[peer.ID]bool)
	startTime := time.Now()

	discoveryCtx, discoveryCancel := context.WithTimeout(ctx, discoveryTimeout)
	defer discoveryCancel()

	peerChan, err := rd.FindPeers(discoveryCtx, "p2p-network-test")
	if err != nil {
		return fmt.Errorf("failed to find peers: %w", err)
	}

	for p := range peerChan {
		if p.ID == h.ID() {
			continue
		}

		if !discoveredPeers[p.ID] {
			discoveredPeers[p.ID] = true

			// Try to connect
			h.Peerstore().AddAddrs(p.ID, p.Addrs, peerstore.TempAddrTTL)
			if err := h.Connect(ctx, p); err != nil {
				runenv.RecordMessage("Discovered but failed to connect to peer %s: %v", p.ID, err)
			} else {
				runenv.RecordMessage("Discovered and connected to peer: %s", p.ID)
			}
		}

		if len(discoveredPeers) >= targetPeers {
			break
		}
	}

	discoveryDuration := time.Since(startTime)

	// Record metrics
	runenv.R().RecordPoint("peers_discovered", float64(len(discoveredPeers)))
	runenv.R().RecordPoint("discovery_duration_ms", float64(discoveryDuration.Milliseconds()))
	runenv.R().RecordPoint("connected_peers", float64(len(h.Network().Peers())))

	runenv.RecordMessage("Discovery complete: found %d peers in %s", len(discoveredPeers), discoveryDuration)

	// Final sync
	client.MustSignalAndWait(ctx, sync.State("done"), runenv.TestInstanceCount)

	// Verify success
	if len(discoveredPeers) >= targetPeers {
		runenv.RecordSuccess()
	} else {
		runenv.RecordFailure(fmt.Errorf("only discovered %d peers, target was %d", len(discoveredPeers), targetPeers))
	}

	return nil
}
