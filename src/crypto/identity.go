// ============================================================================
// src/crypto/identity.go - Secp256k1 Identity + Base58 DIDs
// ============================================================================
// Especificación:
// - Reemplazar curvas elípticas estándar por secp256k1
// - DID did:maia: + Base58( SHA-256(public_key) ) = 32 bytes
// - Métodos Sign() y Verify() para firmas ECDSA
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

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/mr-tron/base58"
)

// DID estructura del identificador descentralizado
// Formato: did:maia:[32-byte Base58]
type DID struct {
	Method string // Siempre "did:maia"
	Hash   []byte // 32 bytes SHA-256 de la clave pública comprimida
}

// String retorna la representación string del DID
func (d *DID) String() string {
	return d.Method + ":" + base58.Encode(d.Hash)
}

// Identity estructura con material criptográfico completo
type Identity struct {
	DID         *DID                    // Identificador público
	PrivateKey  *secp256k1.PrivateKey   // Clave privada secp256k1
	PublicKey   *secp256k1.PublicKey    // Clave pública secp256k1
	Name        string                  // Nombre legible por humanos (opcional)
	CreatedAt   time.Time               // Timestamp de creación
	LastSeen    time.Time               // Para detección de liveness
	Reputation  uint64                  // Trust score (1-1000)
	SignatureCurve string               // "secp256k1"
}

// NewIdentity genera una nueva identidad criptográfica con secp256k1
func NewIdentity(name string) (*Identity, error) {
	// Generar par de claves secp256k1
	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	pubKey := privKey.PubKey()

	// Derivar DID desde clave pública comprimida
	// SHA-256 de la clave pública comprimida (32 bytes)
	pubKeyCompressed := pubKey.SerializeCompressed()
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
		Reputation:     100, // Trust inicial
		SignatureCurve: "secp256k1",
	}, nil
}

// Sign firma datos usando la clave privada ECDSA
// Retorna firma en formato [R || S] (64 bytes, 32+32)
func (id *Identity) Sign(data []byte) ([]byte, error) {
	if id.PrivateKey == nil {
		return nil, fmt.Errorf("no private key available")
	}

	// Calcular hash SHA-256 de los datos
	hash := sha256.Sum256(data)

	// Firmar usando secp256k1
	signature, err := id.PrivateKey.Sign(hash[:])
	if err != nil {
		return nil, fmt.Errorf("signing failed: %w", err)
	}

	// La firma tiene formato [R || S] de 64 bytes
	return signature.Serialize(), nil
}

// Verify verifica una firma usando la clave pública
// signature: firma en formato [R || S] (64 bytes) o [R || S || V] (65 bytes)
func (id *Identity) Verify(data []byte, signature []byte) bool {
	if id.PublicKey == nil {
		return false
	}

	// Validar tamaño mínimo
	if len(signature) < 64 {
		return false
	}

	// Calcular hash SHA-256
	hash := sha256.Sum256(data)

	// Extraer R y S (primeros 32 bytes son R, siguientes 32 son S)
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:64])

	// Convertir secp256k1.PublicKey a ecdsa.PublicKey para verificación
	ecdsaPubKey := ecdsa.PublicKey{
		Curve: elliptic.P256(), // Placeholder - secp256k1 usa su propia curva
		X:     nil,
		Y:     nil,
	}

	// Método alternativo: usar la verificación nativa de secp256k1
	// Deserializar la firma
	var sig secp256k1.Signature
	if err := sig.ParseDERSignature(signature); err != nil {
		// Si falla, intentar formato [R||S]
		sig.SetRS(r, s)
	}

	return sig.Verify(hash[:], id.PublicKey)
}

// GetDIDString retorna el DID como string legible
func (id *Identity) GetDIDString() string {
	return id.DID.String()
}

// GetPublicKeyHex retorna la clave pública en formato hexadecimal
func (id *Identity) GetPublicKeyHex() string {
	return hex.EncodeToString(id.PublicKey.SerializeCompressed())
}

// LoadIdentityFromHex carga una identidad desde clave privada hexadecimal
func LoadIdentityFromHex(privateKeyHex string, name string) (*Identity, error) {
	privKeyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid hex: %w", err)
	}

	privKey := secp256k1.PrivKeyFromBytes(privKeyBytes)
	pubKey := privKey.PubKey()

	pubKeyCompressed := pubKey.SerializeCompressed()
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
