// ============================================================================
// src/storage/crdt.go - Dotted Version Vectors for CRDT Consistency
// ============================================================================

package storage

import (
	"bytes"
	"crypto/sha256"
	"sort"
	"sync"
	"time"
)

type NodeID string

type EventID struct {
	NodeID NodeID
	SeqNum uint32
}

func (e EventID) String() string {
	return string(e.NodeID) + ":" + string(rune(e.SeqNum))
}

type DottedVector struct {
	Causality map[EventID]uint64
	Version   uint64
	mu        sync.RWMutex
}

func NewDottedVector() *DottedVector {
	return &DottedVector{
		Causality: make(map[EventID]uint64),
		Version:   0,
	}
}

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

func (dv *DottedVector) AddEvent(event EventID, value uint64) {
	dv.mu.Lock()
	defer dv.mu.Unlock()
	dv.Causality[event] = value
	dv.Version++
}

func (dv *DottedVector) GetEvent(event EventID) (uint64, bool) {
	dv.mu.RLock()
	defer dv.mu.RUnlock()
	val, ok := dv.Causality[event]
	return val, ok
}

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

func (dv *DottedVector) IsEmpty() bool {
	dv.mu.RLock()
	defer dv.mu.RUnlock()
	return len(dv.Causality) == 0
}

func (dv *DottedVector) Size() int {
	dv.mu.RLock()
	defer dv.mu.RUnlock()
	return len(dv.Causality)
}

type CRDTValue struct {
	Data        []byte
	Version     uint64
	Timestamp   time.Time
	Operation   string
	VectorClock *DottedVector
	SourceNode  NodeID
	Signature   []byte
	Metadata    map[string]string
	mu          sync.RWMutex
}

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
	seqNum := uint32(c.VectorClock.Size() + 1)
	event := EventID{NodeID: sourceNode, SeqNum: seqNum}
	c.VectorClock.AddEvent(event, c.Version)
}

func (c *CRDTValue) Merge(other *CRDTValue) bool {
	if other == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	other.mu.RLock()
	defer other.mu.RUnlock()
	if other.Version > c.Version {
		dataCopy := make([]byte, len(other.Data))
		copy(dataCopy, other.Data)
		c.Data = dataCopy
		c.Version = other.Version
		c.Timestamp = other.Timestamp
		c.Operation = other.Operation
		c.SourceNode = other.SourceNode
		c.VectorClock = other.VectorClock.Clone()
		for k, v := range other.Metadata {
			c.Metadata[k] = v
		}
		return true
	} else if other.Version == c.Version {
		if other.Timestamp.After(c.Timestamp) {
			dataCopy := make([]byte, len(other.Data))
			copy(dataCopy, other.Data)
			c.Data = dataCopy
			c.Timestamp = other.Timestamp
			c.SourceNode = other.SourceNode
			return true
		} else if other.Timestamp.Equal(c.Timestamp) {
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
	return c.VectorClock.Merge(other.VectorClock)
}

type CRDTDocument struct {
	ID        string
	Value     *CRDTValue
	History   []*CRDTValue
	Deleted   bool
	CreatedAt time.Time
	UpdatedAt time.Time
	mu        sync.RWMutex
}

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

func (d *CRDTDocument) Clone() *CRDTDocument {
	if d == nil {
		return nil
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	newDoc := &CRDTDocument{
		ID:        d.ID,
		Deleted:   d.Deleted,
		CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt,
		History:   make([]*CRDTValue, 0, len(d.History)),
	}
	if d.Value != nil {
		newDoc.Value = d.Value.Clone()
	}
	for _, v := range d.History {
		newDoc.History = append(newDoc.History, v.Clone())
	}
	return newDoc
}

func (d *CRDTDocument) Update(data []byte, sourceNode NodeID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.Deleted {
		return
	}
	if len(d.History) < 10 {
		d.History = append(d.History, d.Value.Clone())
	}
	d.Value.Update(data, sourceNode)
	d.UpdatedAt = time.Now()
}

func (d *CRDTDocument) Merge(other *CRDTDocument) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if other.Deleted {
		d.Deleted = true
		return true
	}
	return d.Value.Merge(other.Value)
}

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

func (d *CRDTDocument) GetVersion() uint64 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.Value.Version
}

func (d *CRDTDocument) GetVectorClockSize() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.Value.VectorClock.Size()
}

func (d *CRDTDocument) Delete(sourceNode NodeID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Deleted = true
	d.Value.Operation = "delete"
	d.Value.SourceNode = sourceNode
	d.Value.Version++
	d.UpdatedAt = time.Now()
}

type CRDTStore struct {
	documents map[string]*CRDTDocument
	mu        sync.RWMutex
}

func NewCRDTStore() *CRDTStore {
	return &CRDTStore{
		documents: make(map[string]*CRDTDocument),
	}
}

func (s *CRDTStore) Put(doc *CRDTDocument) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.documents[doc.ID] = doc
}

func (s *CRDTStore) Get(id string) (*CRDTDocument, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	doc, ok := s.documents[id]
	return doc, ok
}

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

func (s *CRDTStore) MergeDocument(externalDoc *CRDTDocument) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	local, exists := s.documents[externalDoc.ID]
	if !exists {
		s.documents[externalDoc.ID] = externalDoc.Clone()
		return true
	}
	return local.Merge(externalDoc)
}

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
