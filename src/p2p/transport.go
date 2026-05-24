package p2p

import (
	"fmt"
	"net"
	"time"
)

// TransportUDP maneja la comunicación UDP subyacente
type TransportUDP struct {
	conn     *net.UDPConn
	port     int
	readTimeout  time.Duration
	writeTimeout time.Duration
	localAddr    *net.UDPAddr
}

// NewTransportUDP crea un nuevo transporte UDP
func NewTransportUDP(port int, readTimeout, writeTimeout time.Duration) (*TransportUDP, error) {
	if readTimeout == 0 {
		readTimeout = 10 * time.Second
	}
	if writeTimeout == 0 {
		writeTimeout = 5 * time.Second
	}

	addr := &net.UDPAddr{IP: net.IPv4(0, 0, 0, 0), Port: port}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen UDP on port %d: %w", port, err)
	}

	return &TransportUDP{
		conn:         conn,
		port:         port,
		readTimeout:  readTimeout,
		writeTimeout: writeTimeout,
		localAddr:    conn.LocalAddr().(*net.UDPAddr),
	}, nil
}

// LocalAddr devuelve la dirección local
func (t *TransportUDP) LocalAddr() *net.UDPAddr {
	return t.localAddr
}

// ReadFrom lee un mensaje UDP
func (t *TransportUDP) ReadFrom() ([]byte, *net.UDPAddr, error) {
	buf := make([]byte, 65536)
	t.conn.SetReadDeadline(time.Now().Add(t.readTimeout))
	n, addr, err := t.conn.ReadFromUDP(buf)
	if err != nil {
		return nil, nil, err
	}
	return buf[:n], addr, nil
}

// WriteTo envía un mensaje UDP a una dirección
func (t *TransportUDP) WriteTo(data []byte, addr *net.UDPAddr) error {
	t.conn.SetWriteDeadline(time.Now().Add(t.writeTimeout))
	_, err := t.conn.WriteToUDP(data, addr)
	return err
}

// Close cierra la conexión
func (t *TransportUDP) Close() error {
	return t.conn.Close()
}
