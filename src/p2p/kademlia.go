package p2p

import (
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

type Kademlia struct {
	localID   NodeID
	transport *TransportUDP
	running   bool
	mu        sync.RWMutex
}

func NewKademlia(transport *TransportUDP) *Kademlia {
	return &Kademlia{
		localID:   GenerateNodeID(),
		transport: transport,
		running:   true,
	}
}

func (k *Kademlia) LocalID() NodeID {
	return k.localID
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
		}
	}
}

func (k *Kademlia) telemetryLoop() {
	for k.running {
		time.Sleep(30 * time.Second)
		telemetry.Log()
	}
}
