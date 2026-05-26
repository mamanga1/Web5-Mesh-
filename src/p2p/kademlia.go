package p2p

import (
	// "log"
	"crypto/rand"
	"net"
	"sync"
	"time"
)

type NodeID [20]byte

func GenerateNodeID() NodeID {
	var id NodeID
	rand.Read(id[:])
	return id
}

type Contact struct {
	ID       NodeID
	Addr     *net.UDPAddr
	LastSeen time.Time
}

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

	// Si ya existe, mover al final
	for i, c := range b.contacts {
		if c.ID == contact.ID {
			b.contacts = append(b.contacts[:i], b.contacts[i+1:]...)
			b.contacts = append(b.contacts, contact)
			return
		}
	}

	// Si hay espacio, agregar
	if len(b.contacts) < b.maxSize {
		b.contacts = append(b.contacts, contact)
		return
	}

	// Si no hay espacio, reemplazar el más viejo si está muerto
	if time.Since(b.contacts[0].LastSeen) > 5*time.Minute {
		b.contacts[0] = contact
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

type Kademlia struct {
	localID   NodeID
	transport *TransportUDP
	buckets   []*Bucket
	running   bool
	mu        sync.RWMutex
    dataStore map[string][]byte
}

func NewKademlia(transport *TransportUDP) *Kademlia {
	k := &Kademlia{
		localID:   GenerateNodeID(),
		transport: transport,
		buckets:   make([]*Bucket, 160),
                dataStore: make(map[string][]byte),
		running:   true,
	}
	for i := 0; i < 160; i++ {
		k.buckets[i] = NewBucket(20)
	}
	return k
}

func (k *Kademlia) LocalID() NodeID {
	return k.localID
}

func (k *Kademlia) getBucketIndex(targetID NodeID) int {
	// Calcular distancia XOR
	var dist [20]byte
	for i := 0; i < 20; i++ {
		dist[i] = k.localID[i] ^ targetID[i]
	}
	// Encontrar el bit más significativo
	for i := 19; i >= 0; i-- {
		if dist[i] != 0 {
			// 8 bits por byte
			for bit := 7; bit >= 0; bit-- {
				if dist[i]&(1<<uint(bit)) != 0 {
					return i*8 + bit
				}
			}
		}
	}
	return 0
}

func (k *Kademlia) AddContact(contact Contact) {
	contact.LastSeen = time.Now()
	index := k.getBucketIndex(contact.ID)
	k.buckets[index].Add(contact)
}

func (k *Kademlia) FindClosest(targetID NodeID, count int) []Contact {
	if count <= 0 {
		count = 20
	}

	var allContacts []Contact
	for i := 0; i < 160; i++ {
		contacts := k.buckets[i].GetContacts(count)
		allContacts = append(allContacts, contacts...)
	}

	// Ordenar por distancia XOR
	for i := 0; i < len(allContacts)-1; i++ {
		for j := i + 1; j < len(allContacts); j++ {
			distI := xorDistance(targetID, allContacts[i].ID)
			distJ := xorDistance(targetID, allContacts[j].ID)
			if distI > distJ {
				allContacts[i], allContacts[j] = allContacts[j], allContacts[i]
			}
		}
	}

	if len(allContacts) > count {
		allContacts = allContacts[:count]
	}
	return allContacts
}

func xorDistance(a, b NodeID) uint64 {
	var result uint64
	for i := 0; i < 20; i++ {
		result = (result << 8) | uint64(a[i]^b[i])
	}
	return result
}

func (k *Kademlia) Start() error {
	k.running = true
	go k.handleMessages()
	go k.telemetryLoop()
	return nil
}

func (k *Kademlia) Stop() {
	k.running = false
}

func (k *Kademlia) Ping(addr *net.UDPAddr) error {
	telemetry.IncPingSent()
	return k.transport.WriteTo([]byte("PING"), addr)
}

func (k *Kademlia) handleMessages() {
	for k.running {
		data, addr, err := k.transport.ReadFrom()
		if err != nil {
			continue
		}
		msg := string(data)

		switch {
		case msg == "PING":
			telemetry.IncPingReceived()
			k.transport.WriteTo([]byte("PONG"), addr)
			telemetry.IncPongSent()
		case msg == "PONG":
			telemetry.IncPongReceived()
		case msg == "FIND_NODE":
			telemetry.IncFindNodeReceived()
			k.transport.WriteTo([]byte("NODES"), addr)
		case len(msg) > 6 && msg[:6] == "STORE:":
			key := msg[6:]
			k.Store(key, []byte("stored"))
		case len(msg) > 10 && msg[:10] == "FIND_VALUE:":
			key := msg[10:]
			if val, ok := k.FindValue(key); ok {
				k.transport.WriteTo([]byte("VALUE:"+string(val)), addr)
			}
		}
	}
}

func (k *Kademlia) telemetryLoop() {
	for k.running {
		time.Sleep(30 * time.Second)
		telemetry.Log()
	}
}
