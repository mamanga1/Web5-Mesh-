package p2p

import (
    "crypto/ed25519"
    "encoding/hex"
    "log"
    "sync"
)

type ACL struct {
    whitelist map[string]bool
    mu        sync.RWMutex
}

func NewACL() *ACL {
    return &ACL{
        whitelist: make(map[string]bool),
    }
}

// AddAuthorizedKey agrega una clave pública autorizada (hex string)
func (a *ACL) AddAuthorizedKey(pubKey ed25519.PublicKey) {
    a.mu.Lock()
    defer a.mu.Unlock()
    keyStr := hex.EncodeToString(pubKey)
    a.whitelist[keyStr] = true
    log.Printf("[ACL] Authorized key added: %x", pubKey[:8])
}

// IsAuthorized verifica si una clave está autorizada
func (a *ACL) IsAuthorized(pubKey ed25519.PublicKey) bool {
    a.mu.RLock()
    defer a.mu.RUnlock()
    keyStr := hex.EncodeToString(pubKey)
    return a.whitelist[keyStr]
}

// CheckHandshake valida que el peer esté autorizado durante el handshake
func (a *ACL) CheckHandshake(remotePub ed25519.PublicKey) bool {
    if !a.IsAuthorized(remotePub) {
        log.Printf("[ACL] Rejected unauthorized peer: %x", remotePub[:8])
        return false
    }
    log.Printf("[ACL] Authorized peer: %x", remotePub[:8])
    return true
}
