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
	// Validar tamaño mínimo
	if len(encrypted.Ciphertext) < chacha20poly1305.Overhead {
		return nil, ErrInvalidCiphertext
	}

	if len(encrypted.Nonce) != chacha20poly1305.NonceSize {
		return nil, fmt.Errorf("invalid nonce size: expected %d, got %d",
			chacha20poly1305.NonceSize, len(encrypted.Nonce))
	}

	// Crear cifrador
	aead, err := chacha20poly1305.New(sessionKey[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Descifrar (Open verifica el tag automáticamente)
	plaintext, err := aead.Open(nil, encrypted.Nonce, encrypted.Ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

// EncryptBytes es una función de conveniencia que retorna los bytes concatenados
// Formato: [nonce (12 bytes)][ciphertext]
func EncryptBytes(plaintext []byte, sessionKey [32]byte) ([]byte, error) {
	msg, err := EncryptPayload(plaintext, sessionKey)
	if err != nil {
		return nil, err
	}

	// Concatenar nonce + ciphertext
	result := make([]byte, 0, len(msg.Nonce)+len(msg.Ciphertext))
	result = append(result, msg.Nonce...)
	result = append(result, msg.Ciphertext...)

	return result, nil
}

// DecryptBytes es la contraparte de EncryptBytes
// Espera formato: [nonce (12 bytes)][ciphertext]
func DecryptBytes(encrypted []byte, sessionKey [32]byte) ([]byte, error) {
	if len(encrypted) < chacha20poly1305.NonceSize+chacha20poly1305.Overhead {
		return nil, ErrInvalidCiphertext
	}

	nonce := encrypted[:chacha20poly1305.NonceSize]
	ciphertext := encrypted[chacha20poly1305.NonceSize:]

	return DecryptPayload(&EncryptedMessage{
		Nonce:      nonce,
		Ciphertext: ciphertext,
	}, sessionKey)
}

// GenerateRandomKey genera una clave aleatoria de 32 bytes para sesiones
func GenerateRandomKey() ([32]byte, error) {
	var key [32]byte
	_, err := rand.Read(key[:])
	if err != nil {
		return key, fmt.Errorf("failed to generate random key: %w", err)
	}
	return key, nil
}

// ZeroKey sobrescribe la clave con ceros (para limpieza segura)
func ZeroKey(key *[32]byte) {
	for i := range key {
		key[i] = 0
	}
}

// EncryptWithAdditionalData cifra con datos adicionales autenticados (AEAD)
// Los datos adicionales no se cifran pero se incluyen en el tag MAC
func EncryptWithAdditionalData(plaintext, additionalData []byte, sessionKey [32]byte) (*EncryptedMessage, error) {
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
