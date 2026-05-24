package main

import (
	"fmt"
	"log"
	"time"

	"github.com/mamanga1/web5-mesh/src/p2p"
)

func main() {
	client := p2p.NewSTUNClient("stun.l.google.com:19302", 5*time.Second)
	ip, err := client.ExternalIP()
	if err != nil {
		log.Fatalf("STUN failed: %v", err)
	}
	fmt.Printf("✅ Public IP: %s\n", ip.String())
}
