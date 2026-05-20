// ============================================================================
// src/reputation/score.go - Distributed Reputation System
// ============================================================================
// Especificación:
// - Sistema de reputación distribuido
// - Algoritmo de cálculo de confianza de nodos (escala 1-1000)
// - Decaimiento por inactividad y penalización por mal comportamiento
// - Sincronización de reputación entre nodos
// ============================================================================

package reputation

import (
	"sync"
	"time"
)

// NodeReputation representa la reputación de un nodo
type NodeReputation struct {
	// Identificación
	NodeID      string    `json:"node_id"`
	DID         string    `json:"did"`

	// Puntajes
	Score       uint64    `json:"score"`        // 1-1000
	BaseScore   uint64    `json:"base_score"`   // Puntaje base inicial
	RelayScore  uint64    `json:"relay_score"`  // Contribución como relay
	StorageScore uint64   `json:"storage_score"` // Contribución de almacenamiento

	// Estadísticas
	UptimeSeconds   uint64    `json:"uptime_seconds"`    // Tiempo total activo
	LastSeen        time.Time `json:"last_seen"`         // Última vez visto
	FirstSeen       time.Time `json:"first_seen"`        // Primera vez visto

	// Contadores de comportamiento
	SuccessCount    uint64    `json:"success_count"`     // Operaciones exitosas
	FailureCount    uint64    `json:"failure_count"`     // Operaciones fallidas
	RelayCount      uint64    `json:"relay_count"`       // Paquetes reenviados
	StorageCount    uint64    `json:"storage_count"`     // Almacenamiento contribuido

	// Penalizaciones
	LastPenalty     time.Time `json:"last_penalty"`      // Última penalización
	PenaltyCount    uint64    `json:"penalty_count"`     // Cantidad de penalizaciones

	// Estado
	IsActive        bool      `json:"is_active"`
	IsBlacklisted   bool      `json:"is_blacklisted"`
	BlacklistReason string    `json:"blacklist_reason,omitempty"`

	mu sync.RWMutex
}

// GetScore retorna el puntaje actual (thread-safe)
func (nr *NodeReputation) GetScore() uint64 {
	nr.mu.RLock()
	defer nr.mu.RUnlock()
	return nr.Score
}

// UpdateScore actualiza el puntaje
func (nr *NodeReputation) UpdateScore(delta int64) {
	nr.mu.Lock()
	defer nr.mu.Unlock()

	newScore := int64(nr.Score) + delta
	if newScore < 1 {
		newScore = 1
	}
	if newScore > 1000 {
		newScore = 1000
	}
	nr.Score = uint64(newScore)
}

// RecordSuccess registra una operación exitosa
func (nr *NodeReputation) RecordSuccess() {
	nr.mu.Lock()
	defer nr.mu.Unlock()
	nr.SuccessCount++
	nr.LastSeen = time.Now()
}

// RecordFailure registra una operación fallida (penaliza)
func (nr *NodeReputation) RecordFailure() {
	nr.mu.Lock()
	defer nr.mu.Unlock()
	nr.FailureCount++
	nr.LastSeen = time.Now()
}

// RecordRelay registra un paquete reenviado
func (nr *NodeReputation) RecordRelay() {
	nr.mu.Lock()
	defer nr.mu.Unlock()
	nr.RelayCount++
}

// RecordStorage registra contribución de almacenamiento
func (nr *NodeReputation) RecordStorage() {
	nr.mu.Lock()
	defer nr.mu.Unlock()
	nr.StorageCount++
}

// ApplyPenalty aplica una penalización por mal comportamiento
func (nr *NodeReputation) ApplyPenalty(reason string, points uint64) {
	nr.mu.Lock()
	defer nr.mu.Unlock()

	nr.PenaltyCount++
	nr.LastPenalty = time.Now()

	// Reducir puntaje
	if nr.Score > points {
		nr.Score -= points
	} else {
		nr.Score = 1
	}

	// Si el puntaje cae muy bajo, marcar como inactivo
	if nr.Score < 100 {
		nr.IsActive = false
	}
}

// Blacklist marca el nodo como en lista negra
func (nr *NodeReputation) Blacklist(reason string) {
	nr.mu.Lock()
	defer nr.mu.Unlock()
	nr.IsBlacklisted = true
	nr.BlacklistReason = reason
	nr.IsActive = false
	nr.Score = 1
}

// ReputationSystem es el sistema de reputación principal
type ReputationSystem struct {
	// Almacenamiento
	reputations map[string]*NodeReputation // NodeID -> Reputation
	mu          sync.RWMutex

	// Configuración
	baseScore       uint64
	maxScore        uint64
	minScore        uint64
	decayRate       float64   // Decaimiento diario
	penaltyMultiplier uint64   // Multiplicador de penalización

	// Actualizaciones
	updateCh    chan ReputationUpdate
	stopCh      chan struct{}
	wg          sync.WaitGroup

	// Estadísticas
	stats struct {
		totalNodes      uint64
		activeNodes     uint64
		blacklistedNodes uint64
		averageScore    float64
		mu              sync.RWMutex
	}
}

// ReputationUpdate representa una actualización de reputación
type ReputationUpdate struct {
	NodeID      string
	Delta       int64
	Reason      string
	IsPenalty   bool
}

// NewReputationSystem crea un nuevo sistema de reputación
func NewReputationSystem() *ReputationSystem {
	return &ReputationSystem{
		reputations:      make(map[string]*NodeReputation),
		baseScore:        100,
		maxScore:         1000,
		minScore:         1,
		decayRate:        0.05, // 5% de decaimiento por día
		penaltyMultiplier: 2,
		updateCh:         make(chan ReputationUpdate, 1000),
		stopCh:           make(chan struct{}),
	}
}

// Start inicia el sistema de reputación
func (rs *ReputationSystem) Start() {
	rs.wg.Add(2)
	go rs.processUpdates()
	go rs.decayLoop()
}

// Stop detiene el sistema de reputación
func (rs *ReputationSystem) Stop() {
	close(rs.stopCh)
	rs.wg.Wait()
}

// RegisterNode registra un nuevo nodo en el sistema
func (rs *ReputationSystem) RegisterNode(nodeID, did string) *NodeReputation {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rep, exists := rs.reputations[nodeID]; exists {
		return rep
	}

	rep := &NodeReputation{
		NodeID:      nodeID,
		DID:         did,
		Score:       rs.baseScore,
		BaseScore:   rs.baseScore,
		FirstSeen:   time.Now(),
		LastSeen:    time.Now(),
		IsActive:    true,
		IsBlacklisted: false,
	}

	rs.reputations[nodeID] = rep
	rs.updateStatsLocked()
	return rep
}

// GetReputation retorna la reputación de un nodo
func (rs *ReputationSystem) GetReputation(nodeID string) (*NodeReputation, bool) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	rep, ok := rs.reputations[nodeID]
	return rep, ok
}

// GetScore retorna el puntaje de un nodo
func (rs *ReputationSystem) GetScore(nodeID string) uint64 {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	if rep, ok := rs.reputations[nodeID]; ok {
		return rep.GetScore()
	}
	return rs.baseScore
}

// UpdateScore actualiza el puntaje de un nodo
func (rs *ReputationSystem) UpdateScore(nodeID string, delta int64, reason string) {
	select {
	case rs.updateCh <- ReputationUpdate{
		NodeID:    nodeID,
		Delta:     delta,
		Reason:    reason,
		IsPenalty: false,
	}:
	default:
		// Canal lleno, procesar directamente
		rs.applyUpdate(nodeID, delta, false)
	}
}

// Penalize penaliza un nodo por mal comportamiento
func (rs *ReputationSystem) Penalize(nodeID string, reason string, points uint64) {
	rs.UpdateScore(nodeID, -int64(points), reason)
}

// RecordSuccess registra una operación exitosa
func (rs *ReputationSystem) RecordSuccess(nodeID string) {
	rs.UpdateScore(nodeID, 1, "success")
}

// RecordFailure registra una operación fallida
func (rs *ReputationSystem) RecordFailure(nodeID string) {
	rs.UpdateScore(nodeID, -5, "failure")
}

// RecordRelay registra un paquete reenviado (bonificación)
func (rs *ReputationSystem) RecordRelay(nodeID string) {
	rs.UpdateScore(nodeID, 2, "relay")
}

// BlacklistNode agrega un nodo a la lista negra
func (rs *ReputationSystem) BlacklistNode(nodeID string, reason string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rep, ok := rs.reputations[nodeID]; ok {
		rep.Blacklist(reason)
	} else {
		rep := &NodeReputation{
			NodeID:        nodeID,
			Score:         1,
			IsBlacklisted: true,
			BlacklistReason: reason,
			IsActive:      false,
		}
		rs.reputations[nodeID] = rep
	}
	rs.updateStatsLocked()
}

// IsBlacklisted verifica si un nodo está en lista negra
func (rs *ReputationSystem) IsBlacklisted(nodeID string) bool {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	if rep, ok := rs.reputations[nodeID]; ok {
		return rep.IsBlacklisted
	}
	return false
}

// IsRelayEligible verifica si un nodo puede ser relay (score > 500)
func (rs *ReputationSystem) IsRelayEligible(nodeID string) bool {
	return rs.GetScore(nodeID) >= 500
}

// processUpdates procesa las actualizaciones de reputación
func (rs *ReputationSystem) processUpdates() {
	defer rs.wg.Done()

	for {
		select {
		case <-rs.stopCh:
			return
		case update := <-rs.updateCh:
			rs.applyUpdate(update.NodeID, update.Delta, update.IsPenalty)
		}
	}
}

// applyUpdate aplica una actualización de reputación
func (rs *ReputationSystem) applyUpdate(nodeID string, delta int64, isPenalty bool) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	rep, exists := rs.reputations[nodeID]
	if !exists {
		rep = &NodeReputation{
			NodeID:    nodeID,
			Score:     rs.baseScore,
			BaseScore: rs.baseScore,
			FirstSeen: time.Now(),
			LastSeen:  time.Now(),
			IsActive:  true,
		}
		rs.reputations[nodeID] = rep
	}

	// Aplicar delta
	rep.UpdateScore(delta)

	// Registrar acción específica
	if isPenalty {
		rep.PenaltyCount++
		rep.LastPenalty = time.Now()
	} else if delta > 0 {
		rep.SuccessCount++
	} else if delta < 0 {
		rep.FailureCount++
	}

	rep.LastSeen = time.Now()
	rep.IsActive = rep.Score > 100 && !rep.IsBlacklisted

	rs.updateStatsLocked()
}

// decayLoop aplica decaimiento de reputación periódicamente
func (rs *ReputationSystem) decayLoop() {
	defer rs.wg.Done()

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-rs.stopCh:
			return
		case <-ticker.C:
			rs.applyDecay()
		}
	}
}

// applyDecay aplica decaimiento a todos los nodos inactivos
func (rs *ReputationSystem) applyDecay() {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	for _, rep := range rs.reputations {
		// Calcular días desde última actividad
		daysInactive := time.Since(rep.LastSeen).Hours() / 24
		if daysInactive > 0 {
			decay := uint64(float64(rep.Score) * rs.decayRate * daysInactive)
			if decay > 0 {
				if rep.Score > decay {
					rep.Score -= decay
				} else {
					rep.Score = rs.minScore
				}
				rep.IsActive = rep.Score > 100
			}
		}
	}
	rs.updateStatsLocked()
}

// updateStatsLocked actualiza las estadísticas globales (requiere lock)
func (rs *ReputationSystem) updateStatsLocked() {
	rs.stats.mu.Lock()
	defer rs.stats.mu.Unlock()

	var totalScore uint64
	rs.stats.totalNodes = 0
	rs.stats.activeNodes = 0
	rs.stats.blacklistedNodes = 0

	for _, rep := range rs.reputations {
		rs.stats.totalNodes++
		totalScore += rep.Score
		if rep.IsActive {
			rs.stats.activeNodes++
		}
		if rep.IsBlacklisted {
			rs.stats.blacklistedNodes++
		}
	}

	if rs.stats.totalNodes > 0 {
		rs.stats.averageScore = float64(totalScore) / float64(rs.stats.totalNodes)
	}
}

// Stats retorna estadísticas del sistema
func (rs *ReputationSystem) Stats() map[string]interface{} {
	rs.stats.mu.RLock()
	defer rs.stats.mu.RUnlock()

	rs.mu.RLock()
	totalNodes := len(rs.reputations)
	rs.mu.RUnlock()

	return map[string]interface{}{
		"total_nodes":       totalNodes,
		"active_nodes":      rs.stats.activeNodes,
		"blacklisted_nodes": rs.stats.blacklistedNodes,
		"average_score":     rs.stats.averageScore,
		"base_score":        rs.baseScore,
		"decay_rate":        rs.decayRate,
	}
}

// GetAllScores retorna todos los puntajes (para sincronización)
func (rs *ReputationSystem) GetAllScores() map[string]uint64 {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	scores := make(map[string]uint64, len(rs.reputations))
	for id, rep := range rs.reputations {
		scores[id] = rep.GetScore()
	}
	return scores
}

// GetTopRelays retorna los N mejores nodos relay
func (rs *ReputationSystem) GetTopRelays(limit int) []string {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	type relay struct {
		id    string
		score uint64
	}
	relays := make([]relay, 0, len(rs.reputations))

	for id, rep := range rs.reputations {
		if rep.RelayCount > 0 && rep.Score >= 500 {
			relays = append(relays, relay{id: id, score: rep.Score})
		}
	}

	// Ordenar por score descendente (bubble sort simplificado)
	for i := 0; i < len(relays)-1; i++ {
		for j := i + 1; j < len(relays); j++ {
			if relays[i].score < relays[j].score {
				relays[i], relays[j] = relays[j], relays[i]
			}
		}
	}

	if limit > len(relays) {
		limit = len(relays)
	}
	result := make([]string, limit)
	for i := 0; i < limit; i++ {
		result[i] = relays[i].id
	}
	return result
}
