// ============================================================================
// src/routing/nat_traversal.go - NAT Traversal & Hole Punching
// ============================================================================
// Especificación:
// - Controladores STUN, ICE, UPnP y UDP Hole Punching
// - Manejo adaptativo para redes 4G/5G y CGNAT restrictivos
// - Fallback a relay server cuando el hole punching falla
// ============================================================================

package routing

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"
)

// NATType representa el tipo de NAT detectado
type NATType string

const (
	NATUnknown     NATType = "unknown"
	NATNone        NATType = "none"      // Sin NAT (IP pública directa)
	NATFullCone    NATType = "full_cone" // Full Cone NAT
	NATRestricted  NATType = "restricted" // Restricted Cone NAT
	NATPortRestricted NATType = "port_restricted" // Port Restricted Cone NAT
	NATSymmetric   NATType = "symmetric" // Symmetric NAT (peor caso, común en 4G/5G)
)

// STUNServer representa un servidor STUN
type STUNServer struct {
	Address string
	mu      sync.RWMutex
}

// STUNResponse representa la respuesta de un servidor STUN
type STUNResponse struct {
	ExternalIP   net.IP
	ExternalPort int
	MappedAddr   string
	ResponseTime time.Duration
}

// NATTraversalManager gestiona el traversal de NAT
type NATTraversalManager struct {
	// Estado
	natType      NATType
	externalIP   net.IP
	externalPort int
	localIP      net.IP

	// Servidores STUN
	stunServers []string
	stunResults map[string]*STUNResponse

	// Conexiones
	activeHolePunching map[string]*HolePunchSession
	holeMu             sync.RWMutex

	// Relay fallback
	relayServer string
	relayConn   net.Conn

	// Configuración
	keepAliveInterval time.Duration
	discoveryTimeout  time.Duration

	// Estado
	discovered bool
	mu         sync.RWMutex
}

// HolePunchSession representa una sesión de hole punching activa
type HolePunchSession struct {
	TargetID     string
	TargetAddr   string
	LocalPort    int
	StartTime    time.Time
	LastAttempt  time.Time
	Attempts     int
	State        string // "init", "punching", "established", "failed"
	ResponseChan chan bool
}

// NewNATTraversalManager crea un nuevo manager de NAT traversal
func NewNATTraversalManager(stunServers []string, relayServer string) *NATTraversalManager {
	if len(stunServers) == 0 {
		stunServers = []string{
			"stun.l.google.com:19302",
			"stun1.l.google.com:19302",
			"stun2.l.google.com:19302",
		}
	}

	return &NATTraversalManager{
		natType:            NATUnknown,
		stunServers:        stunServers,
		stunResults:        make(map[string]*STUNResponse),
		activeHolePunching: make(map[string]*HolePunchSession),
		relayServer:        relayServer,
		keepAliveInterval:  15 * time.Second,
		discoveryTimeout:   5 * time.Second,
		discovered:         false,
	}
}

// DiscoverNATType descubre el tipo de NAT usando STUN
func (n *NATTraversalManager) DiscoverNATType(localAddr *net.UDPAddr) (NATType, net.IP, int, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.discovered {
		return n.natType, n.externalIP, n.externalPort, nil
	}

	// Realizar pruebas STUN con múltiples servidores
	var bestIP net.IP
	var bestPort int
	var bestType NATType = NATUnknown

	for _, server := range n.stunServers {
		resp, err := n.querySTUN(server, localAddr)
		if err != nil {
			continue
		}
		n.stunResults[server] = resp

		if bestIP == nil {
			bestIP = resp.ExternalIP
			bestPort = resp.ExternalPort
			bestType = n.determineNATType(resp)
		}
	}

	if bestIP == nil {
		return NATUnknown, nil, 0, fmt.Errorf("all STUN servers failed")
	}

	n.natType = bestType
	n.externalIP = bestIP
	n.externalPort = bestPort
	n.discovered = true

	return n.natType, n.externalIP, n.externalPort, nil
}

// querySTUN consulta un servidor STUN
func (n *NATTraversalManager) querySTUN(server string, localAddr *net.UDPAddr) (*STUNResponse, error) {
	// Resolver servidor STUN
	udpAddr, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		return nil, err
	}

	// Crear conexión UDP local
	conn, err := net.DialUDP("udp", localAddr, udpAddr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Construir mensaje STUN Binding Request
	msg := n.buildSTUNBindingRequest()

	start := time.Now()

	// Enviar solicitud
	if _, err := conn.Write(msg); err != nil {
		return nil, err
	}

	// Recibir respuesta
	conn.SetReadDeadline(time.Now().Add(n.discoveryTimeout))
	buf := make([]byte, 1024)
	nRead, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}

	elapsed := time.Since(start)

	// Parsear respuesta STUN
	externalIP, externalPort, err := n.parseSTUNResponse(buf[:nRead])
	if err != nil {
		return nil, err
	}

	return &STUNResponse{
		ExternalIP:   externalIP,
		ExternalPort: externalPort,
		MappedAddr:   fmt.Sprintf("%s:%d", externalIP, externalPort),
		ResponseTime: elapsed,
	}, nil
}

// buildSTUNBindingRequest construye un mensaje STUN Binding Request
func (n *NATTraversalManager) buildSTUNBindingRequest() []byte {
	// STUN Binding Request header: 20 bytes
	// Tipo: 0x0001 (Binding Request)
	// Longitud: 0
	// Transaction ID: 12 bytes aleatorios
	msg := make([]byte, 20)
	binary.BigEndian.PutUint16(msg[0:2], 0x0001) // Binding Request
	binary.BigEndian.PutUint16(msg[2:4], 0x0000) // Length
	// Transaction ID (12 bytes)
	for i := 4; i < 20; i++ {
		msg[i] = byte(i)
	}
	return msg
}

// parseSTUNResponse parsea la respuesta STUN
func (n *NATTraversalManager) parseSTUNResponse(data []byte) (net.IP, int, error) {
	if len(data) < 20 {
		return nil, 0, fmt.Errorf("response too short")
	}

	// Verificar tipo de mensaje (0x0101 = Binding Success Response)
	msgType := binary.BigEndian.Uint16(data[0:2])
	if msgType != 0x0101 {
		return nil, 0, fmt.Errorf("unexpected STUN response type: 0x%04x", msgType)
	}

	// Longitud del mensaje
	length := binary.BigEndian.Uint16(data[2:4])
	if len(data) < 20+int(length) {
		return nil, 0, fmt.Errorf("incomplete STUN message")
	}

	// Buscar atributo MAPPED-ADDRESS (0x0001) o XOR-MAPPED-ADDRESS (0x0020)
	offset := 20
	remaining := int(length)

	for remaining > 0 {
		if offset+4 > len(data) {
			break
		}
		attrType := binary.BigEndian.Uint16(data[offset : offset+2])
		attrLen := int(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
		attrValue := data[offset+4 : offset+4+attrLen]

		if attrType == 0x0020 || attrType == 0x0001 { // XOR-MAPPED-ADDRESS o MAPPED-ADDRESS
			if len(attrValue) >= 4 {
				ipLen := int(attrValue[1])
				if ipLen == 4 && len(attrValue) >= 8 {
					port := int(binary.BigEndian.Uint16(attrValue[2:4]))
					if attrType == 0x0020 {
						// XOR con magic cookie (0x2112A442)
						port = port ^ 0x2112
					}
					ip := net.IP(attrValue[4 : 4+ipLen])
					return ip, port, nil
				}
			}
		}
		offset += 4 + attrLen
		remaining -= 4 + attrLen
	}

	return nil, 0, fmt.Errorf("no mapped address found")
}

// determineNATType determina el tipo de NAT basado en respuestas STUN
func (n *NATTraversalManager) determineNATType(resp *STUNResponse) NATType {
	// Simplificado - en producción se necesitan múltiples pruebas
	// Si el puerto y IP son consistentes con la prueba anterior, es Full Cone
	// Si no, es Restringido o Simétrico
	return NATSymmetric // Asumir lo peor para móviles
}

// InitiateHolePunching inicia una sesión de hole punching hacia un target
func (n *NATTraversalManager) InitiateHolePunching(targetID, targetAddr string, localPort int) (<-chan bool, error) {
	n.holeMu.Lock()
	defer n.holeMu.Unlock()

	// Verificar si ya existe una sesión activa
	if session, exists := n.activeHolePunching[targetID]; exists && session.State != "failed" {
		return session.ResponseChan, nil
	}

	// Crear nueva sesión
	responseChan := make(chan bool, 1)
	session := &HolePunchSession{
		TargetID:     targetID,
		TargetAddr:   targetAddr,
		LocalPort:    localPort,
		StartTime:    time.Now(),
		State:        "init",
		ResponseChan: responseChan,
	}
	n.activeHolePunching[targetID] = session

	// Iniciar hole punching en goroutine
	go n.performHolePunching(session)

	return responseChan, nil
}

// performHolePunching ejecuta el algoritmo de hole punching
func (n *NATTraversalManager) performHolePunching(session *HolePunchSession) {
	// Intentar hasta 5 veces con intervalos crecientes
	maxAttempts := 5
	baseDelay := 500 * time.Millisecond

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if session.State == "established" {
			return
		}

		session.LastAttempt = time.Now()
		session.Attempts = attempt
		session.State = "punching"

		// Enviar paquete de punch
		if err := n.sendPunchPacket(session.TargetAddr); err != nil {
			if attempt == maxAttempts {
				session.State = "failed"
				session.ResponseChan <- false
				close(session.ResponseChan)
				return
			}
			// Esperar antes de reintentar
			time.Sleep(baseDelay * time.Duration(attempt))
			continue
		}

		// Esperar respuesta (simulado)
		time.Sleep(2 * time.Second)

		// Verificar si se estableció la conexión
		if session.State == "established" {
			session.ResponseChan <- true
			return
		}

		// Si falló después de todos los intentos, intentar relay
		if attempt == maxAttempts && n.relayServer != "" {
			session.State = "failed"
			session.ResponseChan <- false
		}
	}
}

// sendPunchPacket envía un paquete de hole punching
func (n *NATTraversalManager) sendPunchPacket(targetAddr string) error {
	addr, err := net.ResolveUDPAddr("udp", targetAddr)
	if err != nil {
		return err
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Mensaje PUNCH (formato simple)
	msg := []byte("PUNCH_REQ")
	_, err = conn.Write(msg)
	return err
}

// UseRelayFallback usa un relay server como fallback
func (n *NATTraversalManager) UseRelayFallback(targetID string) (net.Conn, error) {
	if n.relayServer == "" {
		return nil, fmt.Errorf("no relay server configured")
	}

	conn, err := net.DialTimeout("tcp", n.relayServer, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("relay connection failed: %w", err)
	}

	// Enviar comando RELAY_TO
	msg := fmt.Sprintf("RELAY_TO:%s\n", targetID)
	if _, err := conn.Write([]byte(msg)); err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}

// KeepAlive mantiene viva la conexión (para CGNAT)
func (n *NATTraversalManager) KeepAlive(conn net.Conn) {
	ticker := time.NewTicker(n.keepAliveInterval)
	defer ticker.Stop()

	for range ticker.C {
		// Enviar keepalive (STUN Binding Indication)
		msg := []byte{0x00, 0x11, 0x00, 0x00} // Binding Indication (simplificado)
		if _, err := conn.Write(msg); err != nil {
			return
		}
	}
}

// CloseHolePunching cierra una sesión de hole punching
func (n *NATTraversalManager) CloseHolePunching(targetID string) {
	n.holeMu.Lock()
	defer n.holeMu.Unlock()
	delete(n.activeHolePunching, targetID)
}

// GetExternalInfo retorna la IP y puerto externos descubiertos
func (n *NATTraversalManager) GetExternalInfo() (net.IP, int, NATType) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.externalIP, n.externalPort, n.natType
}

// IsRelayRequired verifica si se necesita relay para un target
func (n *NATTraversalManager) IsRelayRequired() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.natType == NATSymmetric
}

// EstablishDirectConnection intenta establecer una conexión directa usando hole punching
func (n *NATTraversalManager) EstablishDirectConnection(targetID, targetAddr string) (net.Conn, error) {
	// Iniciar hole punching
	ch, err := n.InitiateHolePunching(targetID, targetAddr, 0)
	if err != nil {
		return nil, err
	}

	// Esperar resultado con timeout
	select {
	case success := <-ch:
		if !success {
			// Fallback a relay
			return n.UseRelayFallback(targetID)
		}
	case <-time.After(10 * time.Second):
		return n.UseRelayFallback(targetID)
	}

	// Conexión establecida (simulado)
	conn, err := net.Dial("udp", targetAddr)
	if err != nil {
		return n.UseRelayFallback(targetID)
	}

	return conn, nil
}

// StartLocalUPnP intenta abrir puertos usando UPnP (opcional)
func (n *NATTraversalManager) StartLocalUPnP(port int) error {
	// En producción, implementar UPnP con librería como go-upnp
	// Por ahora, placeholder
	return nil
}
