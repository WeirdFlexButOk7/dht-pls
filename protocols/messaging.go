package protocols

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	dht "github.com/libp2p/go-libp2p-kad-dht"
)

const (
	// MessageProtocol is the protocol ID for peer-to-peer messaging
	MessageProtocol protocol.ID = "/dht-p2p/message/1.0.0"
)

// Message represents a P2P message
type Message struct {
	From      string    `json:"from"`
	To        string    `json:"to"`
	Type      string    `json:"type"`
	Payload   string    `json:"payload"`
	Timestamp time.Time `json:"timestamp"`
}

// MessageHandler handles incoming messages
type MessageHandler struct {
	host          host.Host
	dht  					*dht.IpfsDHT
	handleMessage func(*Message)
}

// NewMessageHandler creates a new message handler
func NewMessageHandler(h host.Host, d *dht.IpfsDHT, handleFunc func(*Message)) *MessageHandler {
	handler := &MessageHandler{
		host:          h,
		dht: d,
		handleMessage: handleFunc,
	}

	// Set stream handler for the protocol
	h.SetStreamHandler(MessageProtocol, handler.handleStream)

	return handler
}

// handleStream handles incoming streams for the message protocol
func (mh *MessageHandler) handleStream(s network.Stream) {
	defer s.Close()

	// Set read deadline to prevent hanging
	s.SetReadDeadline(time.Now().Add(30 * time.Second))

	// Read the message
	reader := bufio.NewReader(s)
	msgBytes, err := reader.ReadBytes('\n')
	if err != nil {
		if err != io.EOF {
			fmt.Printf("Error reading message: %v\n", err)
		}
		return
	}

	// Parse message
	var msg Message
	if err := json.Unmarshal(msgBytes, &msg); err != nil {
		fmt.Printf("Error unmarshaling message: %v\n", err)
		return
	}

	// Handle the message
	if mh.handleMessage != nil {
		mh.handleMessage(&msg)
	}

	// Send acknowledgment
	ack := map[string]string{"status": "received"}
	ackBytes, _ := json.Marshal(ack)
	s.Write(append(ackBytes, '\n'))
}

// SendMessage sends a message to a specific peer
func (mh *MessageHandler) SendMessage(ctx context.Context, peerID peer.ID, msg *Message) error {
	// Open a stream to the peer
	info, err := mh.dht.FindPeer(ctx, peerID)
	if err != nil {
		return fmt.Errorf("failed to find peer via DHT: %w", err)
	}

	if err := mh.host.Connect(ctx, info); err != nil {
		return fmt.Errorf("failed to connect to peer: %w", err)
	}
	// Open a stream to the peer
	s, err := mh.host.NewStream(ctx, peerID, MessageProtocol)
	// s, err := mh.host.NewStream(ctx, peerID, MessageProtocol)

	if err != nil {
		return fmt.Errorf("failed to open stream: %w", err)
	}
	defer s.Close()

	// Set write deadline
	s.SetWriteDeadline(time.Now().Add(10 * time.Second))

	// Serialize and send message
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	writer := bufio.NewWriter(s)
	if _, err := writer.Write(append(msgBytes, '\n')); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush message: %w", err)
	}

	// Read acknowledgment
	s.SetReadDeadline(time.Now().Add(10 * time.Second))
	reader := bufio.NewReader(s)
	ackBytes, err := reader.ReadBytes('\n')
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read ack: %w", err)
	}

	var ack map[string]string
	if err := json.Unmarshal(ackBytes, &ack); err == nil {
		if ack["status"] == "received" {
			return nil
		}
	}

	return nil
}

// BroadcastMessage sends a message to all connected peers
func (mh *MessageHandler) BroadcastMessage(ctx context.Context, msg *Message) {
	peers := mh.host.Network().Peers()

	for _, peerID := range peers {
		go func(pid peer.ID) {
			if err := mh.SendMessage(ctx, pid, msg); err != nil {
				fmt.Printf("Failed to send message to %s: %v\n", pid, err)
			}
		}(peerID)
	}
}
