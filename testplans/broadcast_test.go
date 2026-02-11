package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/testground/sdk-go/runtime"
	tsync "github.com/testground/sdk-go/sync"
)

const BroadcastProtocolID = protocol.ID("/test/broadcast/1.0.0")

type BroadcastMessage struct {
	ID        string    `json:"id"`
	From      string    `json:"from"`
	Sequence  int       `json:"sequence"`
	Payload   string    `json:"payload"`
	Timestamp time.Time `json:"timestamp"`
}

func runBroadcastTest(runenv *runtime.RunEnv) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Get test parameters
	broadcastCount := runenv.IntParam("broadcast_count")
	broadcastInterval := runenv.DurationParam("broadcast_interval")

	runenv.RecordMessage("Starting broadcast test")
	runenv.RecordMessage("Broadcasts per node: %d, Interval: %s", broadcastCount, broadcastInterval)

	// Create libp2p host
	h, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"),
	)
	if err != nil {
		return fmt.Errorf("failed to create host: %w", err)
	}
	defer h.Close()

	// Message tracking
	var (
		receivedBroadcasts = make(map[string]bool)
		broadcastMutex     sync.Mutex
		receivedCount      int
	)

	// Set up broadcast handler
	h.SetStreamHandler(BroadcastProtocolID, func(s network.Stream) {
		defer s.Close()

		reader := bufio.NewReader(s)
		data, err := reader.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return
		}

		var msg BroadcastMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return
		}

		broadcastMutex.Lock()
		msgKey := fmt.Sprintf("%s-%d", msg.From, msg.Sequence)
		if !receivedBroadcasts[msgKey] {
			receivedBroadcasts[msgKey] = true
			receivedCount++

			latency := time.Since(msg.Timestamp)
			runenv.R().RecordPoint("broadcast_latency_ms", float64(latency.Milliseconds()))
		}
		broadcastMutex.Unlock()
	})

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

	// Get all peer addresses
	peers := make([]peer.ID, 0)
	peerCh := make(chan *peer.AddrInfo, runenv.TestInstanceCount)

	sctx, scancel := context.WithTimeout(ctx, 30*time.Second)
	client.Subscribe(sctx, ai, peerCh)
	scancel()

	for i := 0; i < runenv.TestInstanceCount; i++ {
		select {
		case peerAddr := <-peerCh:
			if peerAddr.ID != h.ID() {
				if err := h.Connect(ctx, *peerAddr); err != nil {
					runenv.RecordMessage("Failed to connect to %s: %v", peerAddr.ID, err)
				} else {
					peers = append(peers, peerAddr.ID)
				}
			}
		case <-time.After(5 * time.Second):
			break
		}
	}

	runenv.RecordMessage("Connected to %d peers", len(peers))

	// Wait for all connections
	client.MustSignalAndWait(ctx, tsync.State("connected"), runenv.TestInstanceCount)

	// Start broadcasting
	runenv.RecordMessage("Starting broadcasts...")

	sentCount := 0
	failCount := 0

	for i := 0; i < broadcastCount; i++ {
		msg := BroadcastMessage{
			ID:        fmt.Sprintf("%s-%d", h.ID(), i),
			From:      h.ID().String(),
			Sequence:  i,
			Payload:   fmt.Sprintf("Broadcast message %d from %s", i, h.ID()),
			Timestamp: time.Now(),
		}

		msgBytes, _ := json.Marshal(msg)
		msgBytes = append(msgBytes, '\n')

		// Broadcast to all peers
		var wg sync.WaitGroup
		for _, peerID := range peers {
			wg.Add(1)
			go func(pid peer.ID) {
				defer wg.Done()

				s, err := h.NewStream(ctx, pid, BroadcastProtocolID)
				if err != nil {
					broadcastMutex.Lock()
					failCount++
					broadcastMutex.Unlock()
					return
				}
				defer s.Close()

				if _, err := s.Write(msgBytes); err != nil {
					broadcastMutex.Lock()
					failCount++
					broadcastMutex.Unlock()
				}
			}(peerID)
		}

		wg.Wait()
		sentCount++

		runenv.RecordMessage("Sent broadcast %d/%d to %d peers", i+1, broadcastCount, len(peers))

		if i < broadcastCount-1 {
			time.Sleep(broadcastInterval)
		}
	}

	// Wait for all broadcasts to propagate
	runenv.RecordMessage("Waiting for broadcasts to propagate...")
	time.Sleep(10 * time.Second)

	// Record metrics
	runenv.R().RecordPoint("broadcasts_sent", float64(sentCount))
	runenv.R().RecordPoint("broadcast_failures", float64(failCount))
	runenv.R().RecordPoint("broadcasts_received", float64(receivedCount))

	expectedReceived := (runenv.TestInstanceCount - 1) * broadcastCount
	receiveRate := float64(receivedCount) / float64(expectedReceived) * 100.0
	runenv.R().RecordPoint("receive_rate_percent", receiveRate)

	runenv.RecordMessage("Sent %d broadcasts, received %d (expected %d, %.2f%%)",
		sentCount, receivedCount, expectedReceived, receiveRate)

	// Final sync
	client.MustSignalAndWait(ctx, tsync.State("done"), runenv.TestInstanceCount)

	runenv.RecordSuccess()
	return nil
}
