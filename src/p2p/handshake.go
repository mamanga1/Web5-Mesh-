package p2p

import (
    "log"
    "net"
    
    "github.com/mamanga1/web5-mesh/src/crypto"
)

type Handshake struct {
    transport *TransportUDP
    identity  *crypto.Identity
    acl       *ACL
    noise     *NoiseSession
}

func NewHandshake(transport *TransportUDP, identity *crypto.Identity) *Handshake {
    acl := NewACL()
    if identity != nil {
        acl.AddAuthorizedKey(identity.PublicKey)
    }
    
    noise := NewNoiseSession(transport, identity.PrivateKey)
    
    return &Handshake{
        transport: transport,
        identity:  identity,
        acl:       acl,
        noise:     noise,
    }
}

func (h *Handshake) Initiate(addr *net.UDPAddr) error {
    return h.noise.InitiateIK(addr, nil)
}

func (h *Handshake) Respond(addr *net.UDPAddr, data []byte) error {
    if len(data) > 36 && h.identity != nil {
        remotePub := data[4:36]
        if !h.acl.CheckHandshake(remotePub) {
            log.Printf("[HANDSHAKE] Rejected unauthorized peer from %s", addr.String())
            return nil
        }
    }
    
    return h.noise.RespondIK(addr, data, nil)
}

func (h *Handshake) AddAuthorizedKey(pubKey []byte) {
    h.acl.AddAuthorizedKey(pubKey)
}

func (h *Handshake) IsSessionReady() bool {
    return h.noise.IsReady()
}
