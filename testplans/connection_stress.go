package main

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/testground/sdk-go/runtime"
	tsync "github.com/testground/sdk-go/sync"
)

func runConnectionStress(runenv *runtime.RunEnv) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// Get test parameters
	churnPercent := runenv.IntParam("connection_churn")
	churnInterval := runenv.DurationParam("churn_interval")
	testDuration := runenv.DurationParam("test_duration")

	runenv.RecordMessage("Starting connection stress test")
	runenv.RecordMessage("Churn: %d%%, Interval: %s, Duration: %s",
		churnPercent, churnInterval, testDuration)

	// Create libp2p host
	h, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"),
	)
	if err != nil {
		return fmt.Errorf("failed to create host: %w", err)
	}
	defer h.Close()

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

	// Collect all peer addresses
	allPeers := make(map[peer.ID]*peer.AddrInfo)
	peerCh := make(chan *peer.AddrInfo, runenv.TestInstanceCount)

	sctx, scancel := context.WithTimeout(ctx, 30*time.Second)
	client.Subscribe(sctx, ai, peerCh)
	scancel()

	for i := 0; i < runenv.TestInstanceCount; i++ {
		select {
		case peerAddr := <-peerCh:
			if peerAddr.ID != h.ID() {
				allPeers[peerAddr.ID] = peerAddr
			}
		case <-time.After(5 * time.Second):
			break
		}
	}

	runenv.RecordMessage("Collected %d peer addresses", len(allPeers))

	// Initial connection to all peers
	runenv.RecordMessage("Connecting to all peers...")
	for _, peerAddr := range allPeers {
		if err := h.Connect(ctx, *peerAddr); err != nil {
			runenv.RecordMessage("Failed initial connection to %s: %v", peerAddr.ID, err)
		}
	}

	initialConns := len(h.Network().Peers())
	runenv.RecordMessage("Initial connections: %d", initialConns)

	// Wait for all initial connections
	client.MustSignalAndWait(ctx, tsync.State("connected"), runenv.TestInstanceCount)

	// Start stress test
	runenv.RecordMessage("Starting connection churn...")

	startTime := time.Now()
	ticker := time.NewTicker(churnInterval)
	defer ticker.Stop()

	totalDisconnects := 0
	totalReconnects := 0
	failedReconnects := 0

	rnd := rand.New(rand.NewSource(time.Now().UnixNano() + int64(runenv.TestGroupInstanceCount)))

stressLoop:
	for {
		select {
		case <-ticker.C:
			if time.Since(startTime) > testDuration {
				break stressLoop
			}

			currentPeers := h.Network().Peers()
			if len(currentPeers) == 0 {
				runenv.RecordMessage("No peers connected, skipping churn")
				continue
			}

			// Calculate how many connections to churn
			churnCount := (len(currentPeers) * churnPercent) / 100
			if churnCount < 1 {
				churnCount = 1
			}

			runenv.RecordMessage("Churning %d connections...", churnCount)

			// Disconnect random peers
			disconnectedPeers := make([]peer.ID, 0, churnCount)
			for i := 0; i < churnCount && i < len(currentPeers); i++ {
				targetPeer := currentPeers[rnd.Intn(len(currentPeers))]

				if err := h.Network().ClosePeer(targetPeer); err != nil {
					runenv.RecordMessage("Failed to disconnect from %s: %v", targetPeer, err)
				} else {
					disconnectedPeers = append(disconnectedPeers, targetPeer)
					totalDisconnects++
				}
			}

			// Record current connection count
			currentConnCount := len(h.Network().Peers())
			runenv.R().RecordPoint("active_connections", float64(currentConnCount))

			// Wait a bit before reconnecting
			time.Sleep(time.Second)

			// Reconnect to disconnected peers
			for _, peerID := range disconnectedPeers {
				if peerAddr, ok := allPeers[peerID]; ok {
					if err := h.Connect(ctx, *peerAddr); err != nil {
						runenv.RecordMessage("Failed to reconnect to %s: %v", peerID, err)
						failedReconnects++
					} else {
						totalReconnects++
					}
				}
			}

			afterReconnect := len(h.Network().Peers())
			runenv.RecordMessage("Connections after churn: %d", afterReconnect)

		case <-ctx.Done():
			break stressLoop
		}
	}

	// Record final metrics
	finalConns := len(h.Network().Peers())

	runenv.R().RecordPoint("initial_connections", float64(initialConns))
	runenv.R().RecordPoint("final_connections", float64(finalConns))
	runenv.R().RecordPoint("total_disconnects", float64(totalDisconnects))
	runenv.R().RecordPoint("total_reconnects", float64(totalReconnects))
	runenv.R().RecordPoint("failed_reconnects", float64(failedReconnects))

	reconnectRate := 0.0
	if totalDisconnects > 0 {
		reconnectRate = float64(totalReconnects) / float64(totalDisconnects) * 100.0
	}
	runenv.R().RecordPoint("reconnect_success_rate", reconnectRate)

	runenv.RecordMessage("Connection Stress Test Summary:")
	runenv.RecordMessage("  Initial connections: %d", initialConns)
	runenv.RecordMessage("  Final connections: %d", finalConns)
	runenv.RecordMessage("  Total disconnects: %d", totalDisconnects)
	runenv.RecordMessage("  Total reconnects: %d (%.2f%%)", totalReconnects, reconnectRate)
	runenv.RecordMessage("  Failed reconnects: %d", failedReconnects)

	// Final sync
	client.MustSignalAndWait(ctx, tsync.State("done"), runenv.TestInstanceCount)

	runenv.RecordSuccess()
	return nil
}
