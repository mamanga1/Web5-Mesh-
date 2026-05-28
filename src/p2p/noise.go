package p2p

import (
    "crypto/rand"
    "log"
    "net"
    
    "github.com/flynn/noise"
    "golang.org/x/crypto/blake2b"
)

type NoiseSession struct {
    transport   *TransportUDP
    cipherSuite noise.CipherSuite
    localKey    noise.DHKey
    remoteKey   []byte
    handshake   *noise.HandshakeState
    sessionKey  [32]byte
    ready       bool
}

func NewNoiseSession(transport *TransportUDP, staticKey []byte) *NoiseSession {
    dhKey, _ := noise.DH25519.GenerateKeypair(rand.Reader)
    
    return &NoiseSession{
        transport:   transport,
        cipherSuite: noise.NewCipherSuite(noise.DH25519, noise.CipherChaChaPoly, noise.HashSHA256),
        localKey:    dhKey,
        ready:       false,
    }
}

func (n *NoiseSession) InitiateIK(addr *net.UDPAddr, remoteStatic []byte) error {
    config := noise.Config{
        CipherSuite:   n.cipherSuite,
        Pattern:       noise.HandshakeIK,
        Initiator:     true,
        StaticKeypair: n.localKey,
        PeerStatic:    remoteStatic,
    }
    
    hs, err := noise.NewHandshakeState(config)
    if err != nil {
        return err
    }
    n.handshake = hs
    
    msg, _, _, err := hs.WriteMessage(nil, nil)
    if err != nil {
        return err
    }
    
    if err := n.transport.WriteTo(msg, addr); err != nil {
        return err
    }
    
    resp, _, err := n.transport.ReadFrom()
    if err != nil {
        return err
    }
    
    _, _, _, err = hs.ReadMessage(nil, resp)
    if err != nil {
        return err
    }
    
    n.sessionKey = blake2b.Sum256(hs.ChannelBinding())
    n.ready = true
    n.transport.SetSessionKey(n.sessionKey)
    
    log.Printf("[NOISE] Handshake completed with %s (PFS active)", addr.String())
    return nil
}

func (n *NoiseSession) RespondIK(addr *net.UDPAddr, data []byte, remoteStatic []byte) error {
    config := noise.Config{
        CipherSuite:   n.cipherSuite,
        Pattern:       noise.HandshakeIK,
        Initiator:     false,
        StaticKeypair: n.localKey,
        PeerStatic:    remoteStatic,
    }
    
    hs, err := noise.NewHandshakeState(config)
    if err != nil {
        return err
    }
    n.handshake = hs
    
    _, _, _, err = hs.ReadMessage(nil, data)
    if err != nil {
        return err
    }
    
    msg, _, _, err := hs.WriteMessage(nil, nil)
    if err != nil {
        return err
    }
    
    if err := n.transport.WriteTo(msg, addr); err != nil {
        return err
    }
    
    n.sessionKey = blake2b.Sum256(hs.ChannelBinding())
    n.ready = true
    n.transport.SetSessionKey(n.sessionKey)
    
    log.Printf("[NOISE] Handshake responded to %s (PFS active)", addr.String())
    return nil
}

func (n *NoiseSession) IsReady() bool {
    return n.ready
}

func (n *NoiseSession) GetSessionKey() [32]byte {
    return n.sessionKey
}
