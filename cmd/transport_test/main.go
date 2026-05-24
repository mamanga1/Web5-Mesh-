package main

import (
	"fmt"
	"log"
	"time"

	"github.com/mamanga1/web5-mesh/src/p2p"
)

func main() {
	// Crear transporte en puerto 4244
	transport, err := p2p.NewTransportUDP(4244, 5*time.Second, 3*time.Second)
	if err != nil {
		log.Fatalf("Failed to create transport: %v", err)
	}
	defer transport.Close()

	fmt.Printf("✅ Transport listening on %s\n", transport.LocalAddr().String())

	// Goroutine para recibir mensajes
	go func() {
		for {
			data, addr, err := transport.ReadFrom()
			if err != nil {
				continue
			}
			fmt.Printf("📩 Received from %s: %s\n", addr.String(), string(data))
		}
	}()

	// Mantener vivo
	fmt.Println("Waiting for messages... (Ctrl+C to stop)")
	select {}
}
