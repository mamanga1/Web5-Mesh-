// ============================================================================
// src/routing/nat_traversal.go - NAT Traversal & Hole Punching
// ============================================================================

package routing

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"
)

type NATType string

const (
	NATUnknown        NATType = "unknown"
	NATNone           NATType = "none"
	NATFullCone       NATType = "full_cone"
	NATRestricted     NATType = "restricted"
	NATPortRestricted NATType = "port_restricted"
	NATSymmetric      NATType = "symmetric"
)

type STUNServer struct {
	Address string
	mu      sync.RWMutex
}

type STUNResponse struct {
	ExternalIP   net.IP
	ExternalPort int
	MappedAddr   string
	ResponseTime time.Duration
}

type HolePunchSession struct {
	TargetID     string
	TargetAddr   string
	LocalPort    int
	StartTime    time.Time
	LastAttempt  time.Time
	Attempts     int
	State        string
	ResponseChan chan bool
}

type NATTraversalManager struct {
	natType          NATType
	externalIP       net.IP
	externalPort     int
	localIP          net.IP
	stunServers      []string
	stunResults      map[string]*STUNResponse
	activeHolePunching map[string]*HolePunchSession
	holeMu           sync.RWMutex
	relayServer      string
	relayConn        net.Conn
	keepAliveInterval time.Duration
	discoveryTimeout time.Duration
	discovered       bool
	mu               sync.RWMutex
}

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

func (n *NATTraversalManager) DiscoverNATType(localAddr *net.UDPAddr) (NATType, net.IP, int, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.discovered {
		return n.natType, n.externalIP, n.externalPort, nil
	}
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

func (n *NATTraversalManager) querySTUN(server string, localAddr *net.UDPAddr) (*STUNResponse, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp", localAddr, udpAddr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	msg := n.buildSTUNBindingRequest()
	start := time.Now()
	if _, err := conn.Write(msg); err != nil {
		return nil, err
	}
	conn.SetReadDeadline(time.Now().Add(n.discoveryTimeout))
	buf := make([]byte, 1024)
	nRead, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	elapsed := time.Since(start)
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

func (n *NATTraversalManager) buildSTUNBindingRequest() []byte {
	msg := make([]byte, 20)
	binary.BigEndian.PutUint16(msg[0:2], 0x0001)
	binary.BigEndian.PutUint16(msg[2:4], 0x0000)
	for i := 4; i < 20; i++ {
		msg[i] = byte(i)
	}
	return msg
}

func (n *NATTraversalManager) parseSTUNResponse(data []byte) (net.IP, int, error) {
	if len(data) < 20 {
		return nil, 0, fmt.Errorf("response too short")
	}
	msgType := binary.BigEndian.Uint16(data[0:2])
	if msgType != 0x0101 {
		return nil, 0, fmt.Errorf("unexpected STUN response type: 0x%04x", msgType)
	}
	length := binary.BigEndian.Uint16(data[2:4])
	if len(data) < 20+int(length) {
		return nil, 0, fmt.Errorf("incomplete STUN message")
	}
	offset := 20
	remaining := int(length)
	for remaining > 0 {
		if offset+4 > len(data) {
			break
		}
		attrType := binary.BigEndian.Uint16(data[offset : offset+2])
		attrLen := int(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
		attrValue := data[offset+4 : offset+4+attrLen]
		if attrType == 0x0020 || attrType == 0x0001 {
			if len(attrValue) >= 4 {
				ipLen := int(attrValue[1])
				if ipLen == 4 && len(attrValue) >= 8 {
					port := int(binary.BigEndian.Uint16(attrValue[2:4]))
					if attrType == 0x0020 {
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

func (n *NATTraversalManager) determineNATType(resp *STUNResponse) NATType {
	return NATSymmetric
}

func (n *NATTraversalManager) InitiateHolePunching(targetID, targetAddr string, localPort int) (<-chan bool, error) {
	n.holeMu.Lock()
	defer n.holeMu.Unlock()
	if session, exists := n.activeHolePunching[targetID]; exists && session.State != "failed" {
		return session.ResponseChan, nil
	}
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
	go n.performHolePunching(session)
	return responseChan, nil
}

func (n *NATTraversalManager) performHolePunching(session *HolePunchSession) {
	maxAttempts := 5
	baseDelay := 500 * time.Millisecond
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if session.State == "established" {
			return
		}
		session.LastAttempt = time.Now()
		session.Attempts = attempt
		session.State = "punching"
		if err := n.sendPunchPacket(session.TargetAddr); err != nil {
			if attempt == maxAttempts {
				session.State = "failed"
				session.ResponseChan <- false
				close(session.ResponseChan)
				return
			}
			time.Sleep(baseDelay * time.Duration(attempt))
			continue
		}
		time.Sleep(2 * time.Second)
		if session.State == "established" {
			session.ResponseChan <- true
			return
		}
		if attempt == maxAttempts && n.relayServer != "" {
			session.State = "failed"
			session.ResponseChan <- false
		}
	}
}

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
	msg := []byte("PUNCH_REQ")
	_, err = conn.Write(msg)
	return err
}

func (n *NATTraversalManager) UseRelayFallback(targetID string) (net.Conn, error) {
	if n.relayServer == "" {
		return nil, fmt.Errorf("no relay server configured")
	}
	conn, err := net.DialTimeout("tcp", n.relayServer, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("relay connection failed: %w", err)
	}
	msg := fmt.Sprintf("RELAY_TO:%s\n", targetID)
	if _, err := conn.Write([]byte(msg)); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func (n *NATTraversalManager) KeepAlive(conn net.Conn) {
	ticker := time.NewTicker(n.keepAliveInterval)
	defer ticker.Stop()
	for range ticker.C {
		msg := []byte{0x00, 0x11, 0x00, 0x00}
		if _, err := conn.Write(msg); err != nil {
			return
		}
	}
}

func (n *NATTraversalManager) CloseHolePunching(targetID string) {
	n.holeMu.Lock()
	defer n.holeMu.Unlock()
	delete(n.activeHolePunching, targetID)
}

func (n *NATTraversalManager) GetExternalInfo() (net.IP, int, NATType) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.externalIP, n.externalPort, n.natType
}

func (n *NATTraversalManager) IsRelayRequired() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.natType == NATSymmetric
}

func (n *NATTraversalManager) EstablishDirectConnection(targetID, targetAddr string) (net.Conn, error) {
	ch, err := n.InitiateHolePunching(targetID, targetAddr, 0)
	if err != nil {
		return nil, err
	}
	select {
	case success := <-ch:
		if !success {
			return n.UseRelayFallback(targetID)
		}
	case <-time.After(10 * time.Second):
		return n.UseRelayFallback(targetID)
	}
	conn, err := net.Dial("udp", targetAddr)
	if err != nil {
		return n.UseRelayFallback(targetID)
	}
	return conn, nil
}

func (n *NATTraversalManager) StartLocalUPnP(port int) error {
	return nil
}
