package utils

import (
    "context"
    "fmt"

    "github.com/libp2p/go-libp2p/core/host"
    "github.com/libp2p/go-libp2p/core/peer"
    ma "github.com/multiformats/go-multiaddr"
)

func ConnectViaBootstrapRelay(
    ctx context.Context,
    h host.Host,
    bootstrapAddr string, // MUST be full addr with real peer ID
    target peer.ID,
) error {

    // Parse bootstrap multiaddr
    bma, err := ma.NewMultiaddr(bootstrapAddr)
    if err != nil {
        return fmt.Errorf("invalid bootstrap addr: %w", err)
    }

    // Create components SAFELY
    circuit, err := ma.NewMultiaddr("/p2p-circuit")
    if err != nil {
        return err
    }

    targetComp, err := ma.NewMultiaddr("/p2p/" + target.String())
    if err != nil {
        return fmt.Errorf("invalid target peer ID: %w", err)
    }

    relayAddr := bma.
        Encapsulate(circuit).
        Encapsulate(targetComp)

    ai := peer.AddrInfo{
        ID:    target,
        Addrs: []ma.Multiaddr{relayAddr},
    }

    return h.Connect(ctx, ai)
}
