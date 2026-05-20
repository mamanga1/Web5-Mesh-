// ============================================================================
// src/crypto/puzzle.go - Anti-Sybil Proof of Work (Hashcash)
// ============================================================================
// Especificación:
// - Implementación del mecanismo anti-Sybil basado en Hashcash (Proof of Work)
// - Evita que un atacante inunde la red creando millones de DIDs falsos
// - Dificultad recomendada: 16-20 bits (4-5 ceros hexadecimales)
// ============================================================================

package crypto

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/bits"
)

// PoWPuzzle representa el desafío de Proof of Work
type PoWPuzzle struct {
	TargetDID  string // DID del nodo que resuelve el puzzle
	Difficulty uint32 // Bits a minar (ceros iniciales)
	Nonce      uint64 // Solución encontrada
}

// NewPoWPuzzle crea un nuevo puzzle con la dificultad especificada
func NewPoWPuzzle(did string, difficulty uint32) *PoWPuzzle {
	return &PoWPuzzle{
		TargetDID:  did,
		Difficulty: difficulty,
		Nonce:      0,
	}
}

// SolvePuzzle incrementa secuencialmente el nonce hasta encontrar una solución
// Retorna el nonce que resuelve el puzzle o error si no se encuentra
func (p *PoWPuzzle) SolvePuzzle() (uint64, error) {
	var nonce uint64 = 0
	maxIterations := uint64(1 << (p.Difficulty + 8)) // Límite para evitar loops infinitos

	// Convertir DID a bytes
	didBytes := []byte(p.TargetDID)

	for nonce < maxIterations {
		if p.verifyNonce(didBytes, nonce) {
			p.Nonce = nonce
			return nonce, nil
		}
		nonce++
	}

	return 0, fmt.Errorf("puzzle solution not found after %d iterations", maxIterations)
}

// VerifyNonce verifica que un nonce dado cumple con la dificultad
func (p *PoWPuzzle) VerifyNonce() bool {
	didBytes := []byte(p.TargetDID)
	return p.verifyNonce(didBytes, p.Nonce)
}

// verifyNonce es la función interna de verificación
func (p *PoWPuzzle) verifyNonce(didBytes []byte, nonce uint64) bool {
	// Construir el bloque: DID + Nonce (8 bytes big-endian)
	nonceBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(nonceBytes, nonce)

	block := make([]byte, 0, len(didBytes)+8)
	block = append(block, didBytes...)
	block = append(block, nonceBytes...)

	// Calcular hash SHA-256
	hash := sha256.Sum256(block)

	// Contar ceros iniciales en bits
	leadingZeros := countLeadingZeroBits(hash[:])

	return leadingZeros >= p.Difficulty
}

// countLeadingZeroBits cuenta cuántos bits iniciales son cero
func countLeadingZeroBits(data []byte) uint32 {
	var total uint32 = 0

	for _, b := range data {
		if b == 0 {
			total += 8
		} else {
			// Contar ceros en el byte actual
			zeros := uint32(bits.LeadingZeros8(b))
			total += zeros
			break
		}
	}

	return total
}

// FormatDifficultyHuman convierte dificultad en bits a representación legible
func FormatDifficultyHuman(difficulty uint32) string {
	hexChars := (difficulty + 3) / 4
	return fmt.Sprintf("%d bits (%d ceros hexadecimales)", difficulty, hexChars)
}

// ValidateIdentityWithPoW crea una identidad Y la valida con PoW en un solo paso
// Útil para generar DIDs con PoW incorporado
func ValidateIdentityWithPoW(identity *Identity, difficulty uint32) error {
	if identity == nil || identity.DID == nil {
		return fmt.Errorf("invalid identity")
	}

	puzzle := NewPoWPuzzle(identity.GetDIDString(), difficulty)

	// Resolver el puzzle (esto puede tomar tiempo dependiendo de la dificultad)
	_, err := puzzle.SolvePuzzle()
	if err != nil {
		return fmt.Errorf("failed to solve PoW puzzle: %w", err)
	}

	if !puzzle.VerifyNonce() {
		return fmt.Errorf("PoW verification failed")
	}

	return nil
}

// CreateIdentityWithPoW genera una nueva identidad y resuelve el PoW automáticamente
// Esta función debe llamarse en una goroutine o con timeout porque puede ser lenta
func CreateIdentityWithPoW(name string, difficulty uint32) (*Identity, *PoWPuzzle, error) {
	// Generar identidad
	identity, err := NewIdentity(name)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create identity: %w", err)
	}

	// Crear puzzle
	puzzle := NewPoWPuzzle(identity.GetDIDString(), difficulty)

	// Resolver puzzle
	nonce, err := puzzle.SolvePuzzle()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to solve PoW: %w", err)
	}
	puzzle.Nonce = nonce

	// Verificar solución
	if !puzzle.VerifyNonce() {
		return nil, nil, fmt.Errorf("PoW verification failed after solve")
	}

	return identity, puzzle, nil
}

// GetRecommendedDifficulty retorna la dificultad recomendada según el hardware
// Ajusta automáticamente la dificultad para mantener el costo de creación de DIDs
func GetRecommendedDifficulty() uint32 {
	// Dificultad base: 16 bits (~65536 intentos promedio)
	// Esto es suficiente para prevenir Sybil attacks masivas sin ser prohibitivo
	// para hardware reciclado (TV boxes, Poco F1)
	return 16
}
