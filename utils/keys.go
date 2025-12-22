package utils

import (
	"crypto/rand"
	"os"

	"github.com/libp2p/go-libp2p/core/crypto"
)

// LoadOrGenerateKey loads a private key from file or generates a new one
func LoadOrGenerateKey(keyPath string) (crypto.PrivKey, error) {
	if keyPath != "" {
		// Try to load existing key
		keyData, err := os.ReadFile(keyPath)
		if err == nil {
			return crypto.UnmarshalPrivateKey(keyData)
		}
	}

	// Generate new key
	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.Ed25519, 2048, rand.Reader)
	if err != nil {
		return nil, err
	}

	// Save if path provided
	if keyPath != "" {
		keyData, err := crypto.MarshalPrivateKey(priv)
		if err == nil {
			os.WriteFile(keyPath, keyData, 0600)
		}
	}

	return priv, nil
}
