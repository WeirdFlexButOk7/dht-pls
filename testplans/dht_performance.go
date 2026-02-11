package main

import (
	"context"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/testground/sdk-go/runtime"
	tsync "github.com/testground/sdk-go/sync"
)

func runDHTPerformance(runenv *runtime.RunEnv) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Get test parameters
	lookupCount := runenv.IntParam("lookup_count")
	storeCount := runenv.IntParam("store_count")

	runenv.RecordMessage("Starting DHT performance test")
	runenv.RecordMessage("Lookups: %d, Stores: %d", lookupCount, storeCount)

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

	// Wait for all nodes to publish
	client.MustSignalAndWait(ctx, tsync.State("ready"), runenv.TestInstanceCount)

	// Connect to some peers
	peerCh := make(chan *peer.AddrInfo, runenv.TestInstanceCount)
	sctx, scancel := context.WithTimeout(ctx, 30*time.Second)
	client.Subscribe(sctx, ai, peerCh)
	scancel()

	connectedCount := 0
	for i := 0; i < runenv.TestInstanceCount && i < 10; i++ {
		select {
		case peerAddr := <-peerCh:
			if peerAddr.ID != h.ID() {
				if err := h.Connect(ctx, *peerAddr); err == nil {
					connectedCount++
				}
			}
		case <-time.After(2 * time.Second):
			break
		}
	}

	runenv.RecordMessage("Connected to %d peers", connectedCount)

	// Wait for DHT to stabilize
	client.MustSignalAndWait(ctx, tsync.State("dht-ready"), runenv.TestInstanceCount)
	time.Sleep(10 * time.Second)

	// Test DHT operations
	seq := runenv.TestGroupInstanceCount

	// Store values in DHT
	if seq <= storeCount {
		runenv.RecordMessage("Storing values in DHT...")

		for i := 0; i < storeCount; i++ {
			key := fmt.Sprintf("/test/key-%d-%d", seq, i)
			value := []byte(fmt.Sprintf("value-%d-%d", seq, i))

			startTime := time.Now()
			err := kadDHT.PutValue(ctx, key, value)
			duration := time.Since(startTime)

			if err != nil {
				runenv.RecordMessage("Failed to store key %s: %v", key, err)
				runenv.R().RecordPoint("store_failures", 1.0)
			} else {
				runenv.RecordMessage("Stored key %s in %s", key, duration)
				runenv.R().RecordPoint("store_duration_ms", float64(duration.Milliseconds()))
			}
		}
	}

	// Wait for all stores to complete
	client.MustSignalAndWait(ctx, tsync.State("store-complete"), runenv.TestInstanceCount)
	time.Sleep(5 * time.Second)

	// Perform peer lookups
	runenv.RecordMessage("Performing peer lookups...")

	successfulLookups := 0
	totalLookupTime := time.Duration(0)

	for i := 0; i < lookupCount; i++ {
		// Generate a random peer ID to look up
		targetSeq := (seq + i) % runenv.TestInstanceCount
		if targetSeq == 0 {
			targetSeq = runenv.TestInstanceCount
		}

		// Get the actual peer ID from sync
		lookupKey := fmt.Sprintf("peer-%d", targetSeq)

		startTime := time.Now()

		// Find peer via DHT routing table
		closestPeers := kadDHT.RoutingTable().NearestPeers([]byte(lookupKey), 5)

		duration := time.Since(startTime)
		totalLookupTime += duration

		if len(closestPeers) > 0 {
			successfulLookups++
			runenv.R().RecordPoint("lookup_duration_ms", float64(duration.Milliseconds()))
			runenv.RecordMessage("Lookup %d: found %d peers in %s", i+1, len(closestPeers), duration)
		} else {
			runenv.RecordMessage("Lookup %d: no peers found", i+1)
		}
	}

	// Test value retrieval
	if seq > storeCount {
		runenv.RecordMessage("Retrieving values from DHT...")

		retrieveCount := storeCount
		successfulRetrieves := 0

		for i := 0; i < retrieveCount; i++ {
			targetNode := (i % storeCount) + 1
			key := fmt.Sprintf("/test/key-%d-0", targetNode)

			startTime := time.Now()
			value, err := kadDHT.GetValue(ctx, key)
			duration := time.Since(startTime)

			if err != nil {
				runenv.RecordMessage("Failed to retrieve key %s: %v", key, err)
			} else {
				successfulRetrieves++
				runenv.RecordMessage("Retrieved key %s (%d bytes) in %s", key, len(value), duration)
				runenv.R().RecordPoint("retrieve_duration_ms", float64(duration.Milliseconds()))
			}
		}

		retrieveRate := float64(successfulRetrieves) / float64(retrieveCount) * 100.0
		runenv.R().RecordPoint("retrieve_success_rate", retrieveRate)
	}

	// Record final metrics
	runenv.R().RecordPoint("successful_lookups", float64(successfulLookups))
	runenv.R().RecordPoint("lookup_success_rate", float64(successfulLookups)/float64(lookupCount)*100.0)

	if successfulLookups > 0 {
		avgLookupTime := totalLookupTime / time.Duration(successfulLookups)
		runenv.R().RecordPoint("avg_lookup_duration_ms", float64(avgLookupTime.Milliseconds()))
	}

	runenv.R().RecordPoint("routing_table_size", float64(kadDHT.RoutingTable().Size()))

	runenv.RecordMessage("DHT Performance Summary:")
	runenv.RecordMessage("  Lookups: %d/%d successful", successfulLookups, lookupCount)
	runenv.RecordMessage("  Routing table size: %d", kadDHT.RoutingTable().Size())

	// Final sync
	client.MustSignalAndWait(ctx, tsync.State("done"), runenv.TestInstanceCount)

	runenv.RecordSuccess()
	return nil
}
