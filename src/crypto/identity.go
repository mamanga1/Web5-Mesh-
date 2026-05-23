// ============================================================================
// src/crypto/identity.go - Ed25519 Identity (Compatible con ARM64)
// ============================================================================

package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/mr-tron/base58"
)

// DID estructura del identificador descentralizado
type DID struct {
	Method string
	Hash   []byte
}

func (d *DID) String() string {
	return d.Method + ":" + base58.Encode(d.Hash)
}

type Identity struct {
	DID            *DID
	PrivateKey     ed25519.PrivateKey
	PublicKey      ed25519.PublicKey
	Name           string
	CreatedAt      time.Time
	LastSeen       time.Time
	Reputation     uint64
	SignatureCurve string
}

func NewIdentity(name string) (*Identity, error) {
	// Generar clave privada Ed25519 (compatible con ARM64)
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}
	
	// Generar DID a partir del hash de la clave pública
	hash := sha256.Sum256(pubKey)
	did := &DID{
		Method: "did:maia",
		Hash:   hash[:],
	}
	
	now := time.Now()
	return &Identity{
		DID:            did,
		PrivateKey:     privKey,
		PublicKey:      pubKey,
		Name:           name,
		CreatedAt:      now,
		LastSeen:       now,
		Reputation:     100,
		SignatureCurve: "ed25519",
	}, nil
}

// Sign firma un mensaje usando Ed25519
func (id *Identity) Sign(data []byte) ([]byte, error) {
	if id.PrivateKey == nil {
		return nil, fmt.Errorf("no private key available")
	}
	
	signature := ed25519.Sign(id.PrivateKey, data)
	return signature, nil
}

// Verify verifica una firma Ed25519
func (id *Identity) Verify(data []byte, signature []byte) bool {
	if id.PublicKey == nil {
		return false
	}
	
	return ed25519.Verify(id.PublicKey, data, signature)
}

func (id *Identity) GetDIDString() string {
	return id.DID.String()
}

func (id *Identity) GetPublicKeyHex() string {
	return hex.EncodeToString(id.PublicKey)
}

func (id *Identity) GetPublicKeyCompressed() []byte {
	return id.PublicKey
}
