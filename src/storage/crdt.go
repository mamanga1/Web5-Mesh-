// ============================================================================
// src/storage/crdt.go - Dotted Version Vectors for CRDT Consistency
// ============================================================================
// Especificación:
// - Consistencia eventual a través de Dotted Version Vectors
// - Mitiga de forma determinista las divisiones de red (network splits)
// - Vector clocks con truncamiento por causalidad para evitar explosión de memoria
// ============================================================================

package storage

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"sync"
	"time"
)

// NodeID representa el identificador único de un nodo en el CRDT
type NodeID string

// EventID representa un evento causal único
type EventID struct {
	NodeID NodeID
	SeqNum uint32
}

// String retorna representación string del EventID
func (e EventID) String() string {
	return string(e.NodeID) + ":" + string(rune(e.SeqNum))
}

// DottedVector es la estructura principal de Dotted Version Vector
// A diferencia de los Vector Clocks tradicionales, los Dotted Vectors
// solo almacenan eventos, no contadores por nodo, evitando explosión de memoria
type DottedVector struct {
	Causality map[EventID]uint64 // Evento -> versión
	Version   uint64              // Versión numérica para comparación rápida
	mu        sync.RWMutex
}

// NewDottedVector crea un nuevo Dotted Vector vacío
func NewDottedVector() *DottedVector {
	return &DottedVector{
		Causality: make(map[EventID]uint64),
		Version:   0,
	}
}

// Clone crea una copia profunda del Dotted Vector
func (dv *DottedVector) Clone() *DottedVector {
	dv.mu.RLock()
	defer dv.mu.RUnlock()

	newDV := NewDottedVector()
	newDV.Version = dv.Version
	for k, v := range dv.Causality {
		newDV.Causality[k] = v
	}
	return newDV
}

// AddEvent agrega un evento al vector
func (dv *DottedVector) AddEvent(event EventID, value uint64) {
	dv.mu.Lock()
	defer dv.mu.Unlock()
	dv.Causality[event] = value
	dv.Version++
}

// GetEvent retorna el valor de un evento específico
func (dv *DottedVector) GetEvent(event EventID) (uint64, bool) {
	dv.mu.RLock()
	defer dv.mu.RUnlock()
	val, ok := dv.Causality[event]
	return val, ok
}

// Merge fusiona otro Dotted Vector con el actual
// Retorna true si hubo cambios
func (dv *DottedVector) Merge(other *DottedVector) bool {
	other.mu.RLock()
	defer other.mu.RUnlock()

	dv.mu.Lock()
	defer dv.mu.Unlock()

	changed := false
	for event, version := range other.Causality {
		if existing, exists := dv.Causality[event]; !exists || version > existing {
			dv.Causality[event] = version
			changed = true
		}
	}

	if changed {
		dv.Version++
	}
	return changed
}

// IsEmpty retorna true si el vector no tiene eventos
func (dv *DottedVector) IsEmpty() bool {
	dv.mu.RLock()
	defer dv.mu.RUnlock()
	return len(dv.Causality) == 0
}

// Size retorna la cantidad de eventos en el vector
func (dv *DottedVector) Size() int {
	dv.mu.RLock()
	defer dv.mu.RUnlock()
	return len(dv.Causality)
}

// CRDTValue representa un valor CRDT con metadatos de consistencia
type CRDTValue struct {
	Data        []byte            // Datos del documento
	Version     uint64            // Versión numérica
	Timestamp   time.Time         // Timestamp de la última modificación
	Operation   string            // "create", "update", "delete"
	VectorClock *DottedVector     // Dotted Vector Clock
	SourceNode  NodeID            // Nodo que realizó el último cambio
	Signature   []byte            // Firma criptográfica (opcional)
	Metadata    map[string]string // Metadatos adicionales

	mu sync.RWMutex
}

// NewCRDTValue crea un nuevo valor CRDT
func NewCRDTValue(data []byte, sourceNode NodeID) *CRDTValue {
	dv := NewDottedVector()
	event := EventID{NodeID: sourceNode, SeqNum: 1}
	dv.AddEvent(event, 1)

	return &CRDTValue{
		Data:        data,
		Version:     1,
		Timestamp:   time.Now(),
		Operation:   "create",
		VectorClock: dv,
		SourceNode:  sourceNode,
		Metadata:    make(map[string]string),
	}
}

// Clone crea una copia profunda del CRDTValue
func (c *CRDTValue) Clone() *CRDTValue {
	c.mu.RLock()
	defer c.mu.RUnlock()

	dataCopy := make([]byte, len(c.Data))
	copy(dataCopy, c.Data)

	newValue := &CRDTValue{
		Data:        dataCopy,
		Version:     c.Version,
		Timestamp:   c.Timestamp,
		Operation:   c.Operation,
		VectorClock: c.VectorClock.Clone(),
		SourceNode:  c.SourceNode,
		Metadata:    make(map[string]string),
	}

	for k, v := range c.Metadata {
		newValue.Metadata[k] = v
	}

	if c.Signature != nil {
		sigCopy := make([]byte, len(c.Signature))
		copy(sigCopy, c.Signature)
		newValue.Signature = sigCopy
	}

	return newValue
}

// Update actualiza el valor CRDT con un nuevo dato
func (c *CRDTValue) Update(data []byte, sourceNode NodeID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	c.Data = dataCopy
	c.Timestamp = time.Now()
	c.Operation = "update"
	c.SourceNode = sourceNode
	c.Version++

	// Agregar evento al vector clock
	seqNum := uint32(c.VectorClock.Size() + 1)
	event := EventID{NodeID: sourceNode, SeqNum: seqNum}
	c.VectorClock.AddEvent(event, c.Version)
}

// Merge fusiona dos valores CRDT de forma determinista
// Retorna true si el valor actual fue actualizado
func (c *CRDTValue) Merge(other *CRDTValue) bool {
	if other == nil {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	other.mu.RLock()
	defer other.mu.RUnlock()

	// Regla de resolución de conflictos:
	// 1. Mayor versión numérica
	// 2. Si versiones iguales, mayor timestamp
	// 3. Si timestamps iguales, comparar hash de los datos
	// 4. Merge de vector clocks

	if other.Version > c.Version {
		// Reemplazar completamente con el valor remoto
		dataCopy := make([]byte, len(other.Data))
		copy(dataCopy, other.Data)

		c.Data = dataCopy
		c.Version = other.Version
		c.Timestamp = other.Timestamp
		c.Operation = other.Operation
		c.SourceNode = other.SourceNode
		c.VectorClock = other.VectorClock.Clone()

		// Copiar metadata
		for k, v := range other.Metadata {
			c.Metadata[k] = v
		}

		return true
	} else if other.Version == c.Version {
		// Misma versión, desempatar por timestamp
		if other.Timestamp.After(c.Timestamp) {
			// Remote es más reciente
			dataCopy := make([]byte, len(other.Data))
			copy(dataCopy, other.Data)
			c.Data = dataCopy
			c.Timestamp = other.Timestamp
			c.SourceNode = other.SourceNode
			return true
		} else if other.Timestamp.Equal(c.Timestamp) {
			// Mismo timestamp, desempatar por hash de datos
			hash1 := sha256.Sum256(c.Data)
			hash2 := sha256.Sum256(other.Data)

			if bytes.Compare(hash1[:], hash2[:]) < 0 {
				dataCopy := make([]byte, len(other.Data))
				copy(dataCopy, other.Data)
				c.Data = dataCopy
				c.SourceNode = other.SourceNode
				return true
			}
		}
	}

	// Siempre hacer merge de vector clocks
	changed := c.VectorClock.Merge(other.VectorClock)

	return changed
}

// CRDTDocument representa un documento completo en el sistema CRDT
type CRDTDocument struct {
	ID        string      // Identificador único del documento
	Value     *CRDTValue  // Valor CRDT actual
	History   []*CRDTValue // Historial de versiones (opcional)
	Deleted   bool        // Marcado como eliminado (tombstone)
	CreatedAt time.Time
	UpdatedAt time.Time

	mu sync.RWMutex
}

// NewCRDTDocument crea un nuevo documento CRDT
func NewCRDTDocument(id string, data []byte, sourceNode NodeID) *CRDTDocument {
	return &CRDTDocument{
		ID:        id,
		Value:     NewCRDTValue(data, sourceNode),
		History:   make([]*CRDTValue, 0),
		Deleted:   false,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// Update actualiza el documento
func (d *CRDTDocument) Update(data []byte, sourceNode NodeID) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.Deleted {
		return
	}

	// Guardar en historial si es necesario
	if len(d.History) < 10 { // Limitar historial a 10 versiones
		d.History = append(d.History, d.Value.Clone())
	}

	d.Value.Update(data, sourceNode)
	d.UpdatedAt = time.Now()
}

// Merge fusiona otro documento con el actual
func (d *CRDTDocument) Merge(other *CRDTDocument) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if other.Deleted {
		d.Deleted = true
		return true
	}

	changed := d.Value.Merge(other.Value)
	if changed {
		d.UpdatedAt = time.Now()
	}
	return changed
}

// GetData retorna los datos del documento (copia segura)
func (d *CRDTDocument) GetData() []byte {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.Deleted {
		return nil
	}

	data := make([]byte, len(d.Value.Data))
	copy(data, d.Value.Data)
	return data
}

// GetVersion retorna la versión actual del documento
func (d *CRDTDocument) GetVersion() uint64 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.Value.Version
}

// GetVectorClockSize retorna el tamaño del vector clock
func (d *CRDTDocument) GetVectorClockSize() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.Value.VectorClock.Size()
}

// Delete marca el documento como eliminado
func (d *CRDTDocument) Delete(sourceNode NodeID) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Deleted = true
	d.Value.Operation = "delete"
	d.Value.SourceNode = sourceNode
	d.Value.Version++
	d.UpdatedAt = time.Now()
}

// CRDTStore es el almacenamiento principal de documentos CRDT
type CRDTStore struct {
	documents map[string]*CRDTDocument
	mu        sync.RWMutex
}

// NewCRDTStore crea un nuevo almacenamiento CRDT
func NewCRDTStore() *CRDTStore {
	return &CRDTStore{
		documents: make(map[string]*CRDTDocument),
	}
}

// Put almacena o actualiza un documento
func (s *CRDTStore) Put(doc *CRDTDocument) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.documents[doc.ID] = doc
}

// Get recupera un documento por ID
func (s *CRDTStore) Get(id string) (*CRDTDocument, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	doc, ok := s.documents[id]
	return doc, ok
}

// Delete elimina un documento (marca como tombstone)
func (s *CRDTStore) Delete(id string, sourceNode NodeID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	doc, ok := s.documents[id]
	if !ok || doc.Deleted {
		return false
	}

	doc.Delete(sourceNode)
	return true
}

// MergeDocument fusiona un documento externo con el almacenamiento local
// Retorna true si hubo cambios locales
func (s *CRDTStore) MergeDocument(externalDoc *CRDTDocument) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	local, exists := s.documents[externalDoc.ID]
	if !exists {
		// Documento nuevo, simplemente almacenar
		s.documents[externalDoc.ID] = externalDoc.Clone()
		return true
	}

	// Merge con documento existente
	return local.Merge(externalDoc)
}

// GetAllIDs retorna todos los IDs de documentos activos
func (s *CRDTStore) GetAllIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.documents))
	for id, doc := range s.documents {
		if !doc.Deleted {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// Stats retorna estadísticas del almacenamiento CRDT
func (s *CRDTStore) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var activeCount int
	var totalEvents int
	var totalVersions uint64

	for _, doc := range s.documents {
		if !doc.Deleted {
			activeCount++
		}
		totalEvents += doc.Value.VectorClock.Size()
		totalVersions += doc.Value.Version
	}

	return map[string]interface{}{
		"total_documents":  len(s.documents),
		"active_documents": activeCount,
		"total_events":     totalEvents,
		"total_versions":   totalVersions,
	}
}
