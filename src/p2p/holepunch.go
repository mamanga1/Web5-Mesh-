package p2p

import (
    "log"
    "net"
    "strconv"
    "strings"
    "time"
)

type HolePuncher struct {
    transport *TransportUDP
    faroAddr  *net.UDPAddr
    nodeID    string
    nodeIP    string
    nodePort  int
}

type PeerInfo struct {
    NodeID   string
    IP       string
    Port     int
    LastSeen time.Time
}

var registeredPeers = make(map[string]*PeerInfo)

func NewHolePuncher(transport *TransportUDP, faroAddr string) *HolePuncher {
    addr, _ := net.ResolveUDPAddr("udp", faroAddr)
    return &HolePuncher{
        transport: transport,
        faroAddr:  addr,
    }
}

// SetNodeInfo establece la información del nodo local
func (h *HolePuncher) SetNodeInfo(nodeID string, ip string, port int) {
    h.nodeID = nodeID
    h.nodeIP = ip
    h.nodePort = port
}

// RegisterPublicIP registra la IP pública del nodo en el faro
func (h *HolePuncher) RegisterPublicIP(publicIP string, publicPort int) {
    msg := []byte("REGISTER:" + h.nodeID + ":" + publicIP + ":" + strconv.Itoa(publicPort))
    h.transport.WriteTo(msg, h.faroAddr)
    log.Printf("[HOLEPUNCH] Registered public IP: %s:%d", publicIP, publicPort)
}

// RequestPunch solicita al faro que conecte con otro nodo
func (h *HolePuncher) RequestPunch(targetID string) {
    msg := []byte("PUNCH_REQUEST:" + h.nodeID + ":" + targetID)
    h.transport.WriteTo(msg, h.faroAddr)
    log.Printf("[HOLEPUNCH] Requested punch to %s", targetID)
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

// SendPunchRequest envía una solicitud de punch al faro
func (h *HolePuncher) SendPunchRequest(targetID string, targetIP string, targetPort int) {
    msg := []byte("PUNCH:" + h.nodeID + ":" + h.nodeIP + ":" + strconv.Itoa(h.nodePort) + ":" + targetID + ":" + targetIP + ":" + strconv.Itoa(targetPort))
    h.transport.WriteTo(msg, h.faroAddr)
    log.Printf("[HOLEPUNCH] Sent punch request to faro for %s", targetID)
}

// HandlePunch procesa una solicitud de punch del faro
func (h *HolePuncher) HandlePunch(data []byte, fromAddr *net.UDPAddr) {
    msg := string(data)
    log.Printf("[HOLEPUNCH] Received punch data: %s", msg)

    parts := strings.Split(msg, ":")
    if len(parts) < 5 {
        return
    }

    // Formato esperado: PUNCH:targetID:targetIP:targetPort:initiatorID
    if parts[0] == "PUNCH" && len(parts) >= 5 {
        targetID := parts[1]
        targetIP := parts[2]
        targetPort, _ := strconv.Atoi(parts[3])

        log.Printf("[HOLEPUNCH] Punching %s (%s:%d) initiated by %s", targetID, targetIP, targetPort, parts[4])

        // Realizar el punch
        addr := &net.UDPAddr{IP: net.ParseIP(targetIP), Port: targetPort}
        for i := 0; i < 3; i++ {
            h.transport.WriteTo([]byte("SYN"), addr)
            time.Sleep(30 * time.Millisecond)
        }
    }
}

// ProcessIncomingMessage procesa mensajes entrantes relacionados con hole punching
func (h *HolePuncher) ProcessIncomingMessage(msg string, addr *net.UDPAddr) {
    if len(msg) < 4 {
        return
    }

    switch {
    case msg[:4] == "SYN":
        log.Printf("[HOLEPUNCH] SYN received from %s, sending ACK", addr.String())
        h.transport.WriteTo([]byte("ACK"), addr)

    case msg[:3] == "ACK":
        log.Printf("[HOLEPUNCH] ACK received from %s, connection established", addr.String())

    case len(msg) > 8 && msg[:8] == "REGISTER":
        parts := strings.Split(msg, ":")
        if len(parts) >= 4 {
            nodeID := parts[1]
            ip := parts[2]
            port, _ := strconv.Atoi(parts[3])

            registeredPeers[nodeID] = &PeerInfo{
                NodeID:   nodeID,
                IP:       ip,
                Port:     port,
                LastSeen: time.Now(),
            }
            log.Printf("[HOLEPUNCH] Registered peer %s at %s:%d", nodeID[:8], ip, port)
        }

    case len(msg) > 12 && msg[:12] == "PUNCH_REQUEST":
        parts := strings.Split(msg, ":")
        if len(parts) >= 3 {
            initiatorID := parts[1]
            targetID := parts[2]

            log.Printf("[HOLEPUNCH] Punch request from %s to %s", initiatorID[:8], targetID[:8])

            // Buscar información del target
            if target, ok := registeredPeers[targetID]; ok {
                // Enviar instrucción de punch al iniciador
                punchMsg := []byte("PUNCH:" + targetID + ":" + target.IP + ":" + strconv.Itoa(target.Port) + ":" + initiatorID)
                h.transport.WriteTo(punchMsg, addr)
                log.Printf("[HOLEPUNCH] Sent punch instruction to %s", initiatorID[:8])
            } else {
                log.Printf("[HOLEPUNCH] Target %s not registered", targetID[:8])
            }
        }
    }
}

// GetRegisteredPeers retorna la lista de peers registrados
func GetRegisteredPeers() map[string]*PeerInfo {
    return registeredPeers
}
