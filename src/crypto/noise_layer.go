// ============================================================================
// src/crypto/noise_layer.go - Noise Protocol Handshake (KK pattern)
// ============================================================================
// Especificación:
// - Handshake criptográfico para canales de comunicación seguros
// - Patrón KK (Known Key): ambas partes conocen las claves públicas del otro
// - Deriva claves de sesión efímeras para forward secrecy
// ============================================================================

package crypto

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/chacha20poly1305"
)

// Errores de Noise
var (
	ErrInvalidHandshakeState = errors.New("invalid handshake state")
	ErrHandshakeTimeout      = errors.New("handshake timeout")
	ErrInvalidMessage        = errors.New("invalid handshake message")
)

// NoiseHandshakeState representa el estado del handshake Noise (patrón KK)
type NoiseHandshakeState struct {
	// Estado del handshake
	Step int // 0: init, 1: sent_ephemeral, 2: complete

	// Claves estáticas (long-term)
	LocalStatic  [32]byte // Clave privada estática local
	RemoteStatic [32]byte // Clave pública estática remota (conocida previamente)

	// Claves efímeras (ephemeral) - para forward secrecy
	LocalEphemeral  [32]byte // Clave privada efímera local
	RemoteEphemeral [32]byte // Clave pública efímera remota

	// Claves de sesión derivadas
	SessionKeyLocalToRemote  [32]byte // Cifrado local -> remoto
	SessionKeyRemoteToLocal  [32]byte // Cifrado remoto -> local
	SessionNonceLocal        uint64   // Nonce para local->remote
	SessionNonceRemote       uint64   // Nonce para remote->local

	// Estado de la máquina
	IsInitiator bool
	Complete    bool
}

// NewNoiseHandshake crea un nuevo handshake Noise
// - initiator: true si somos el que inicia la conexión
// - localStatic: nuestra clave privada estática (32 bytes)
// - remoteStatic: clave pública estática del peer (conocida previamente)
func NewNoiseHandshake(initiator bool, localStatic, remoteStatic [32]byte) *NoiseHandshakeState {
	return &NoiseHandshakeState{
		Step:         0,
		IsInitiator:  initiator,
		LocalStatic:  localStatic,
		RemoteStatic: remoteStatic,
		Complete:     false,
	}
}

// GenerateEphemeralKey genera una clave efímera aleatoria
func (n *NoiseHandshakeState) GenerateEphemeralKey() error {
	_, err := rand.Read(n.LocalEphemeral[:])
	if err != nil {
		return fmt.Errorf("failed to generate ephemeral key: %w", err)
	}
	return nil
}

// WriteMessage genera el próximo mensaje de handshake
// Retorna el mensaje a enviar al peer
func (n *NoiseHandshakeState) WriteMessage() ([]byte, error) {
	switch n.Step {
	case 0:
		// Paso 0: Iniciador envía su clave efímera
		if !n.IsInitiator {
			return nil, fmt.Errorf("responder espera mensaje entrante")
		}
		if err := n.GenerateEphemeralKey(); err != nil {
			return nil, err
		}
		n.Step = 1
		return n.LocalEphemeral[:], nil

	case 1:
		// Paso 1: Responder envía su clave efímera + cifrado con clave estática
		if n.IsInitiator {
			return nil, fmt.Errorf("iniciador ya envió su mensaje")
		}
		if err := n.GenerateEphemeralKey(); err != nil {
			return nil, err
		}

		// Calcular secreto compartido (ECDH entre ephemeral local y remote static)
		sharedSecret := n.calculateDH(n.LocalEphemeral, n.RemoteStatic)

		// Derivar clave de cifrado para el mensaje cifrado
		cipherKey := n.deriveCipherKey(sharedSecret, []byte("handshake_cipher"))

		// Construir payload: ephemeral public key + encrypted static signature
		payload := n.RemoteEphemeral[:]

		// Cifrar el payload
		encrypted, err := n.encryptHandshakeMessage(payload, cipherKey)
		if err != nil {
			return nil, err
		}

		n.Step = 2
		return encrypted, nil

	default:
		return nil, ErrInvalidHandshakeState
	}
}

// ReadMessage procesa un mensaje entrante y actualiza el estado del handshake
func (n *NoiseHandshakeState) ReadMessage(msg []byte) error {
	switch n.Step {
	case 0:
		// Responder recibe la clave efímera del iniciador
		if n.IsInitiator {
			return fmt.Errorf("iniciador no debería recibir mensaje en paso 0")
		}
		if len(msg) != 32 {
			return ErrInvalidMessage
		}
		copy(n.RemoteEphemeral[:], msg[:32])
		n.Step = 1
		return nil

	case 1:
		// Iniciador recibe la respuesta cifrada del responder
		if !n.IsInitiator {
			return fmt.Errorf("responder no debería recibir mensaje en paso 1")
		}
		if len(msg) < 32+16 { // Mínimo: clave efímera (32) + tag (16)
			return ErrInvalidMessage
		}

		// Extraer clave efímera remota (primeros 32 bytes sin cifrar)
		copy(n.RemoteEphemeral[:], msg[:32])

		// Calcular secreto compartido
		sharedSecret := n.calculateDH(n.LocalEphemeral, n.RemoteStatic)

		// Derivar clave de cifrado
		cipherKey := n.deriveCipherKey(sharedSecret, []byte("handshake_cipher"))

		// Descifrar el resto del mensaje
		decrypted, err := n.decryptHandshakeMessage(msg[32:], cipherKey)
		if err != nil {
			return err
		}

		// Verificar que el decrypted contiene la clave efímera (debería ser 32 bytes)
		if len(decrypted) < 32 {
			return ErrInvalidMessage
		}

		// Actualizar estado
		n.Step = 2
		n.Complete = true

		// Derivar claves de sesión finales
		return n.deriveSessionKeys()

	case 2:
		// Responder completa el handshake después de enviar su mensaje
		if n.IsInitiator {
			return nil
		}
		if !n.Complete {
			n.Complete = true
			return n.deriveSessionKeys()
		}
		return nil

	default:
		return ErrInvalidHandshakeState
	}
}

// calculateDH realiza ECDH entre clave privada local y clave pública remota
// En una implementación real, usar X25519 o secp256k1
func (n *NoiseHandshakeState) calculateDH(private, public [32]byte) [32]byte {
	// Placeholder: En producción implementar X25519 o secp256k1 ECDH
	// Por ahora, retornar hash(private || public)
	hash := blake2b.Sum256(append(private[:], public[:]...))
	return hash
}

// deriveCipherKey deriva una clave de cifrado desde el secreto compartido
func (n *NoiseHandshakeState) deriveCipherKey(sharedSecret [32]byte, context []byte) [32]byte {
	data := make([]byte, 0, 32+len(context))
	data = append(data, sharedSecret[:]...)
	data = append(data, context...)
	hash := blake2b.Sum256(data)
	return hash
}

// encryptHandshakeMessage cifra un mensaje del handshake
func (n *NoiseHandshakeState) encryptHandshakeMessage(plaintext []byte, key [32]byte) ([]byte, error) {
	aead, err := chacha20poly1305.New(key[:])
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, chacha20poly1305.NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	ciphertext := aead.Seal(nil, nonce, plaintext, nil)

	// Formato: nonce (12) + ciphertext
	result := make([]byte, 0, len(nonce)+len(ciphertext))
	result = append(result, nonce...)
	result = append(result, ciphertext...)

	return result, nil
}

// decryptHandshakeMessage descifra un mensaje del handshake
func (n *NoiseHandshakeState) decryptHandshakeMessage(data []byte, key [32]byte) ([]byte, error) {
	if len(data) < chacha20poly1305.NonceSize+chacha20poly1305.Overhead() {
		return nil, ErrInvalidMessage
	}

	nonce := data[:chacha20poly1305.NonceSize]
	ciphertext := data[chacha20poly1305.NonceSize:]

	aead, err := chacha20poly1305.New(key[:])
	if err != nil {
		return nil, err
	}

	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrInvalidMessage
	}

	return plaintext, nil
}

// deriveSessionKeys deriva las claves de sesión finales después del handshake
func (n *NoiseHandshakeState) deriveSessionKeys() error {
	// Combinar todas las claves para derivar claves de sesión
	// static_static + ephemeral_static + static_ephemeral + ephemeral_ephemeral
	var combined [4][32]byte

	combined[0] = n.calculateDH(n.LocalStatic, n.RemoteStatic)
	combined[1] = n.calculateDH(n.LocalEphemeral, n.RemoteStatic)
	combined[2] = n.calculateDH(n.LocalStatic, n.RemoteEphemeral)
	combined[3] = n.calculateDH(n.LocalEphemeral, n.RemoteEphemeral)

	// Construir semilla
	seed := make([]byte, 0, 128)
	for i := 0; i < 4; i++ {
		seed = append(seed, combined[i][:]...)
	}

	// Derivar claves de sesión con contexto
	sessionKeyMaterial := blake2b.Sum512(seed)

	// Clave local->remote (primeros 32 bytes)
	copy(n.SessionKeyLocalToRemote[:], sessionKeyMaterial[:32])

	// Clave remote->local (siguientes 32 bytes)
	copy(n.SessionKeyRemoteToLocal[:], sessionKeyMaterial[32:64])

	// Inicializar nonces
	n.SessionNonceLocal = 0
	n.SessionNonceRemote = 0

	return nil
}

// EncryptSessionMessage cifra un mensaje de sesión usando la clave local->remote
func (n *NoiseHandshakeState) EncryptSessionMessage(plaintext []byte) ([]byte, error) {
	if !n.Complete {
		return nil, ErrInvalidHandshakeState
	}

	// Incrementar nonce y usarlo
	n.SessionNonceLocal++

	// Nonce de 12 bytes para ChaCha20
	nonce := make([]byte, 12)
	binary.LittleEndian.PutUint64(nonce[4:], n.SessionNonceLocal)

	aead, err := chacha20poly1305.New(n.SessionKeyLocalToRemote[:])
	if err != nil {
		return nil, err
	}

	ciphertext := aead.Seal(nil, nonce, plaintext, nil)

	return ciphertext, nil
}

// DecryptSessionMessage descifra un mensaje de sesión usando la clave remote->local
func (n *NoiseHandshakeState) DecryptSessionMessage(ciphertext []byte) ([]byte, error) {
	if !n.Complete {
		return nil, ErrInvalidHandshakeState
	}

	// Incrementar nonce remoto
	n.SessionNonceRemote++

	nonce := make([]byte, 12)
	binary.LittleEndian.PutUint64(nonce[4:], n.SessionNonceRemote)

	aead, err := chacha20poly1305.New(n.SessionKeyRemoteToLocal[:])
	if err != nil {
		return nil, err
	}

	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

// IsComplete retorna true si el handshake finalizó correctamente
func (n *NoiseHandshakeState) IsComplete() bool {
	return n.Complete
}

// GetSessionKeys retorna las claves de sesión (para depuración/monitoreo)
func (n *NoiseHandshakeState) GetSessionKeys() (localToRemote, remoteToLocal [32]byte) {
	return n.SessionKeyLocalToRemote, n.SessionKeyRemoteToLocal
}

// Reset nonces (usado después de establecer la conexión)
func (n *NoiseHandshakeState) ResetNonces() {
	n.SessionNonceLocal = 0
	n.SessionNonceRemote = 0
}
