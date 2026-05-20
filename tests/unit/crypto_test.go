// ============================================================================
// tests/unit/crypto_test.go - Cryptographic Operations Unit Tests
// ============================================================================
// Especificación:
// - Validación de performance del cifrado simétrico
// - Pruebas de ChaCha20-Poly1305 encryption/decryption
// - Tests de integridad y autenticación
// ============================================================================

package unit

import (
	"bytes"
	"crypto/rand"
	"testing"

	"web5-mesh/src/crypto"
)

// TestEncryptionDecryption prueba el cifrado y descifrado de datos
func TestEncryptionDecryption(t *testing.T) {
	sessionKey, err := crypto.GenerateRandomKey()
	if err != nil {
		t.Fatalf("Failed to generate session key: %v", err)
	}

	testCases := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"small", []byte("Hello, MaIA Mesh!")},
		{"medium", bytes.Repeat([]byte("A"), 1024)},
		{"large", bytes.Repeat([]byte("B"), 65536)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Cifrar
			encrypted, err := crypto.EncryptBytes(tc.data, sessionKey)
			if err != nil {
				t.Fatalf("Encryption failed: %v", err)
			}

			// Descifrar
			decrypted, err := crypto.DecryptBytes(encrypted, sessionKey)
			if err != nil {
				t.Fatalf("Decryption failed: %v", err)
			}

			// Comparar
			if !bytes.Equal(tc.data, decrypted) {
				t.Errorf("Data mismatch: expected %v, got %v", tc.data, decrypted)
			}
		})
	}
}

// TestEncryptionWithAdditionalData prueba AEAD con datos adicionales
func TestEncryptionWithAdditionalData(t *testing.T) {
	sessionKey, err := crypto.GenerateRandomKey()
	if err != nil {
		t.Fatalf("Failed to generate session key: %v", err)
	}

	plaintext := []byte("secret message")
	additionalData := []byte("public metadata")

	encrypted, err := crypto.EncryptWithAdditionalData(plaintext, additionalData, sessionKey)
	if err != nil {
		t.Fatalf("Encryption with AD failed: %v", err)
	}

	// Descifrar con AD correcto
	decrypted, err := crypto.DecryptWithAdditionalData(encrypted, additionalData, sessionKey)
	if err != nil {
		t.Fatalf("Decryption with correct AD failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Error("Data mismatch with correct AD")
	}

	// Descifrar con AD incorrecto
	wrongAD := []byte("wrong metadata")
	_, err = crypto.DecryptWithAdditionalData(encrypted, wrongAD, sessionKey)
	if err == nil {
		t.Error("Decryption with wrong AD should fail")
	}
}

// TestEncryptionIntegrity prueba que los datos cifrados no puedan ser modificados
func TestEncryptionIntegrity(t *testing.T) {
	sessionKey, err := crypto.GenerateRandomKey()
	if err != nil {
		t.Fatalf("Failed to generate session key: %v", err)
	}

	originalData := []byte("important data")
	encrypted, err := crypto.EncryptBytes(originalData, sessionKey)
	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	// Modificar el ciphertext
	if len(encrypted) > 0 {
		modified := make([]byte, len(encrypted))
		copy(modified, encrypted)
		modified[0] ^= 0xFF

		_, err := crypto.DecryptBytes(modified, sessionKey)
		if err == nil {
			t.Error("Modified ciphertext should fail decryption")
		}
	}
}

// TestSessionKeyDerivation prueba la derivación de claves de sesión
func TestSessionKeyDerivation(t *testing.T) {
	sharedSecret := []byte("shared-secret-from-handshake")
	salt := []byte("random-salt")

	key1 := crypto.DeriveSessionKey(sharedSecret, salt)
	key2 := crypto.DeriveSessionKey(sharedSecret, salt)

	if !bytes.Equal(key1[:], key2[:]) {
		t.Error("Session key derivation not deterministic")
	}

	// Diferente salt debe dar diferente clave
	salt2 := []byte("different-salt")
	key3 := crypto.DeriveSessionKey(sharedSecret, salt2)

	if bytes.Equal(key1[:], key3[:]) {
		t.Error("Different salt produced same key")
	}
}

// TestEncryptionPerformance prueba el rendimiento del cifrado
func TestEncryptionPerformance(t *testing.T) {
	sessionKey, err := crypto.GenerateRandomKey()
	if err != nil {
		t.Fatalf("Failed to generate session key: %v", err)
	}

	sizes := []int{1024, 10240, 102400} // 1KB, 10KB, 100KB

	for _, size := range sizes {
		t.Run("size_"+string(rune(size)), func(t *testing.T) {
			data := make([]byte, size)
			rand.Read(data)

			encrypted, err := crypto.EncryptBytes(data, sessionKey)
			if err != nil {
				t.Fatalf("Encryption failed for size %d: %v", size, err)
			}

			decrypted, err := crypto.DecryptBytes(encrypted, sessionKey)
			if err != nil {
				t.Fatalf("Decryption failed for size %d: %v", size, err)
			}

			if !bytes.Equal(data, decrypted) {
				t.Errorf("Data mismatch for size %d", size)
			}

			t.Logf("Size %d: original=%d, encrypted=%d", size, len(data), len(encrypted))
		})
	}
}

// TestRandomKeyGeneration prueba la generación de claves aleatorias
func TestRandomKeyGeneration(t *testing.T) {
	key1, err := crypto.GenerateRandomKey()
	if err != nil {
		t.Fatalf("Failed to generate key 1: %v", err)
	}

	key2, err := crypto.GenerateRandomKey()
	if err != nil {
		t.Fatalf("Failed to generate key 2: %v", err)
	}

	if bytes.Equal(key1[:], key2[:]) {
		t.Error("Generated identical keys")
	}

	// Verificar que la clave tiene la longitud correcta
	if len(key1) != 32 {
		t.Errorf("Expected key length 32, got %d", len(key1))
	}
}

// TestZeroKey prueba la limpieza segura de claves
func TestZeroKey(t *testing.T) {
	key, err := crypto.GenerateRandomKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	// Guardar copia
	keyCopy := key

	// Limpiar
	crypto.ZeroKey(&key)

	// Verificar que fue limpiada
	allZero := true
	for i := 0; i < len(key); i++ {
		if key[i] != 0 {
			allZero = false
			break
		}
	}

	if !allZero {
		t.Error("ZeroKey did not clear all bytes")
	}

	// La copia original no debe verse afectada
	same := bytes.Equal(keyCopy[:], key[:])
	if same {
		t.Error("ZeroKey affected original slice")
	}
}

// TestEncryptDecryptPayload prueba las funciones con struct
func TestEncryptDecryptPayload(t *testing.T) {
	sessionKey, err := crypto.GenerateRandomKey()
	if err != nil {
		t.Fatalf("Failed to generate session key: %v", err)
	}

	plaintext := []byte("test payload for encrypt/decrypt")

	msg, err := crypto.EncryptPayload(plaintext, sessionKey)
	if err != nil {
		t.Fatalf("EncryptPayload failed: %v", err)
	}

	if len(msg.Nonce) != 12 {
		t.Errorf("Expected nonce length 12, got %d", len(msg.Nonce))
	}

	decrypted, err := crypto.DecryptPayload(msg, sessionKey)
	if err != nil {
		t.Fatalf("DecryptPayload failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Error("Payload data mismatch")
	}
}

// TestEncryptDecryptEmptyPayload prueba con payload vacío
func TestEncryptDecryptEmptyPayload(t *testing.T) {
	sessionKey, err := crypto.GenerateRandomKey()
	if err != nil {
		t.Fatalf("Failed to generate session key: %v", err)
	}

	plaintext := []byte{}

	encrypted, err := crypto.EncryptBytes(plaintext, sessionKey)
	if err != nil {
		t.Fatalf("Encryption of empty data failed: %v", err)
	}

	decrypted, err := crypto.DecryptBytes(encrypted, sessionKey)
	if err != nil {
		t.Fatalf("Decryption of empty data failed: %v", err)
	}

	if len(decrypted) != 0 {
		t.Errorf("Expected empty decrypted, got length %d", len(decrypted))
	}
}
