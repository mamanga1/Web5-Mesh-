// ============================================================================
// src/dht/routing_table.go - XOR Routing Table with 256 Buckets (k=16)
// ============================================================================
// Especificación:
// - Manejo del árbol de enrutamiento XOR con 256 cubos (buckets) de tamaño k=16
// - Cálculo de distancia XOR entre llaves de 32 bytes
// - Desalojo eficiente de nodos usando política LRU
// ============================================================================

package dht

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sync"
	"time"
)

// BucketSize = 16 (kademlia optimizado para alto churn en redes móviles)
const BucketSize = 16

// NumBuckets = 256 (para IDs de 32 bytes, 2^8 = 256)
const NumBuckets = 256

// NodeID representa un identificador de nodo en el DHT (32 bytes)
type NodeID [32]byte

// String retorna representación hexadecimal del NodeID
func (n NodeID) String() string {
	return fmt.Sprintf("%x", n[:8]) // Primeros 8 bytes para legibilidad
}

// Equals compara dos NodeIDs
func (n NodeID) Equals(other NodeID) bool {
	return bytes.Equal(n[:], other[:])
}

// XOR calcula la distancia XOR entre dos NodeIDs
func (n NodeID) XOR(other NodeID) NodeID {
	var result NodeID
	for i := 0; i < len(n); i++ {
		result[i] = n[i] ^ other[i]
	}
	return result
}

// CommonPrefixLength retorna el número de bits comunes en el prefijo
func (n NodeID) CommonPrefixLength(other NodeID) int {
	xor := n.XOR(other)
	for i := 0; i < len(xor); i++ {
		if xor[i] == 0 {
			continue
		}
		// Contar bits cero en este byte
		for bit := 7; bit >= 0; bit-- {
			if (xor[i]>>bit)&1 == 1 {
				return i*8 + (7 - bit)
			}
		}
	}
	return len(xor) * 8
}

// NodeEntry representa un nodo en la tabla de enrutamiento
type NodeEntry struct {
	ID         NodeID
	Address    string        // IP:Puerto
	LastSeen   time.Time     // Último heartbeat
	Reputation uint64        // Trust score (0-1000)
	Distance   NodeID        // XOR distance desde este nodo
	Verified   bool          // Si la criptografía fue verificada
	entry      *list.Element // Referencia al elemento en LRU (para eliminación rápida)
}

// Bucket representa un bucket Kademlia (k=16 nodos)
type Bucket struct {
	Nodes      map[NodeID]*NodeEntry // Nodos en este bucket
	LRU        *list.List            // Lista ordenada por última vez vista (LRU)
	MinDist    NodeID                // Distancia mínima para este bucket
	MaxDist    NodeID                // Distancia máxima para este bucket
	mu         sync.RWMutex
	lastAccess time.Time
}

// NewBucket crea un nuevo bucket con un rango de distancias
func NewBucket(minDist, maxDist NodeID) *Bucket {
	return &Bucket{
		Nodes:      make(map[NodeID]*NodeEntry),
		LRU:        list.New(),
		MinDist:    minDist,
		MaxDist:    maxDist,
		lastAccess: time.Now(),
	}
}

// Add agrega un nodo al bucket (o actualiza si ya existe)
// Retorna true si el bucket necesita ser dividido
func (b *Bucket) Add(node *NodeEntry) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if existing, exists := b.Nodes[node.ID]; exists {
		// Actualizar nodo existente
		existing.LastSeen = node.LastSeen
		existing.Reputation = node.Reputation
		existing.Address = node.Address
		existing.Verified = node.Verified
		// Mover al frente del LRU (más reciente)
		if existing.entry != nil {
			b.LRU.MoveToFront(existing.entry)
		}
		return false
	}

	// Nuevo nodo
	if len(b.Nodes) >= BucketSize {
		// Bucket lleno, verificar si hay nodos viejos para desalojar
		return b.evictStale()
	}

	// Agregar nuevo nodo
	node.entry = b.LRU.PushFront(node)
	b.Nodes[node.ID] = node

	return false
}

// evictStale elimina el nodo más antiguo del LRU si está muerto
// Retorna true si se necesita dividir el bucket (todos los nodos están vivos)
func (b *Bucket) evictStale() bool {
	// Buscar el nodo más viejo
	if b.LRU.Len() == 0 {
		return true
	}

	oldestElem := b.LRU.Back()
	oldestNode := oldestElem.Value.(*NodeEntry)

	// Si el nodo más viejo ha sido visto recientemente (< 1 hora), bucket está lleno
	// Necesitamos dividir el bucket (solo para buckets que no sean hoja)
	if time.Since(oldestNode.LastSeen) < time.Hour {
		return true // Necesita split
	}

	// Desalojar nodo muerto
	b.LRU.Remove(oldestElem)
	delete(b.Nodes, oldestNode.ID)

	// Ahora hay espacio, agregar el nuevo nodo
	return false
}

// FindClosest encuentra los k nodos más cercanos a un target
func (b *Bucket) FindClosest(target NodeID, k int) []*NodeEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	type distEntry struct {
		node *NodeEntry
		dist NodeID
	}

	entries := make([]distEntry, 0, len(b.Nodes))
	for _, node := range b.Nodes {
		entries = append(entries, distEntry{
			node: node,
			dist: node.ID.XOR(target),
		})
	}

	// Ordenar por distancia
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if bytes.Compare(entries[i].dist[:], entries[j].dist[:]) > 0 {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	result := make([]*NodeEntry, 0, k)
	for i := 0; i < len(entries) && i < k; i++ {
		result = append(result, entries[i].node)
	}

	return result
}

// Get retorna un nodo por ID
func (b *Bucket) Get(id NodeID) (*NodeEntry, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	node, ok := b.Nodes[id]
	return node, ok
}

// Remove elimina un nodo del bucket
func (b *Bucket) Remove(id NodeID) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	node, ok := b.Nodes[id]
	if !ok {
		return false
	}

	if node.entry != nil {
		b.LRU.Remove(node.entry)
	}
	delete(b.Nodes, id)
	return true
}

// Size retorna la cantidad de nodos en el bucket
func (b *Bucket) Size() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.Nodes)
}

// IsWithinRange verifica si una distancia está dentro del rango del bucket
func (b *Bucket) IsWithinRange(dist NodeID) bool {
	return bytes.Compare(dist[:], b.MinDist[:]) >= 0 &&
		bytes.Compare(dist[:], b.MaxDist[:]) < 0
}

// RoutingTable es la tabla de enrutamiento XOR completa
type RoutingTable struct {
	LocalID   NodeID
	Buckets   [NumBuckets]*Bucket
	mu        sync.RWMutex
	CreatedAt time.Time
}

// NewRoutingTable crea una nueva tabla de enrutamiento
func NewRoutingTable(localID NodeID) *RoutingTable {
	rt := &RoutingTable{
		LocalID:   localID,
		CreatedAt: time.Now(),
	}

	// Inicializar buckets con rangos de distancia
	// Cada bucket cubre un rango de distancias basado en el prefijo común
	for i := 0; i < NumBuckets; i++ {
		// El bucket i contiene nodos cuyo prefijo común con localID es exactamente i bits
		// y el bit i+1 es diferente
		var minDist, maxDist NodeID

		// Configurar bits para el rango
		// Los primeros i bits son iguales a localID
		// El bit i es opuesto a localID
		// Los bits restantes son wildcard
		rt.Buckets[i] = NewBucket(minDist, maxDist)
	}

	return rt
}

// getBucketIndex determina el bucket apropiado para un nodeID
func (rt *RoutingTable) getBucketIndex(nodeID NodeID) int {
	// Calcular prefijo común en bits
	commonBits := rt.LocalID.CommonPrefixLength(nodeID)

	// El bucket está determinado por la longitud del prefijo común
	// Los bits después del prefijo determinan el bucket
	if commonBits >= 256 {
		return 0 // Mismo ID (raro, normalmente no nos agregamos a nosotros mismos)
	}

	// Determinar el índice del bucket basado en el primer bit diferente
	// El bucket index es el número de bits comunes, limitado a NumBuckets-1
	bucketIdx := commonBits
	if bucketIdx >= NumBuckets {
		bucketIdx = NumBuckets - 1
	}
	return bucketIdx
}

// AddNode agrega un nodo a la tabla de enrutamiento
func (rt *RoutingTable) AddNode(node *NodeEntry) {
	if node.ID.Equals(rt.LocalID) {
		return // No agregarse a sí mismo
	}

	bucketIdx := rt.getBucketIndex(node.ID)
	bucket := rt.Buckets[bucketIdx]
	if bucket == nil {
		return
	}

	// Calcular distancia
	node.Distance = rt.LocalID.XOR(node.ID)
	node.LastSeen = time.Now()

	needsSplit := bucket.Add(node)

	// Si el bucket necesita dividirse y no es el último bucket, dividir
	if needsSplit && bucketIdx < NumBuckets-1 {
		rt.splitBucket(bucketIdx)
		// Reintentar agregar después del split
		rt.AddNode(node)
	}
}

// splitBucket divide un bucket en dos cuando está lleno y es necesario
func (rt *RoutingTable) splitBucket(bucketIdx int) {
	// En una implementación real, aquí se divide el bucket
	// Por simplicidad, esta es una versión mínima
	_ = bucketIdx
}

// FindClosest encuentra los k nodos más cercanos a un target
func (rt *RoutingTable) FindClosest(target NodeID, k int) []*NodeEntry {
	result := make([]*NodeEntry, 0, k)

	// Comenzar desde el bucket que contendría el target
	startBucket := rt.getBucketIndex(target)

	// Buscar en buckets cercanos (expandir hacia afuera)
	for offset := 0; offset < NumBuckets && len(result) < k; offset++ {
		// Bucket superior
		if startBucket+offset < NumBuckets {
			bucket := rt.Buckets[startBucket+offset]
			if bucket != nil {
				candidates := bucket.FindClosest(target, k-len(result))
				result = append(result, candidates...)
			}
		}

		// Bucket inferior (evitar duplicados cuando offset=0)
		if offset > 0 && startBucket-offset >= 0 {
			bucket := rt.Buckets[startBucket-offset]
			if bucket != nil {
				candidates := bucket.FindClosest(target, k-len(result))
				result = append(result, candidates...)
			}
		}
	}

	return result
}

// GetNode recupera un nodo por ID
func (rt *RoutingTable) GetNode(id NodeID) (*NodeEntry, bool) {
	bucketIdx := rt.getBucketIndex(id)
	bucket := rt.Buckets[bucketIdx]
	if bucket == nil {
		return nil, false
	}
	return bucket.Get(id)
}

// RemoveNode elimina un nodo de la tabla
func (rt *RoutingTable) RemoveNode(id NodeID) bool {
	bucketIdx := rt.getBucketIndex(id)
	bucket := rt.Buckets[bucketIdx]
	if bucket == nil {
		return false
	}
	return bucket.Remove(id)
}

// TotalNodes retorna la cantidad total de nodos en la tabla
func (rt *RoutingTable) TotalNodes() int {
	total := 0
	for _, bucket := range rt.Buckets {
		if bucket != nil {
			total += bucket.Size()
		}
	}
	return total
}

// GetBucketStats retorna estadísticas de cada bucket
func (rt *RoutingTable) GetBucketStats() map[int]int {
	stats := make(map[int]int)
	for i, bucket := range rt.Buckets {
		if bucket != nil {
			stats[i] = bucket.Size()
		}
	}
	return stats
}

// HashKey convierte una clave (string o []byte) en NodeID
func HashKey(key []byte) NodeID {
	hash := sha256.Sum256(key)
	var nodeID NodeID
	copy(nodeID[:], hash[:])
	return nodeID
}

// GenerateRandomNodeID genera un NodeID aleatorio (para pruebas)
func GenerateRandomNodeID() NodeID {
	var id NodeID
	hash := sha256.Sum256([]byte(time.Now().String()))
	copy(id[:], hash[:])
	return id
}

// Uint64ToNodeID convierte uint64 a NodeID (útil para rangos)
func Uint64ToNodeID(value uint64) NodeID {
	var id NodeID
	binary.BigEndian.PutUint64(id[:], value)
	return id
}
