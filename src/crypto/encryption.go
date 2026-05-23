// ============================================================================
// src/crypto/encryption.go - ChaCha20-Poly1305 Symmetric Encryption
// ============================================================================
// Especificación:
// - Cifrado simétrico de alta velocidad para comunicación P2P
// - ChaCha20-Poly1305 es más rápido que AES en hardware sin aceleración
// - Ideal para TV boxes y dispositivos móviles (ARM sin AES-NI)
// ============================================================================

package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

// Errores comunes de encriptación
var (
	ErrInvalidKeySize    = errors.New("invalid key size: must be 32 bytes")
	ErrInvalidCiphertext = errors.New("invalid ciphertext: too short")
	ErrDecryptionFailed  = errors.New("decryption failed: authentication mismatch")
)

// EncryptedMessage representa un mensaje cifrado con metadatos
type EncryptedMessage struct {
	Nonce      []byte // Nonce de 12 bytes (ChaCha20 requiere 12)
	Ciphertext []byte // Datos cifrados + Poly1305 tag (16 bytes extra)
}

// DeriveSessionKey deriva una clave de sesión de 32 bytes desde un secreto compartido
// Usa SHA-256 como KDF simple (en producción usar HKDF)
func DeriveSessionKey(sharedSecret []byte, salt []byte) [32]byte {
	// Combinar secreto + salt
	data := make([]byte, 0, len(sharedSecret)+len(salt))
	data = append(data, sharedSecret...)
	data = append(data, salt...)

	// Derivar clave de 32 bytes
	hash := sha256.Sum256(data)
	return hash
}

// EncryptPayload cifra un payload usando ChaCha20-Poly1305
// Retorna: nonce (12 bytes) + ciphertext (datos cifrados + tag)
func EncryptPayload(plaintext []byte, sessionKey [32]byte) (*EncryptedMessage, error) {
	// Crear cifrador ChaCha20-Poly1305
	aead, err := chacha20poly1305.New(sessionKey[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Generar nonce aleatorio de 12 bytes
	nonce := make([]byte, chacha20poly1305.NonceSize) // 12 bytes
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Cifrar: Seal(dst, nonce, plaintext, additionalData)
	// El tag Poly1305 se incluye automáticamente al final del ciphertext
	ciphertext := aead.Seal(nil, nonce, plaintext, nil)

	return &EncryptedMessage{
		Nonce:      nonce,
		Ciphertext: ciphertext,
	}, nil
}

// DecryptPayload descifra un mensaje cifrado con ChaCha20-Poly1305
// Verifica automáticamente la autenticidad (Poly1305 tag)
func DecryptPayload(encrypted *EncryptedMessage, sessionKey [32]byte) ([]byte, error) {
	// Validar tamaño mínimo - CORREGIDO: Overhead es constante, no función
	if len(encrypted.Ciphertext) < chacha20poly1305.Overhead {
		return nil, ErrInvalidCiphertext
	}

	if len(encrypted.Nonce) != chacha20poly1305.NonceSize {
		return nil, fmt.Errorf("invalid nonce size: expected %d, got %d", chacha20poly1305.NonceSize, len(encrypted.Nonce))
	}

	// Crear cifrador
	aead, err := chacha20poly1305.New(sessionKey[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Descifrar y verificar autenticidad
	plaintext, err := aead.Open(nil, encrypted.Nonce, encrypted.Ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

// EncryptBytes versión simplificada: retorna nonce|ciphertext concatenado
func EncryptBytes(plaintext []byte, sessionKey [32]byte) ([]byte, error) {
	// Crear cifrador
	aead, err := chacha20poly1305.New(sessionKey[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Generar nonce
	nonce := make([]byte, chacha20poly1305.NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Cifrar
	ciphertext := aead.Seal(nil, nonce, plaintext, nil)

	// Concatenar nonce + ciphertext
	result := make([]byte, 0, len(nonce)+len(ciphertext))
	result = append(result, nonce...)
	result = append(result, ciphertext...)

	return result, nil
}

// DecryptBytes es la contraparte de EncryptBytes
// Espera formato: [nonce (12 bytes)][ciphertext]
func DecryptBytes(encrypted []byte, sessionKey [32]byte) ([]byte, error) {
	// CORREGIDO: Overhead es constante
	if len(encrypted) < chacha20poly1305.NonceSize+chacha20poly1305.Overhead {
		return nil, ErrInvalidCiphertext
	}

	nonce := encrypted[:chacha20poly1305.NonceSize]
	ciphertext := encrypted[chacha20poly1305.NonceSize:]

	aead, err := chacha20poly1305.New(sessionKey[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

// EncryptWithAdditionalData cifra incluyendo datos adicionales (AD) que no se cifran pero se autentican
func EncryptWithAdditionalData(plaintext []byte, additionalData []byte, sessionKey [32]byte) (*EncryptedMessage, error) {
	aead, err := chacha20poly1305.New(sessionKey[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	nonce := make([]byte, chacha20poly1305.NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := aead.Seal(nil, nonce, plaintext, additionalData)

	return &EncryptedMessage{
		Nonce:      nonce,
		Ciphertext: ciphertext,
	}, nil
}

// DecryptWithAdditionalData descifra verificando datos adicionales
func DecryptWithAdditionalData(encrypted *EncryptedMessage, additionalData []byte, sessionKey [32]byte) ([]byte, error) {
	// CORREGIDO: Overhead es constante
	if len(encrypted.Ciphertext) < chacha20poly1305.Overhead {
		return nil, ErrInvalidCiphertext
	}

	if len(encrypted.Nonce) != chacha20poly1305.NonceSize {
		return nil, fmt.Errorf("invalid nonce size")
	}

	aead, err := chacha20poly1305.New(sessionKey[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	plaintext, err := aead.Open(nil, encrypted.Nonce, encrypted.Ciphertext, additionalData)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}
