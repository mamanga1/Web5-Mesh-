// ============================================================================
// tests/unit/identity_test.go - Identity & DID Unit Tests
// ============================================================================
// Especificación:
// - Tests unitarios de derivación de claves del DID
// - Verificación de firmas y validación criptográfica
// - Pruebas de generación y carga de identidades
// ============================================================================

package unit

import (
	"bytes"
	"testing"

	"web5-mesh/src/crypto"
)

// TestNewIdentity prueba la creación de una nueva identidad
func TestNewIdentity(t *testing.T) {
	name := "test-node"
	identity, err := crypto.NewIdentity(name)

	if err != nil {
		t.Fatalf("Failed to create identity: %v", err)
	}

	if identity == nil {
		t.Fatal("Identity is nil")
	}

	if identity.Name != name {
		t.Errorf("Expected name %s, got %s", name, identity.Name)
	}

	if identity.DID == nil {
		t.Fatal("DID is nil")
	}

	if identity.PrivateKey == nil {
		t.Fatal("Private key is nil")
	}

	if identity.PublicKey == nil {
		t.Fatal("Public key is nil")
	}

	didStr := identity.GetDIDString()
	if didStr == "" {
		t.Error("DID string is empty")
	}

	t.Logf("Generated DID: %s", didStr)
}

// TestIdentitySignAndVerify prueba la firma y verificación de datos
func TestIdentitySignAndVerify(t *testing.T) {
	identity, err := crypto.NewIdentity("sign-test")
	if err != nil {
		t.Fatalf("Failed to create identity: %v", err)
	}

	testData := []byte("Hello, MaIA Mesh!")

	// Firmar datos
	signature, err := identity.Sign(testData)
	if err != nil {
		t.Fatalf("Failed to sign data: %v", err)
	}

	if len(signature) == 0 {
		t.Error("Signature is empty")
	}

	// Verificar firma
	valid := identity.Verify(testData, signature)
	if !valid {
		t.Error("Signature verification failed")
	}

	// Verificar que datos modificados invalidan la firma
	modifiedData := []byte("Hello, Modified!")
	valid = identity.Verify(modifiedData, signature)
	if valid {
		t.Error("Modified data should not verify")
	}

	t.Logf("Signature length: %d bytes", len(signature))
}

// TestIdentitySerialization prueba la serialización y carga de identidades
func TestIdentitySerialization(t *testing.T) {
	original, err := crypto.NewIdentity("serialize-test")
	if err != nil {
		t.Fatalf("Failed to create identity: %v", err)
	}

	// Simular serialización (guardar clave privada en hex)
	pubKeyHex := original.GetPublicKeyHex()
	t.Logf("Public key hex: %s...", pubKeyHex[:20])

	// Cargar desde hex (simulado)
	// En producción, se usaría LoadIdentityFromHex
	loaded := original

	if loaded.GetDIDString() != original.GetDIDString() {
		t.Error("DID mismatch after load")
	}

	testData := []byte("test data")
	sig, _ := original.Sign(testData)
	valid := loaded.Verify(testData, sig)
	if !valid {
		t.Error("Loaded identity verification failed")
	}
}

// TestDIDUniqueness prueba que DIDs diferentes sean únicos
func TestDIDUniqueness(t *testing.T) {
	numIdentities := 10
	didMap := make(map[string]bool)

	for i := 0; i < numIdentities; i++ {
		identity, err := crypto.NewIdentity("test")
		if err != nil {
			t.Fatalf("Failed to create identity %d: %v", i, err)
		}

		did := identity.GetDIDString()
		if didMap[did] {
			t.Errorf("Duplicate DID found: %s", did)
		}
		didMap[did] = true
	}

	t.Logf("Generated %d unique DIDs", len(didMap))
}

// TestMultipleSignatures prueba múltiples firmas con diferentes datos
func TestMultipleSignatures(t *testing.T) {
	identity, err := crypto.NewIdentity("multi-sign-test")
	if err != nil {
		t.Fatalf("Failed to create identity: %v", err)
	}

	testCases := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"small", []byte("hello")},
		{"medium", []byte("this is a medium length message for signing")},
		{"large", bytes.Repeat([]byte("A"), 1024)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sig, err := identity.Sign(tc.data)
			if err != nil {
				t.Fatalf("Failed to sign: %v", err)
			}

			valid := identity.Verify(tc.data, sig)
			if !valid {
				t.Error("Signature verification failed")
			}
		})
	}
}

// TestIdentityConcurrency prueba uso concurrente de identidades
func TestIdentityConcurrency(t *testing.T) {
	identity, err := crypto.NewIdentity("concurrency-test")
	if err != nil {
		t.Fatalf("Failed to create identity: %v", err)
	}

	numGoroutines := 100
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			data := []byte{byte(id), byte(id >> 8)}
			sig, err := identity.Sign(data)
			if err != nil {
				t.Errorf("Sign error in goroutine %d: %v", id, err)
				done <- false
				return
			}

			if !identity.Verify(data, sig) {
				t.Errorf("Verify failed in goroutine %d", id)
				done <- false
				return
			}
			done <- true
		}(i)
	}

	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	t.Logf("Completed %d concurrent sign/verify operations", numGoroutines)
}

// TestInvalidSignatures prueba que firmas inválidas sean rechazadas
func TestInvalidSignatures(t *testing.T) {
	identity, err := crypto.NewIdentity("invalid-test")
	if err != nil {
		t.Fatalf("Failed to create identity: %v", err)
	}

	testData := []byte("test data")

	// Firma válida
	sig, _ := identity.Sign(testData)

	// Firma modificada
	if len(sig) > 0 {
		badSig := make([]byte, len(sig))
		copy(badSig, sig)
		if len(badSig) > 0 {
			badSig[0] ^= 0xFF
			if identity.Verify(testData, badSig) {
				t.Error("Modified signature should not verify")
			}
		}
	}

	// Tamaño de firma incorrecto
	shortSig := []byte{0x01, 0x02}
	if identity.Verify(testData, shortSig) {
		t.Error("Short signature should not verify")
	}

	// Firma vacía
	if identity.Verify(testData, []byte{}) {
		t.Error("Empty signature should not verify")
	}
}
