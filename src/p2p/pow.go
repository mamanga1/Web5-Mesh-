package p2p

import (
    "crypto/sha256"
    "encoding/binary"
)

// Difficulty niveles de PoW (3-4 ceros iniciales = ~16-32 intentos promedio)
const DefaultPoWDifficulty = 4

// HashWithNonce calcula hash de pubKey + nonce
func HashWithNonce(pubKey []byte, nonce uint64) [32]byte {
    data := make([]byte, len(pubKey)+8)
    copy(data, pubKey)
    binary.BigEndian.PutUint64(data[len(pubKey):], nonce)
    return sha256.Sum256(data)
}

// CheckPoW verifica si el hash cumple la dificultad (primeros 'bits' bits en cero)
func CheckPoW(hash [32]byte, bits int) bool {
    if bits <= 0 {
        return true
    }
    if bits > 32 {
        bits = 32
    }
    // Verificar bytes completos
    bytes := bits / 8
    for i := 0; i < bytes; i++ {
        if hash[i] != 0 {
            return false
        }
    }
    // Verificar bits restantes
    remaining := bits % 8
    if remaining > 0 && (hash[bytes]>>(8-remaining)) != 0 {
        return false
    }
    return true
}

// FindNonce busca un nonce que cumpla la dificultad
func FindNonce(pubKey []byte, bits int, maxAttempts uint64) (uint64, [32]byte, bool) {
    for nonce := uint64(0); nonce < maxAttempts; nonce++ {
        hash := HashWithNonce(pubKey, nonce)
        if CheckPoW(hash, bits) {
            return nonce, hash, true
        }
    }
    return 0, [32]byte{}, false
}

// ValidateNodeID valida que un NodeID cumpla con la PoW
func ValidateNodeID(nodeID NodeID, pubKey []byte, nonce uint64, bits int) bool {
    expectedHash := HashWithNonce(pubKey, nonce)
    for i := 0; i < 20; i++ {
        if expectedHash[i] != nodeID[i] {
            return false
        }
    }
    return CheckPoW(expectedHash, bits)
}
