package main

import (
	"fmt"
	"log"
	"time"

	"github.com/mamanga1/web5-mesh/src/p2p"
)

func main() {
	transport, err := p2p.NewTransportUDP(4245, 10*time.Second, 5*time.Second)
	if err != nil {
		log.Fatalf("Failed to create transport: %v", err)
	}
	defer transport.Close()

	kad := p2p.NewKademlia(transport)
	fmt.Printf("✅ Local Node ID: %x\n", kad.LocalID())

	if err := kad.Start(); err != nil {
		log.Fatalf("Failed to start Kademlia: %v", err)
	}

	fmt.Println("Kademlia running... (Ctrl+C to stop)")
	select {}
}
