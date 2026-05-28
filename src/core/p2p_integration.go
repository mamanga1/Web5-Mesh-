package core

import (
	"log"
	"time"

	"github.com/mamanga1/web5-mesh/src/p2p"
)

func (n *SovereignNode) InitP2P() {
	transport, err := p2p.NewTransportUDP(n.config.Network.UDPPort, 10*time.Second, 5*time.Second)
	if err != nil {
		log.Printf("[P2P] Failed to create transport: %v", err)
		return
	}
	n.p2pTransport = transport
	n.kademlia = p2p.NewKademlia(transport, n.identity)
	n.kademlia.Start()
	log.Printf("[P2P] Kademlia started with Node ID: %x", n.kademlia.LocalID())
}
