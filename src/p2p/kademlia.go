package p2p

import (
	"crypto/rand"
	"fmt"
	"net"
)

type NodeID [20]byte

func GenerateNodeID() NodeID {
	var id NodeID
	rand.Read(id[:])
	return id
}

type Kademlia struct {
	localID   NodeID
	transport *TransportUDP
	running   bool
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
	return nil
}

func (k *Kademlia) Stop() {
	k.running = false
}

func (k *Kademlia) Ping(addr *net.UDPAddr) error {
	return k.transport.WriteTo([]byte("PING"), addr)
}

func (k *Kademlia) handleMessages() {
	for k.running {
		data, addr, err := k.transport.ReadFrom()
		if err != nil {
			fmt.Printf("[KAD] Read error: %v\n", err)
			continue
		}
		fmt.Printf("[KAD] Received %d bytes from %s: %s\n", len(data), addr.String(), string(data))

		if string(data) == "PING" {
			fmt.Printf("[KAD] PING from %s, sending PONG\n", addr.String())
			k.transport.WriteTo([]byte("PONG"), addr)
		}
	}
}
