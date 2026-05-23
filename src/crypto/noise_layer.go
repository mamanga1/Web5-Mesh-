// ============================================================================
// src/crypto/noise_layer.go - Noise Protocol Handshake (KK pattern)
// ============================================================================
// Especificación:
// - Handshake criptográfico para canales de comunicación seguros
// - Patrón KK (Known Key): ambas partes conocen las claves públicas del otro
// - Deriva claves de sesión efímeras para forward secrecy
// ============================================================================

// ============================================================================
// src/crypto/noise_layer.go - Noise Protocol Handshake (KK pattern)
// ============================================================================

package crypto

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"

	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/chacha20poly1305"
)

var (
	ErrInvalidHandshakeState = errors.New("invalid handshake state")
	ErrHandshakeTimeout      = errors.New("handshake timeout")
	ErrInvalidMessage        = errors.New("invalid handshake message")
)

type NoiseHandshakeState struct {
	Step                   int
	LocalStatic            [32]byte
	RemoteStatic           [32]byte
	LocalEphemeral         [32]byte
	RemoteEphemeral        [32]byte
	SessionKeyLocalToRemote [32]byte
	SessionKeyRemoteToLocal [32]byte
	SessionNonceLocal      uint64
	SessionNonceRemote     uint64
	IsInitiator            bool
	Complete               bool
}

func NewNoiseHandshake(initiator bool, localStatic, remoteStatic [32]byte) *NoiseHandshakeState {
	return &NoiseHandshakeState{
		Step:         0,
		IsInitiator:  initiator,
		LocalStatic:  localStatic,
		RemoteStatic: remoteStatic,
		Complete:     false,
	}
}

func (n *NoiseHandshakeState) GenerateEphemeralKey() error {
	_, err := rand.Read(n.LocalEphemeral[:])
	if err != nil {
		return fmt.Errorf("failed to generate ephemeral key: %w", err)
	}
	return nil
}

func (n *NoiseHandshakeState) WriteMessage() ([]byte, error) {
	switch n.Step {
	case 0:
		if !n.IsInitiator {
			return nil, fmt.Errorf("responder espera mensaje entrante")
		}
		if err := n.GenerateEphemeralKey(); err != nil {
			return nil, err
		}
		n.Step = 1
		return n.LocalEphemeral[:], nil
	case 1:
		if n.IsInitiator {
			return nil, fmt.Errorf("iniciador ya envió su mensaje")
		}
		if err := n.GenerateEphemeralKey(); err != nil {
			return nil, err
		}
		sharedSecret := n.calculateDH(n.LocalEphemeral, n.RemoteStatic)
		cipherKey := n.deriveCipherKey(sharedSecret, []byte("handshake_cipher"))
		payload := n.RemoteEphemeral[:]
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

func (n *NoiseHandshakeState) ReadMessage(msg []byte) error {
	switch n.Step {
	case 0:
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
		if !n.IsInitiator {
			return fmt.Errorf("responder no debería recibir mensaje en paso 1")
		}
		if len(msg) < 32+16 {
			return ErrInvalidMessage
		}
		copy(n.RemoteEphemeral[:], msg[:32])
		sharedSecret := n.calculateDH(n.LocalEphemeral, n.RemoteStatic)
		cipherKey := n.deriveCipherKey(sharedSecret, []byte("handshake_cipher"))
		decrypted, err := n.decryptHandshakeMessage(msg[32:], cipherKey)
		if err != nil {
			return err
		}
		if len(decrypted) < 32 {
			return ErrInvalidMessage
		}
		n.Step = 2
		n.Complete = true
		return n.deriveSessionKeys()
	case 2:
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

func (n *NoiseHandshakeState) calculateDH(private, public [32]byte) [32]byte {
	hash := blake2b.Sum256(append(private[:], public[:]...))
	return hash
}

func (n *NoiseHandshakeState) deriveCipherKey(sharedSecret [32]byte, context []byte) [32]byte {
	data := make([]byte, 0, 32+len(context))
	data = append(data, sharedSecret[:]...)
	data = append(data, context...)
	hash := blake2b.Sum256(data)
	return hash
}

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
	result := make([]byte, 0, len(nonce)+len(ciphertext))
	result = append(result, nonce...)
	result = append(result, ciphertext...)
	return result, nil
}

func (n *NoiseHandshakeState) decryptHandshakeMessage(data []byte, key [32]byte) ([]byte, error) {
	if len(data) < chacha20poly1305.NonceSize+chacha20poly1305.Overhead {
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

func (n *NoiseHandshakeState) deriveSessionKeys() error {
	var combined [4][32]byte
	combined[0] = n.calculateDH(n.LocalStatic, n.RemoteStatic)
	combined[1] = n.calculateDH(n.LocalEphemeral, n.RemoteStatic)
	combined[2] = n.calculateDH(n.LocalStatic, n.RemoteEphemeral)
	combined[3] = n.calculateDH(n.LocalEphemeral, n.RemoteEphemeral)
	seed := make([]byte, 0, 128)
	for i := 0; i < 4; i++ {
		seed = append(seed, combined[i][:]...)
	}
	sessionKeyMaterial := blake2b.Sum512(seed)
	copy(n.SessionKeyLocalToRemote[:], sessionKeyMaterial[:32])
	copy(n.SessionKeyRemoteToLocal[:], sessionKeyMaterial[32:64])
	n.SessionNonceLocal = 0
	n.SessionNonceRemote = 0
	return nil
}

func (n *NoiseHandshakeState) EncryptSessionMessage(plaintext []byte) ([]byte, error) {
	if !n.Complete {
		return nil, ErrInvalidHandshakeState
	}
	n.SessionNonceLocal++
	nonce := make([]byte, 12)
	binary.LittleEndian.PutUint64(nonce[4:], n.SessionNonceLocal)
	aead, err := chacha20poly1305.New(n.SessionKeyLocalToRemote[:])
	if err != nil {
		return nil, err
	}
	ciphertext := aead.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nil
}

func (n *NoiseHandshakeState) DecryptSessionMessage(ciphertext []byte) ([]byte, error) {
	if !n.Complete {
		return nil, ErrInvalidHandshakeState
	}
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

func (n *NoiseHandshakeState) IsComplete() bool {
	return n.Complete
}

func (n *NoiseHandshakeState) GetSessionKeys() (localToRemote, remoteToLocal [32]byte) {
	return n.SessionKeyLocalToRemote, n.SessionKeyRemoteToLocal
}

func (n *NoiseHandshakeState) ResetNonces() {
	n.SessionNonceLocal = 0
	n.SessionNonceRemote = 0
}
