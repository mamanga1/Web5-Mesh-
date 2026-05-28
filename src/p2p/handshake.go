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
    acl       *ACL
}

func NewHandshake(transport *TransportUDP, identity *crypto.Identity) *Handshake {
    acl := NewACL()
    if identity != nil {
        acl.AddAuthorizedKey(identity.PublicKey)
    }
    return &Handshake{
        transport: transport,
        identity:  identity,
        acl:       acl,
    }
}

func (h *Handshake) Initiate(addr *net.UDPAddr) error {
    var ephemeralKey [32]byte
    rand.Read(ephemeralKey[:])
    
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
        sharedKey := blake2b.Sum256(ephemeralKey[:])
        h.transport.SetSessionKey(sharedKey)
        log.Printf("[HANDSHAKE] Session encrypted with %s", addr.String())
    }
    
    return nil
}

func (h *Handshake) Respond(addr *net.UDPAddr, data []byte) error {
    if len(data) < 4 || string(data[:4]) != "HELLO" {
        return nil
    }
    
    if len(data) > 36 {
        remotePub := data[4:36]
        if !h.acl.CheckHandshake(remotePub) {
            log.Printf("[HANDSHAKE] Rejected unauthorized peer from %s", addr.String())
            return nil
        }
        
        var ephemeralKey [32]byte
        copy(ephemeralKey[:], data[len(data)-32:])
        sharedKey := blake2b.Sum256(ephemeralKey[:])
        h.transport.SetSessionKey(sharedKey)
        
        respMsg := append([]byte("HELLO"), h.identity.PublicKey...)
        respMsg = append(respMsg, ephemeralKey[:]...)
        h.transport.WriteTo(respMsg, addr)
        
        log.Printf("[HANDSHAKE] Session encrypted with authorized peer %s", addr.String())
    }
    
    return nil
}

func (h *Handshake) AddAuthorizedKey(pubKey []byte) {
    h.acl.AddAuthorizedKey(pubKey)
}
