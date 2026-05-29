package p2p

import (
    "crypto/ed25519"
    "encoding/json"
    "log"
    "time"
)

// Role define la jerarquía del nodo en la red
type Role string

const (
    RoleCore  Role = "core"  // Núcleo soberano (Xeon, Faros autorizados)
    RoleRelay Role = "relay" // Periferia mula (corporaciones)
    RoleFree  Role = "free"  // Usuario común (hasta 2 nodos)
)

// Ticket estructura de membresía firmada por la Xeon
type Ticket struct {
    DID       string    `json:"did"`       // Identificador del nodo
    Role      Role      `json:"role"`      // core | relay | free
    ExpiresAt time.Time `json:"expires_at"` // Fecha de expiración
    MaxNodes  int       `json:"max_nodes"` // Cantidad máxima de nodos permitidos
}

// SignedTicket contiene el ticket + su firma Ed25519
type SignedTicket struct {
    Ticket    []byte `json:"ticket"`     // Ticket marshalled
    Signature []byte `json:"signature"`  // Firma de la Xeon (64 bytes)
}

// ValidateTicket valida la firma y la vigencia del ticket
func ValidateTicket(signed *SignedTicket, corePublicKey ed25519.PublicKey) (*Ticket, error) {
    // 1. Verificar firma Ed25519
    if !ed25519.Verify(corePublicKey, signed.Ticket, signed.Signature) {
        log.Printf("[AUTH] ⚠️ Firma inválida para ticket (posible adulteración)")
        return nil, ErrInvalidSignature
    }

    // 2. Deserializar ticket
    var ticket Ticket
    if err := json.Unmarshal(signed.Ticket, &ticket); err != nil {
        log.Printf("[AUTH] ❌ Error parseando ticket: %v", err)
        return nil, err
    }

    // 3. Verificar expiración
    if time.Now().After(ticket.ExpiresAt) {
        log.Printf("[AUTH] ⏰ Ticket expirado para DID: %s (expiró en %s)", 
            ticket.DID, ticket.ExpiresAt.Format(time.RFC3339))
        return nil, ErrExpiredTicket
    }

    log.Printf("[AUTH] ✅ Ticket válido | DID: %s | Rol: %s | MaxNodes: %d", 
        ticket.DID, ticket.Role, ticket.MaxNodes)
    
    return &ticket, nil
}

// GetRoleForConnection determina el rol según el ticket o la IP
func GetRoleForConnection(remoteIP string, remoteDID string, ticket *Ticket) Role {
    if ticket != nil {
        return ticket.Role
    }
    
    // Lógica por defecto sin ticket (usuario común, gratis)
    log.Printf("[AUTH] 📡 Conexión sin ticket desde IP: %s | Asignando rol FREE", remoteIP)
    return RoleFree
}

// Errores comunes
var (
    ErrInvalidSignature = &AuthError{Msg: "firma criptográfica inválida"}
    ErrExpiredTicket    = &AuthError{Msg: "ticket expirado, renovar en 4sk.uk"}
)

type AuthError struct {
    Msg string
}

func (e *AuthError) Error() string {
    return e.Msg
}
