package p2p

import (
    "crypto/rand"
    "log"
    "net"
    
    "github.com/mamanga1/web5-mesh/src/crypto"
    "golang.org/x/crypto/blake2b"
)

type Handshake struct {
    transport *TransportUDP
    identity  *crypto.Identity
}

func NewHandshake(transport *TransportUDP, identity *crypto.Identity) *Handshake {
    return &Handshake{
        transport: transport,
        identity:  identity,
    }
}

// Initiate inicia handshake como emisor
func (h *Handshake) Initiate(addr *net.UDPAddr) error {
    // Generar clave efímera
    var ephemeralKey [32]byte
    rand.Read(ephemeralKey[:])
    
    // Enviar HELLO con clave pública
    pubKey := h.identity.PublicKey
    helloMsg := append([]byte("HELLO"), pubKey...)
    helloMsg = append(helloMsg, ephemeralKey[:]...)
    
    if err := h.transport.WriteTo(helloMsg, addr); err != nil {
        return err
    }
    
    // Esperar respuesta
    resp, _, err := h.transport.ReadFrom()
    if err != nil {
        return err
    }
    
    if len(resp) > 4 && string(resp[:4]) == "HELLO" {
        // Derivar clave compartida
        sharedKey := blake2b.Sum256(ephemeralKey[:])
        h.transport.SetSessionKey(sharedKey)
        log.Printf("[HANDSHAKE] Session encrypted with %s", addr.String())
    }
    
    return nil
}

// Respond responde a handshake como receptor
func (h *Handshake) Respond(addr *net.UDPAddr, data []byte) error {
    if len(data) < 4 || string(data[:4]) != "HELLO" {
        return nil
    }
    
    // Extraer clave efímera del mensaje
    if len(data) > 36 {
        var ephemeralKey [32]byte
        copy(ephemeralKey[:], data[len(data)-32:])
        sharedKey := blake2b.Sum256(ephemeralKey[:])
        h.transport.SetSessionKey(sharedKey)
        log.Printf("[HANDSHAKE] Session encrypted with %s", addr.String())
    }
    
    return nil
}
