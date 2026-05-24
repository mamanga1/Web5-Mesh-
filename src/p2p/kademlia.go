package p2p

import (
	"crypto/rand"
	"net"
	"sync"
	"time"
)

// NodeID es un identificador de 20 bytes (160 bits como Kademlia)
type NodeID [20]byte

// GenerateNodeID genera un ID aleatorio
func GenerateNodeID() NodeID {
	var id NodeID
	rand.Read(id[:])
	return id
}

// XorDistance calcula la distancia XOR entre dos IDs
func XorDistance(a, b NodeID) uint64 {
	var result uint64
	for i := 0; i < 20; i++ {
		result = (result << 8) | uint64(a[i]^b[i])
	}
	return result
}

// Contact representa un nodo en la red
type Contact struct {
	ID       NodeID
	Addr     *net.UDPAddr
	LastSeen time.Time
}

// Bucket es un k-bucket de Kademlia
type Bucket struct {
	contacts []Contact
	maxSize  int
	mu       sync.RWMutex
}

func NewBucket(maxSize int) *Bucket {
	if maxSize == 0 {
		maxSize = 20
	}
	return &Bucket{
		contacts: make([]Contact, 0),
		maxSize:  maxSize,
	}
}

func (b *Bucket) Add(contact Contact) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i, c := range b.contacts {
		if c.ID == contact.ID {
			b.contacts = append(b.contacts[:i], b.contacts[i+1:]...)
			b.contacts = append(b.contacts, contact)
			return
		}
	}

	if len(b.contacts) < b.maxSize {
		b.contacts = append(b.contacts, contact)
	} else {
		if time.Since(b.contacts[0].LastSeen) > 5*time.Minute {
			b.contacts[0] = contact
		}
	}
}

func (b *Bucket) GetContacts(limit int) []Contact {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if limit > len(b.contacts) {
		limit = len(b.contacts)
	}
	result := make([]Contact, limit)
	copy(result, b.contacts[:limit])
	return result
}

// Kademlia es el DHT principal
type Kademlia struct {
	localID   NodeID
	transport *TransportUDP
	buckets   []*Bucket
	mu        sync.RWMutex
	running   bool
}

// NewKademlia crea una nueva instancia de Kademlia
func NewKademlia(transport *TransportUDP) *Kademlia {
	k := &Kademlia{
		localID:   GenerateNodeID(),
		transport: transport,
		buckets:   make([]*Bucket, 160),
		running:   true,
	}
	for i := 0; i < 160; i++ {
		k.buckets[i] = NewBucket(20)
	}
	return k
}

// LocalID devuelve el ID local
func (k *Kademlia) LocalID() NodeID {
	return k.localID
}

// getBucketIndex devuelve el bucket para una distancia
func (k *Kademlia) getBucketIndex(targetID NodeID) int {
	dist := XorDistance(k.localID, targetID)
	if dist == 0 {
		return 0
	}
	for i := 159; i >= 0; i-- {
		if dist&(1<<uint(i)) != 0 {
			return i
		}
	}
	return 0
}

// AddContact agrega un contacto a la tabla de routing
func (k *Kademlia) AddContact(contact Contact) {
	contact.LastSeen = time.Now()
	index := k.getBucketIndex(contact.ID)
	k.buckets[index].Add(contact)
}

// FindClosest encuentra los contactos más cercanos a un ID
func (k *Kademlia) FindClosest(targetID NodeID, count int) []Contact {
	if count <= 0 {
		count = 20
	}

	var allContacts []Contact
	for i := 0; i < 160; i++ {
		contacts := k.buckets[i].GetContacts(count)
		allContacts = append(allContacts, contacts...)
	}

	for i := 0; i < len(allContacts)-1; i++ {
		for j := i + 1; j < len(allContacts); j++ {
			if XorDistance(targetID, allContacts[i].ID) > XorDistance(targetID, allContacts[j].ID) {
				allContacts[i], allContacts[j] = allContacts[j], allContacts[i]
			}
		}
	}

	if len(allContacts) > count {
		allContacts = allContacts[:count]
	}
	return allContacts
}

// Ping envía un ping a un contacto
func (k *Kademlia) Ping(addr *net.UDPAddr) error {
	msg := []byte{0x01, 0x50, 0x49, 0x4e, 0x47}
	return k.transport.WriteTo(msg, addr)
}

// Start inicia el loop de procesamiento
func (k *Kademlia) Start() error {
	k.running = true
	go k.handleMessages()
	return nil
}

// Stop detiene el DHT
func (k *Kademlia) Stop() {
	k.running = false
}

func (k *Kademlia) handleMessages() {
	for k.running {
		data, addr, err := k.transport.ReadFrom()
		if err != nil {
			continue
		}
		if len(data) == 5 && data[0] == 0x01 && string(data[1:5]) == "PING" {
			pong := []byte{0x02, 0x50, 0x4f, 0x4e, 0x47}
			k.transport.WriteTo(pong, addr)
		}
	}
}
