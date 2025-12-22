package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"dht-p2p/config"
	"dht-p2p/node"
	"dht-p2p/protocols"

	"github.com/libp2p/go-libp2p/core/peer"
	// "github.com/libp2p/go-libp2p/core/routing"
)

func main() {
	// Parse command-line flags
	port := flag.Int("port", 0, "Port to listen on (0 for random)")
	bootstrap := flag.String("bootstrap", "", "Comma-separated list of bootstrap peers")
	keyPath := flag.String("key", "", "Path to private key file")
	dhtMode := flag.String("dht-mode", "auto", "DHT mode: server, client, or auto")
	privateNet := flag.String("private", "", "Private network key (leave empty for public network)")
	flag.Parse()

	// Create configuration
	cfg := config.DefaultConfig()

	// Override with command-line flags
	if *port != 0 {
		cfg.ListenAddresses = []string{
			fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", *port),
			fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1", *port),
		}
	}

	if *bootstrap != "" {
		cfg.BootstrapPeers = strings.Split(*bootstrap, ",")
	}

	if *keyPath != "" {
		cfg.PrivateKeyPath = *keyPath
	}

	cfg.DHTMode = *dhtMode
	cfg.ProtectKey = *privateNet

	// Create context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create and start the node
	fmt.Println("🚀 Starting P2P node...")
	n, err := node.NewNode(ctx, cfg)
	if err != nil {
		fmt.Printf("❌ Failed to create node: %v\n", err)
		os.Exit(1)
	}
	defer n.Close()

	// Initialize DHT
	fmt.Println("🔍 Initializing DHT for peer discovery...")
	if err := n.InitDHT(ctx); err != nil {
		fmt.Printf("❌ Failed to initialize DHT: %v\n", err)
		os.Exit(1)
	}

	// Connect to bootstrap peers
	fmt.Println("🔗 Connecting to bootstrap peers...")
	if err := n.Connect(ctx); err != nil {
		fmt.Printf("⚠️  Warning: Failed to connect to some bootstrap peers: %v\n", err)
	}

	// routingDiscovery := routing.NewRoutingDiscovery(dht)
	// routingDiscovery.Advertise(ctx, "my-app")
	
	// Print node information
	fmt.Println("\n✅ Node is running!")
	fmt.Printf("📍 Peer ID: %s\n", n.Host.ID())
	fmt.Println("📡 Listening on:")
	for _, addr := range n.GetAddresses() {
		fmt.Printf("   %s\n", addr)
	}

	// Setup message handler
	msgHandler := protocols.NewMessageHandler(n.Host, n.DHT, func(msg *protocols.Message) {
		fmt.Printf("\n📨 Message from %s: %s\n", msg.From, msg.Payload)
		fmt.Print("> ")
	})

	// Print connected peers periodically
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				peers := n.Host.Network().Peers()
				fmt.Printf("\n👥 Connected peers: %d\n", len(peers))
				if len(peers) > 0 {
					fmt.Println("Peers:")
					for i, p := range peers {
						if i < 5 { // Show first 5 peers
							fmt.Printf("   - %s\n", p)
						}
					}
					if len(peers) > 5 {
						fmt.Printf("   ... and %d more\n", len(peers)-5)
					}
				}
				fmt.Print("> ")
			}
		}
	}()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start interactive CLI
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		fmt.Println("\n💬 Commands:")
		fmt.Println("   send <peer-id> <message>  - Send a message to a specific peer")
		fmt.Println("   broadcast <message>       - Broadcast a message to all peers")
		fmt.Println("   peers                     - List connected peers")
		fmt.Println("   info                      - Show node information")
		fmt.Println("   quit                      - Exit the application")
		fmt.Print("> ")

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				fmt.Print("> ")
				continue
			}

			parts := strings.Fields(line)
			cmd := parts[0]

			switch cmd {
			case "send":
				if len(parts) < 3 {
					fmt.Println("Usage: send <peer-id> <message>")
				} else {
					peerID, err := peer.Decode(parts[1])
					if err != nil {
						fmt.Printf("Invalid peer ID: %v\n", err)
					} else {
						msg := &protocols.Message{
							From:      n.Host.ID().String(),
							To:        peerID.String(),
							Type:      "text",
							Payload:   strings.Join(parts[2:], " "),
							Timestamp: time.Now(),
						}
						if err := msgHandler.SendMessage(ctx, peerID, msg); err != nil {
							fmt.Printf("Failed to send message: %v\n", err)
						} else {
							fmt.Println("✅ Message sent")
						}
					}
				}

			case "broadcast":
				if len(parts) < 2 {
					fmt.Println("Usage: broadcast <message>")
				} else {
					msg := &protocols.Message{
						From:      n.Host.ID().String(),
						Type:      "broadcast",
						Payload:   strings.Join(parts[1:], " "),
						Timestamp: time.Now(),
					}
					msgHandler.BroadcastMessage(ctx, msg)
					fmt.Println("✅ Message broadcasted")
				}

			case "peers":
				peers := n.Host.Network().Peers()
				fmt.Printf("Connected peers: %d\n", len(peers))
				for _, p := range peers {
					fmt.Printf("   - %s\n", p)
				}

			case "info":
				fmt.Printf("Peer ID: %s\n", n.Host.ID())
				fmt.Println("Addresses:")
				for _, addr := range n.GetAddresses() {
					fmt.Printf("   %s\n", addr)
				}
				fmt.Printf("DHT Mode: %s\n", cfg.DHTMode)
				fmt.Printf("Connected Peers: %d\n", len(n.Host.Network().Peers()))

			case "quit", "exit":
				fmt.Println("Shutting down...")
				cancel()
				return

			default:
				fmt.Printf("Unknown command: %s\n", cmd)
			}

			fmt.Print("> ")
		}
	}()

	// Wait for shutdown signal
	<-sigChan
	fmt.Println("\n🛑 Received shutdown signal, closing node...")
	cancel()

	// Give some time for graceful shutdown
	time.Sleep(time.Second)
	fmt.Println("👋 Goodbye!")
}
