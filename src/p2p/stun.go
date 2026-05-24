package p2p

import (
	"fmt"
	"net"
	"time"

	"github.com/pion/stun"
)

// STUNClient maneja la comunicación con servidores STUN
type STUNClient struct {
	serverAddr string
	timeout    time.Duration
}

// NewSTUNClient crea un nuevo cliente STUN
func NewSTUNClient(serverAddr string, timeout time.Duration) *STUNClient {
	if serverAddr == "" {
		serverAddr = "stun.l.google.com:19302"
	}
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	return &STUNClient{
		serverAddr: serverAddr,
		timeout:    timeout,
	}
}

// ExternalIP obtiene la IP pública consultando al servidor STUN
func (c *STUNClient) ExternalIP() (net.IP, error) {
	// Crear conexión UDP local para escuchar
	laddr, err := net.ResolveUDPAddr("udp", "0.0.0.0:0")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve local address: %w", err)
	}

	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen UDP: %w", err)
	}
	defer conn.Close()

	// Configurar timeout
	if err := conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return nil, fmt.Errorf("failed to set deadline: %w", err)
	}

	// Crear mensaje STUN de binding request
	message := stun.MustBuild(stun.TransactionID, stun.BindingRequest)

	// Resolver dirección del servidor STUN
	serverAddr, err := net.ResolveUDPAddr("udp", c.serverAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve STUN server: %w", err)
	}

	// Enviar request
	if _, err := conn.WriteToUDP(message.Raw, serverAddr); err != nil {
		return nil, fmt.Errorf("failed to send STUN request: %w", err)
	}

	// Leer respuesta
	buf := make([]byte, 1024)
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read STUN response: %w", err)
	}

	// Parsear respuesta
	resp := &stun.Message{Raw: buf[:n]}
	if err := resp.Decode(); err != nil {
		return nil, fmt.Errorf("failed to decode STUN response: %w", err)
	}

	// Obtener atributo XOR-MAPPED-ADDRESS
	var xorAddr stun.XORMappedAddress
	if err := xorAddr.GetFrom(resp); err != nil {
		// Intentar con MAPPED-ADDRESS (fallback)
		var mappedAddr stun.MappedAddress
		if err := mappedAddr.GetFrom(resp); err != nil {
			return nil, fmt.Errorf("failed to get address from STUN response: %w", err)
		}
		return mappedAddr.IP, nil
	}

	return xorAddr.IP, nil
}
