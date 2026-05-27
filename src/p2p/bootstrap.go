package p2p

import (
	"log"
	"net"
	"time"
)

// Bootstrapper maneja la conexión inicial a los nodos semilla
type Bootstrapper struct {
	transport *TransportUDP
	kademlia  *Kademlia
	seeds     []string
}

// NewBootstrapper crea un nuevo bootstrapper
func NewBootstrapper(transport *TransportUDP, kademlia *Kademlia, seeds []string) *Bootstrapper {
	return &Bootstrapper{
		transport: transport,
		kademlia:  kademlia,
		seeds:     seeds,
	}
}

// Start inicia el bootstrap hacia los nodos semilla
func (b *Bootstrapper) Start() {
	if len(b.seeds) == 0 {
		log.Printf("[BOOTSTRAP] No seed nodes configured")
		return
	}

	for _, seed := range b.seeds {
		log.Printf("[BOOTSTRAP] Connecting to seed: %s", seed)
		
		addr, err := net.ResolveUDPAddr("udp", seed)
		if err != nil {
			log.Printf("[BOOTSTRAP] Failed to resolve seed %s: %v", seed, err)
			continue
		}

		// Enviar PING al seed
		if err := b.transport.WriteTo([]byte("PING"), addr); err != nil {
			log.Printf("[BOOTSTRAP] Failed to ping seed %s: %v", seed, err)
			continue
		}

		// Enviar FIND_NODE para obtener vecinos
		if err := b.transport.WriteTo([]byte("FIND_NODE"), addr); err != nil {
			log.Printf("[BOOTSTRAP] Failed to send FIND_NODE to %s: %v", seed, err)
			continue
		}

		log.Printf("[BOOTSTRAP] Successfully connected to seed: %s", seed)
	}
}

// BootstrapLoop ejecuta bootstrap periódicamente
func (b *Bootstrapper) BootstrapLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		b.Start()
	}
}
