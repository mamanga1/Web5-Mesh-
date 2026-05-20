// ============================================================================
// src/dht/memory_pool.go - Arena Allocation for GC Pressure Reduction
// ============================================================================
// Especificación:
// - Arena allocation para reducir presión del Garbage Collector
// - Usa arrays planos en lugar de punteros
// - Reduce drásticamente la sobrecarga de GC en nodos con poca RAM (TV boxes)
// ============================================================================

package dht

import (
	"sync"
	"sync/atomic"
	"time"
)

// NodeEntryFlat es una versión "plana" de NodeEntry para arena allocation
// Todos los campos son valores, no punteros, para evitar que el GC los escanee
type NodeEntryFlat struct {
	// IDs (arrays fijos, no slices)
	ID [32]byte // NodeID fijo de 32 bytes

	// Dirección (buffer fijo de 64 bytes para IP:Puerto)
	Address [64]byte

	// Timestamps como uint64 (evita punteros a time.Time)
	LastSeenUnix int64
	CreatedUnix  int64

	// Reputación y distancia
	Reputation uint64
	Distance   [32]byte // NodeID para distancia

	// Flags
	Verified   uint32 // 0=false, 1=true
	IsValid    uint32 // 0=false, 1=true
	Index      uint32 // Índice en el pool
	Generation uint32 // Para detectar referencias obsoletas
}

// SetAddress copia una dirección string al buffer fijo
func (n *NodeEntryFlat) SetAddress(addr string) {
	copy(n.Address[:], addr)
	if len(addr) < len(n.Address) {
		n.Address[len(addr)] = 0
	}
}

// GetAddress retorna la dirección como string
func (n *NodeEntryFlat) GetAddress() string {
	// Encontrar el final de la cadena (terminada en null)
	length := 0
	for i := 0; i < len(n.Address) && n.Address[i] != 0; i++ {
		length++
	}
	return string(n.Address[:length])
}

// SetID copia un NodeID al buffer fijo
func (n *NodeEntryFlat) SetID(id NodeID) {
	copy(n.ID[:], id[:])
}

// GetID retorna el NodeID como valor
func (n *NodeEntryFlat) GetID() NodeID {
	var id NodeID
	copy(id[:], n.ID[:])
	return id
}

// SetDistance copia la distancia XOR
func (n *NodeEntryFlat) SetDistance(dist NodeID) {
	copy(n.Distance[:], dist[:])
}

// GetDistance retorna la distancia XOR
func (n *NodeEntryFlat) GetDistance() NodeID {
	var dist NodeID
	copy(dist[:], n.Distance[:])
	return dist
}

// LastSeen retorna el timestamp como time.Time
func (n *NodeEntryFlat) LastSeen() time.Time {
	if n.LastSeenUnix == 0 {
		return time.Time{}
	}
	return time.Unix(0, n.LastSeenUnix)
}

// SetLastSeen actualiza el timestamp
func (n *NodeEntryFlat) SetLastSeen(t time.Time) {
	if !t.IsZero() {
		n.LastSeenUnix = t.UnixNano()
	} else {
		n.LastSeenUnix = time.Now().UnixNano()
	}
}

// Created retorna el timestamp de creación
func (n *NodeEntryFlat) Created() time.Time {
	if n.CreatedUnix == 0 {
		return time.Time{}
	}
	return time.Unix(0, n.CreatedUnix)
}

// SetCreated actualiza el timestamp de creación
func (n *NodeEntryFlat) SetCreated(t time.Time) {
	if !t.IsZero() {
		n.CreatedUnix = t.UnixNano()
	} else {
		n.CreatedUnix = time.Now().UnixNano()
	}
}

// MemoryPool es un pool de memoria preasignada para NodeEntryFlat
// Elimina la necesidad de asignar nuevos objetos en el heap durante el hot path
type MemoryPool struct {
	// Arena principal: array plano de NodeEntryFlat
	nodes []NodeEntryFlat

	// Mapa de índice por ID (ID -> index)
	indexMap map[[32]byte]uint32

	// Gestión de slots libres
	freeSlots []uint32
	freeCount uint32

	// Capacidad total y contadores
	capacity    uint32
	allocated   uint32
	maxAllocated uint32

	// Estadísticas
	hits   uint64
	misses uint64
	evicts uint64

	// Protección de concurrencia
	mu sync.RWMutex

	// Control de generación para detectar uso después de liberar
	generationCounter uint32
}

// NewMemoryPool crea un nuevo memory pool con la capacidad especificada
// capacity: número máximo de nodos que puede almacenar
func NewMemoryPool(capacity uint32) *MemoryPool {
	return &MemoryPool{
		nodes:       make([]NodeEntryFlat, capacity),
		indexMap:    make(map[[32]byte]uint32, capacity),
		freeSlots:   make([]uint32, 0, capacity),
		capacity:    capacity,
		freeCount:   capacity, // Inicialmente todos están libres
		allocated:   0,
		generationCounter: 1,
	}
}

// Add agrega un nodo al pool
// Retorna el índice del nodo en el pool (para referencia rápida)
func (p *MemoryPool) Add(id NodeID, address string, reputation uint64) uint32 {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Verificar si ya existe
	var idArray [32]byte
	copy(idArray[:], id[:])

	if idx, exists := p.indexMap[idArray]; exists {
		// Actualizar nodo existente
		node := &p.nodes[idx]
		node.SetAddress(address)
		node.Reputation = reputation
		node.SetLastSeen(time.Now())
		node.Verified = 1
		node.Generation++
		p.hits++
		return idx
	}

	// Buscar slot libre
	var idx uint32
	var found bool

	if p.freeCount > 0 {
		// Reutilizar slot libre
		if len(p.freeSlots) > 0 {
			idx = p.freeSlots[len(p.freeSlots)-1]
			p.freeSlots = p.freeSlots[:len(p.freeSlots)-1]
			found = true
		} else {
			// Escanear en busca de slot libre
			for i := uint32(0); i < p.capacity; i++ {
				if p.nodes[i].IsValid == 0 {
					idx = i
					found = true
					break
				}
			}
		}
	}

	if !found {
		// Pool lleno, desalojar el nodo más viejo
		idx = p.evictOldestLocked()
		p.evicts++
	}

	// Inicializar el slot
	node := &p.nodes[idx]
	node.SetID(id)
	node.SetAddress(address)
	node.Reputation = reputation
	node.SetLastSeen(time.Now())
	node.SetCreated(time.Now())
	node.Verified = 1
	node.IsValid = 1
	node.Index = idx
	node.Generation = p.generationCounter
	p.generationCounter++

	p.indexMap[idArray] = idx

	if p.freeCount > 0 {
		p.freeCount--
	}
	p.allocated++

	if p.allocated > p.maxAllocated {
		p.maxAllocated = p.allocated
	}

	p.misses++

	return idx
}

// evictOldestLocked encuentra y marca el nodo más antiguo para desalojo
// NOTA: debe llamarse con el lock adquirido
func (p *MemoryPool) evictOldestLocked() uint32 {
	var oldestIdx uint32 = 0
	var oldestTime int64 = 0

	for i := uint32(0); i < p.capacity; i++ {
		if p.nodes[i].IsValid == 0 {
			continue
		}
		if oldestTime == 0 || p.nodes[i].LastSeenUnix < oldestTime {
			oldestTime = p.nodes[i].LastSeenUnix
			oldestIdx = i
		}
	}

	// Remover del índice
	var idArray [32]byte
	copy(idArray[:], p.nodes[oldestIdx].ID[:])
	delete(p.indexMap, idArray)

	p.nodes[oldestIdx].IsValid = 0
	p.nodes[oldestIdx].Generation++

	return oldestIdx
}

// Get recupera un nodo por ID
// Retorna (NodeEntryFlat, true) si existe, (NodeEntryFlat{}, false) si no
func (p *MemoryPool) Get(id NodeID) (NodeEntryFlat, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var idArray [32]byte
	copy(idArray[:], id[:])

	idx, exists := p.indexMap[idArray]
	if !exists {
		return NodeEntryFlat{}, false
	}

	node := p.nodes[idx]
	if node.IsValid == 0 {
		return NodeEntryFlat{}, false
	}

	// Actualizar timestamp (sin lock de escritura, usamos aproximación)
	// En producción, esto podría ser atómico
	node.SetLastSeen(time.Now())

	return node, true
}

// GetByIndex recupera un nodo por su índice (más rápido que Get)
// Útil cuando ya tenemos el índice de una operación anterior
func (p *MemoryPool) GetByIndex(idx uint32) (NodeEntryFlat, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if idx >= p.capacity {
		return NodeEntryFlat{}, false
	}

	node := p.nodes[idx]
	if node.IsValid == 0 {
		return NodeEntryFlat{}, false
	}

	return node, true
}

// Remove elimina un nodo del pool
func (p *MemoryPool) Remove(id NodeID) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	var idArray [32]byte
	copy(idArray[:], id[:])

	idx, exists := p.indexMap[idArray]
	if !exists {
		return false
	}

	// Marcar como inválido
	p.nodes[idx].IsValid = 0
	p.nodes[idx].Generation++
	delete(p.indexMap, idArray)

	// Agregar a freeSlots para reutilización
	p.freeSlots = append(p.freeSlots, idx)
	p.freeCount++

	if p.allocated > 0 {
		p.allocated--
	}

	return true
}

// UpdateActualiza la información de un nodo existente
func (p *MemoryPool) Update(id NodeID, address string, reputation uint64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	var idArray [32]byte
	copy(idArray[:], id[:])

	idx, exists := p.indexMap[idArray]
	if !exists {
		return false
	}

	node := &p.nodes[idx]
	if node.IsValid == 0 {
		return false
	}

	if address != "" {
		node.SetAddress(address)
	}
	if reputation > 0 {
		node.Reputation = reputation
	}
	node.SetLastSeen(time.Now())
	node.Generation++

	return true
}

// Contains verifica si un ID existe en el pool
func (p *MemoryPool) Contains(id NodeID) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var idArray [32]byte
	copy(idArray[:], id[:])
	_, exists := p.indexMap[idArray]
	return exists
}

// Len retorna la cantidad de nodos actualmente en el pool
func (p *MemoryPool) Len() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return int(p.allocated)
}

// Cap retorna la capacidad máxima del pool
func (p *MemoryPool) Cap() uint32 {
	return p.capacity
}

// Free retorna la cantidad de slots libres
func (p *MemoryPool) Free() uint32 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.freeCount
}

// GetAllIDs retorna todos los NodeIDs actualmente en el pool
func (p *MemoryPool) GetAllIDs() []NodeID {
	p.mu.RLock()
	defer p.mu.RUnlock()

	ids := make([]NodeID, 0, p.allocated)
	for idArray := range p.indexMap {
		var id NodeID
		copy(id[:], idArray[:])
		ids = append(ids, id)
	}
	return ids
}

// Iterate itera sobre todos los nodos válidos
// La función callback recibe el índice y el nodo
// Si la función retorna false, se detiene la iteración
func (p *MemoryPool) Iterate(fn func(idx uint32, node NodeEntryFlat) bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for i := uint32(0); i < p.capacity; i++ {
		if p.nodes[i].IsValid == 0 {
			continue
		}
		if !fn(i, p.nodes[i]) {
			break
		}
	}
}

// Stats retorna estadísticas del pool
func (p *MemoryPool) Stats() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return map[string]interface{}{
		"capacity":       p.capacity,
		"allocated":      p.allocated,
		"free":           p.freeCount,
		"max_allocated":  p.maxAllocated,
		"hits":           p.hits,
		"misses":         p.misses,
		"evicts":         p.evicts,
		"hit_ratio":      float64(p.hits) / float64(p.hits+p.misses+1),
		"generation":     p.generationCounter,
	}
}

// Reset limpia el pool completo
func (p *MemoryPool) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.nodes = make([]NodeEntryFlat, p.capacity)
	p.indexMap = make(map[[32]byte]uint32, p.capacity)
	p.freeSlots = make([]uint32, 0, p.capacity)
	p.freeCount = p.capacity
	p.allocated = 0
	p.maxAllocated = 0
	p.hits = 0
	p.misses = 0
	p.evicts = 0
	p.generationCounter = 1
}

// ToNodeEntry convierte NodeEntryFlat a NodeEntry (para compatibilidad)
// NOTA: Esto crea un nuevo objeto en el heap, solo usar cuando sea necesario
func (n *NodeEntryFlat) ToNodeEntry() *NodeEntry {
	id := n.GetID()
	return &NodeEntry{
		ID:         id,
		Address:    n.GetAddress(),
		LastSeen:   n.LastSeen(),
		Reputation: n.Reputation,
		Distance:   n.GetDistance(),
		Verified:   n.Verified == 1,
	}
}

// NodeEntryFlatFromNodeEntry convierte NodeEntry a NodeEntryFlat
func NodeEntryFlatFromNodeEntry(node *NodeEntry) NodeEntryFlat {
	var flat NodeEntryFlat
	flat.SetID(node.ID)
	flat.SetAddress(node.Address)
	flat.Reputation = node.Reputation
	flat.SetLastSeen(node.LastSeen)
	flat.SetDistance(node.Distance)
	if node.Verified {
		flat.Verified = 1
	}
	flat.IsValid = 1
	return flat
}

// UpdateNodeAge actualiza la antigüedad de todos los nodos (llamada periódica)
func (p *MemoryPool) UpdateNodeAge() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now().UnixNano()
	for i := uint32(0); i < p.capacity; i++ {
		if p.nodes[i].IsValid == 0 {
			continue
		}
		// Solo actualizar si ha pasado más de 1 minuto
		if now-p.nodes[i].LastSeenUnix > 60_000_000_000 {
			// Marcar como potencialmente muerto (dejar que el ping lo confirme)
			// No removemos inmediatamente
		}
	}
}

// Atomic increment para contadores (sin lock)
func (p *MemoryPool) incrementHits() {
	atomic.AddUint64(&p.hits, 1)
}

func (p *MemoryPool) incrementMisses() {
	atomic.AddUint64(&p.misses, 1)
}
