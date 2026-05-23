// ============================================================================
// src/crypto/identity.go - Secp256k1 Identity + Base58 DIDs
// ============================================================================

package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
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
	PrivateKey     *ecdsa.PrivateKey
	PublicKey      *ecdsa.PublicKey
	Name           string
	CreatedAt      time.Time
	LastSeen       time.Time
	Reputation     uint64
	SignatureCurve string
}

func NewIdentity(name string) (*Identity, error) {
	// Generar clave privada secp256k1 usando crypto/ecdsa
	privKey, err := ecdsa.GenerateKey(secp256k1Curve(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}
	pubKey := &privKey.PublicKey
	
	// Serializar clave pública comprimida
	pubKeyCompressed := compressPublicKey(pubKey)
	hash := sha256.Sum256(pubKeyCompressed)
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
		SignatureCurve: "secp256k1",
	}, nil
}

// secp256k1Curve retorna la curva secp256k1
func secp256k1Curve() elliptic.Curve {
	// Parámetros de secp256k1
	// p = 2^256 - 2^32 - 977
	// a = 0
	// b = 7
	// G = (Gx, Gy)
	// n = 115792089237316195423570985008687907852837564279074904382605163141518161494337
	
	type secp256k1CurveParams struct {
		elliptic.CurveParams
	}
	
	params := &secp256k1CurveParams{}
	params.P, _ = new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEFFFFFC2F", 16)
	params.N, _ = new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141", 16)
	params.B, _ = new(big.Int).SetString("0000000000000000000000000000000000000000000000000000000000000007", 16)
	params.Gx, _ = new(big.Int).SetString("79BE667EF9DCBBAC55A06295CE870B07029BFCDB2DCE28D959F2815B16F81798", 16)
	params.Gy, _ = new(big.Int).SetString("483ADA7726A3C4655DA4FBFC0E1108A8FD17B448A68554199C47D08FFB10D4B8", 16)
	params.BitSize = 256
	params.Name = "secp256k1"
	
	return params
}

// compressPublicKey comprime una clave pública en formato 33 bytes (02/03 + X)
func compressPublicKey(pub *ecdsa.PublicKey) []byte {
	// Formato comprimido: 0x02 o 0x03 + X (32 bytes)
	compressed := make([]byte, 33)
	xBytes := pub.X.Bytes()
	
	// Rellenar X a 32 bytes
	offset := 32 - len(xBytes)
	for i := 0; i < len(xBytes); i++ {
		compressed[offset+i+1] = xBytes[i]
	}
	
	// Determinar si Y es par (0x02) o impar (0x03)
	if pub.Y.Bit(0) == 0 {
		compressed[0] = 0x02
	} else {
		compressed[0] = 0x03
	}
	
	return compressed
}

// Sign firma un mensaje usando ECDSA secp256k1
func (id *Identity) Sign(data []byte) ([]byte, error) {
	if id.PrivateKey == nil {
		return nil, fmt.Errorf("no private key available")
	}
	hash := sha256.Sum256(data)
	r, s, err := ecdsa.Sign(rand.Reader, id.PrivateKey, hash[:])
	if err != nil {
		return nil, fmt.Errorf("signing failed: %w", err)
	}
	
	// Serializar firma: r (32 bytes) + s (32 bytes)
	signature := make([]byte, 64)
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	
	copy(signature[32-len(rBytes):32], rBytes)
	copy(signature[64-len(sBytes):64], sBytes)
	
	return signature, nil
}

// Verify verifica una firma ECDSA secp256k1
func (id *Identity) Verify(data []byte, signature []byte) bool {
	if id.PublicKey == nil {
		return false
	}
	if len(signature) < 64 {
		return false
	}
	
	hash := sha256.Sum256(data)
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:64])
	
	return ecdsa.Verify(id.PublicKey, hash[:], r, s)
}

func (id *Identity) GetDIDString() string {
	return id.DID.String()
}

func (id *Identity) GetPublicKeyHex() string {
	pubCompressed := compressPublicKey(id.PublicKey)
	return hex.EncodeToString(pubCompressed)
}
