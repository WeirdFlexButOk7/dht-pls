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
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/testground/sdk-go/runtime"
	tsync "github.com/testground/sdk-go/sync"
)

const MessageProtocolID = protocol.ID("/test/message/1.0.0")

type TestMessage struct {
	ID        int       `json:"id"`
	Payload   string    `json:"payload"`
	Timestamp time.Time `json:"timestamp"`
}

func runDirectMessaging(runenv *runtime.RunEnv) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Get test parameters
	messageCount := runenv.IntParam("message_count")
	messageSize := runenv.IntParam("message_size")
	concurrentSends := runenv.IntParam("concurrent_sends")

	runenv.RecordMessage("Starting direct messaging test")
	runenv.RecordMessage("Messages: %d, Size: %d bytes, Concurrent: %d",
		messageCount, messageSize, concurrentSends)

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
		receivedMsgs = make(map[int]bool)
		msgMutex     sync.Mutex
		receivedChan = make(chan *TestMessage, messageCount*10)
	)

	// Set up message handler
	h.SetStreamHandler(MessageProtocolID, func(s network.Stream) {
		defer s.Close()

		reader := bufio.NewReader(s)
		data, err := reader.ReadBytes('\n')
		if err != nil && err != io.EOF {
			runenv.RecordMessage("Error reading message: %v", err)
			return
		}

		var msg TestMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			runenv.RecordMessage("Error unmarshaling message: %v", err)
			return
		}

		msgMutex.Lock()
		if !receivedMsgs[msg.ID] {
			receivedMsgs[msg.ID] = true
			receivedChan <- &msg
		}
		msgMutex.Unlock()

		// Send ack
		ack := []byte("ACK\n")
		s.Write(ack)
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

	// Connect to all peers
	for _, peerAddr := range peers {
		if err := h.Connect(ctx, *peerAddr); err != nil {
			runenv.RecordMessage("Failed to connect to %s: %v", peerAddr.ID, err)
		} else {
			runenv.RecordMessage("Connected to peer: %s", peerAddr.ID)
		}
	}

	// Wait for all connections
	client.MustSignalAndWait(ctx, tsync.State("connected"), runenv.TestInstanceCount)

	// Create test payload
	payload := make([]byte, messageSize)
	for i := range payload {
		payload[i] = byte(i % 256)
	}

	// Send messages
	seq := runenv.TestGroupInstanceCount
	if seq == 1 {
		// First node is the sender
		runenv.RecordMessage("Starting to send %d messages", messageCount)

		sentCount := 0
		failCount := 0
		totalLatency := time.Duration(0)

		var wg sync.WaitGroup
		semaphore := make(chan struct{}, concurrentSends)

		startTime := time.Now()

		for i := 0; i < messageCount; i++ {
			wg.Add(1)
			semaphore <- struct{}{}

			go func(msgID int) {
				defer wg.Done()
				defer func() { <-semaphore }()

				// Pick a random peer
				var targetPeer peer.ID
				for pid := range peers {
					targetPeer = pid
					break
				}

				msg := TestMessage{
					ID:        msgID,
					Payload:   string(payload),
					Timestamp: time.Now(),
				}

				msgBytes, _ := json.Marshal(msg)
				msgBytes = append(msgBytes, '\n')

				sendStart := time.Now()

				s, err := h.NewStream(ctx, targetPeer, MessageProtocolID)
				if err != nil {
					runenv.RecordMessage("Failed to create stream to %s: %v", targetPeer, err)
					failCount++
					return
				}
				defer s.Close()

				// Send message
				if _, err := s.Write(msgBytes); err != nil {
					runenv.RecordMessage("Failed to send message %d: %v", msgID, err)
					failCount++
					return
				}

				// Read ack
				reader := bufio.NewReader(s)
				_, err = reader.ReadBytes('\n')
				if err != nil && err != io.EOF {
					runenv.RecordMessage("Failed to read ack for message %d: %v", msgID, err)
					failCount++
					return
				}

				latency := time.Since(sendStart)
				totalLatency += latency

				sentCount++
				if sentCount%10 == 0 {
					runenv.RecordMessage("Sent %d/%d messages", sentCount, messageCount)
				}
			}(i)
		}

		wg.Wait()
		totalDuration := time.Since(startTime)

		// Record metrics
		runenv.R().RecordPoint("messages_sent", float64(sentCount))
		runenv.R().RecordPoint("messages_failed", float64(failCount))
		runenv.R().RecordPoint("total_duration_ms", float64(totalDuration.Milliseconds()))
		if sentCount > 0 {
			avgLatency := totalLatency / time.Duration(sentCount)
			runenv.R().RecordPoint("avg_latency_ms", float64(avgLatency.Milliseconds()))
		}

		runenv.RecordMessage("Sent %d messages, %d failed, duration: %s",
			sentCount, failCount, totalDuration)
	} else {
		// Other nodes are receivers
		runenv.RecordMessage("Waiting to receive messages...")

		timeout := time.After(2 * time.Minute)
		receivedCount := 0

	receiveLoop:
		for {
			select {
			case msg := <-receivedChan:
				receivedCount++
				latency := time.Since(msg.Timestamp)
				runenv.R().RecordPoint("receive_latency_ms", float64(latency.Milliseconds()))

				if receivedCount%10 == 0 {
					runenv.RecordMessage("Received %d messages", receivedCount)
				}
			case <-timeout:
				break receiveLoop
			}
		}

		runenv.RecordMessage("Received %d messages", receivedCount)
		runenv.R().RecordPoint("messages_received", float64(receivedCount))
	}

	// Final sync
	client.MustSignalAndWait(ctx, tsync.State("done"), runenv.TestInstanceCount)

	runenv.RecordSuccess()
	return nil
}
