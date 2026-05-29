package p2p

import (
    "crypto/sha256"
    "encoding/binary"
    "log"
    "sync"
    "time"
)

// PoWManager maneja la dificultad dinámica por IP
type PoWManager struct {
    mu          sync.RWMutex
    ipLoad      map[string]*IPLoad
    baseDifficulty int
}

type IPLoad struct {
    NodeCount     int
    LastSeen      time.Time
    CurrentDifficulty int
}

// NewPoWManager crea un nuevo gestor de Proof of Work
func NewPoWManager() *PoWManager {
    return &PoWManager{
        ipLoad:         make(map[string]*IPLoad),
        baseDifficulty: 0, // Por defecto, sin PoW
    }
}

// GetDifficulty calcula la dificultad dinámica según la carga de la IP
func (p *PoWManager) GetDifficulty(ip string, currentNodes int) int {
    p.mu.Lock()
    defer p.mu.Unlock()
    
    load, exists := p.ipLoad[ip]
    if !exists {
        load = &IPLoad{CurrentDifficulty: p.baseDifficulty}
        p.ipLoad[ip] = load
    }
    
    load.LastSeen = time.Now()
    load.NodeCount = currentNodes
    
    // Reglas de aduana (directivas de trinchera)
    switch {
    case currentNodes <= 2:
        load.CurrentDifficulty = 0  // Gratis (usuarios comunes)
        log.Printf("[GAS] 🔓 IP %s: %d nodos -> dificultad 0 (libre)", ip, currentNodes)
    case currentNodes <= 5:
        load.CurrentDifficulty = 4  // Peaje leve (pymes)
        log.Printf("[GAS] ⚠️ IP %s: %d nodos -> dificultad 4 (peaje bajo)", ip, currentNodes)
    case currentNodes <= 10:
        load.CurrentDifficulty = 12 // Costo medio (corporaciones chicas)
        log.Printf("[GAS] 🚨 IP %s: %d nodos -> dificultad 12 (peaje medio)", ip, currentNodes)
    default:
        load.CurrentDifficulty = 20 // Ataque o corporación grande (quema CPU)
        log.Printf("[GAS] 🔥 IP %s: %d nodos -> dificultad 20 (MODO COMBATE)", ip, currentNodes)
    }
    
    return load.CurrentDifficulty
}

// VerifyPoW verifica si el nonce resuelve el desafío
func VerifyPoW(challenge []byte, nonce uint64, difficulty int) bool {
    if difficulty <= 0 {
        return true // Sin peaje
    }
    
    // Hash del desafío + nonce
    data := make([]byte, len(challenge)+8)
    copy(data, challenge)
    binary.BigEndian.PutUint64(data[len(challenge):], nonce)
    
    hash := sha256.Sum256(data)
    
    // Verificar que los primeros 'difficulty' bits sean cero
    // Para difficulty=20, verificamos ~2.5 bytes
    bits := uint(difficulty)
    bytes := bits / 8
    remaining := bits % 8
    
    for i := uint(0); i < bytes; i++ {
        if hash[i] != 0 {
            return false
        }
    }
    
    if remaining > 0 {
        mask := byte(0xFF) << (8 - remaining)
        if (hash[bytes] & mask) != 0 {
            return false
        }
    }
    
    return true
}

// CleanupIPs elimina IPs inactivas (para no acumular memoria)
func (p *PoWManager) CleanupIPs(maxAge time.Duration) {
    p.mu.Lock()
    defer p.mu.Unlock()
    
    now := time.Now()
    for ip, load := range p.ipLoad {
        if now.Sub(load.LastSeen) > maxAge {
            delete(p.ipLoad, ip)
            log.Printf("[GAS] 🧹 IP %s eliminada del registro (inactiva)", ip)
        }
    }
}
