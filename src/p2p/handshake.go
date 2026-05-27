package p2p

import (
    "crypto/rand"
    "log"
    "net"
    
    "github.com/mamanga1/web5-mesh/src/crypto"
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

func (h *Handshake) Initiate(addr *net.UDPAddr) error {
    var ephemeralKey [32]byte
    rand.Read(ephemeralKey[:])
    
    // Usar la clave pública directamente (Ed25519 son 32 bytes)
    pubKey := h.identity.PublicKey
    
    helloMsg := append([]byte("HELLO"), pubKey...)
    helloMsg = append(helloMsg, ephemeralKey[:]...)
    
    if err := h.transport.WriteTo(helloMsg, addr); err != nil {
        return err
    }
    
    resp, _, err := h.transport.ReadFrom()
    if err != nil {
        return err
    }
    
    if len(resp) > 4 && string(resp[:4]) == "HELLO" {
        log.Printf("[HANDSHAKE] Completed with %s", addr.String())
    }
    
    return nil
}

func (h *Handshake) Respond(addr *net.UDPAddr, data []byte) error {
    if len(data) < 4 || string(data[:4]) != "HELLO" {
        return nil
    }
    
    log.Printf("[HANDSHAKE] Received handshake from %s", addr.String())
    return nil
}
