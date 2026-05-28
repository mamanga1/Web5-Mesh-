package p2p

import (
    "log"
    "net"
    "time"
)

type HolePuncher struct {
    transport *TransportUDP
    faroAddr  *net.UDPAddr
}

func NewHolePuncher(transport *TransportUDP, faroAddr string) *HolePuncher {
    addr, _ := net.ResolveUDPAddr("udp", faroAddr)
    return &HolePuncher{
        transport: transport,
        faroAddr:  addr,
    }
}

// RegisterPublicIP registra la IP pública del nodo en el faro
func (h *HolePuncher) RegisterPublicIP(publicIP string, publicPort int) {
    msg := []byte("REGISTER:" + publicIP + ":" + string(rune(publicPort)))
    h.transport.WriteTo(msg, h.faroAddr)
    log.Printf("[HOLEPUNCH] Registered public IP: %s:%d", publicIP, publicPort)
}

// RequestPunch solicita al faro que conecte con otro nodo
func (h *HolePuncher) RequestPunch(targetID string) {
    msg := []byte("PUNCH_REQUEST:" + targetID)
    h.transport.WriteTo(msg, h.faroAddr)
}

// Punch realiza el golpe contra el target
func (h *HolePuncher) Punch(targetIP string, targetPort int) {
    addr := &net.UDPAddr{IP: net.ParseIP(targetIP), Port: targetPort}
    for i := 0; i < 5; i++ {
        h.transport.WriteTo([]byte("SYN"), addr)
        time.Sleep(50 * time.Millisecond)
    }
    log.Printf("[HOLEPUNCH] Punched %s:%d", targetIP, targetPort)
}
