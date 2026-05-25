package p2p

import (
	"log"
	"sync"
	"time"
)

// Telemetry registra estadísticas del DHT
type Telemetry struct {
	mu               sync.RWMutex
	pingSent         int64
	pingReceived     int64
	pongSent         int64
	pongReceived     int64
	findNodeSent     int64
	findNodeReceived int64
	nodesDiscovered  int64
	lastLogTime      time.Time
}

var telemetry = &Telemetry{
	lastLogTime: time.Now(),
}

// IncPingSent incrementa contador de PING enviados
func (t *Telemetry) IncPingSent() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pingSent++
}

// IncPingReceived incrementa contador de PING recibidos
func (t *Telemetry) IncPingReceived() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pingReceived++
}

// IncPongSent incrementa contador de PONG enviados
func (t *Telemetry) IncPongSent() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pongSent++
}

// IncPongReceived incrementa contador de PONG recibidos
func (t *Telemetry) IncPongReceived() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pongReceived++
}

// IncFindNodeSent incrementa contador de FIND_NODE enviados
func (t *Telemetry) IncFindNodeSent() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.findNodeSent++
}

// IncFindNodeReceived incrementa contador de FIND_NODE recibidos
func (t *Telemetry) IncFindNodeReceived() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.findNodeReceived++
}

// IncNodesDiscovered incrementa contador de nodos descubiertos
func (t *Telemetry) IncNodesDiscovered() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nodesDiscovered++
}

// Log imprime estadísticas periódicas
func (t *Telemetry) Log() {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	if now.Sub(t.lastLogTime) < 30*time.Second {
		return
	}
	t.lastLogTime = now

	log.Printf("[TELEMETRY] PING: sent=%d recv=%d | PONG: sent=%d recv=%d | FIND_NODE: sent=%d recv=%d | Discovered: %d",
		t.pingSent, t.pingReceived,
		t.pongSent, t.pongReceived,
		t.findNodeSent, t.findNodeReceived,
		t.nodesDiscovered)
}
