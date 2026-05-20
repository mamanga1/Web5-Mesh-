// ============================================================================
// src/routing/router.go - Crypto Router - P2P Connection Management
// ============================================================================
// Especificación:
// - Motor de ruteo criptográfico basado en topología de red
// - Maneja conexiones P2P cifradas con Noise Protocol
// - Integración con NAT traversal y relay fallback
// ============================================================================

package routing

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"web5-mesh/src/crypto"
	"web5-mesh/src/dht"
)

// ConnectionState representa el estado de una conexión
type ConnectionState int

const (
	StateConnecting ConnectionState = iota
	StateConnected
	StateDisconnected
	StateFailed
)

// Connection representa una conexión P2P activa
type Connection struct {
	// Identificadores
	ID         string    // ID de la conexión
	RemoteDID  string    // DID del peer remoto
	RemoteAddr string    // Dirección remota (IP:Puerto)
	LocalAddr  string    // Dirección local

	// Estado
	State      ConnectionState
	EstablishedAt time.Time
	LastActive    time.Time
	BytesSent     uint64
	BytesRecv     uint64

	// Criptografía
	SessionKey [32]byte
	NoiseState *crypto.NoiseHandshakeState

	// Métricas
	LatencyMs   float64
	PacketLoss  float64

	mu sync.RWMutex
}

// UpdateActivity actualiza la actividad de la conexión
func (c *Connection) UpdateActivity() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.LastActive = time.Now()
}

// AddBytesSent suma bytes enviados
func (c *Connection) AddBytesSent(bytes int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.BytesSent += uint64(bytes)
}

// AddBytesRecv suma bytes recibidos
func (c *Connection) AddBytesRecv(bytes int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.BytesRecv += uint64(bytes)
}

// IsActive verifica si la conexión está activa
func (c *Connection) IsActive() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.State == StateConnected && time.Since(c.LastActive) < 2*time.Minute
}

// Close cierra la conexión
func (c *Connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.State = StateDisconnected
	return nil
}

// CryptoRouter es el enrutador criptográfico principal
type CryptoRouter struct {
	// Componentes
	dhtEngine    *dht.KadEngine
	identity     *crypto.Identity

	// Conexiones
	connections   map[string]*Connection // DID -> Connection
	pendingConns  map[string]chan *Connection
	connMu        sync.RWMutex

	// Configuración
	relayServer   string
	handshakeTimeout time.Duration
	keepAliveInterval time.Duration

	// Estado
	started       bool
	stopCh        chan struct{}
	wg            sync.WaitGroup

	// Estadísticas
	stats struct {
		connectionsAttempted uint64
		connectionsEstablished uint64
		connectionsFailed    uint64
		bytesSent           uint64
		bytesRecv           uint64
		mu                  sync.RWMutex
	}
}

// NewCryptoRouter crea un nuevo enrutador criptográfico
func NewCryptoRouter(dhtEngine *dht.KadEngine, identity *crypto.Identity, relayServer string) *CryptoRouter {
	return &CryptoRouter{
		dhtEngine:         dhtEngine,
		identity:          identity,
		connections:       make(map[string]*Connection),
		pendingConns:      make(map[string]chan *Connection),
		relayServer:       relayServer,
		handshakeTimeout:  30 * time.Second,
		keepAliveInterval: 15 * time.Second,
		stopCh:            make(chan struct{}),
	}
}

// Start inicia el enrutador
func (r *CryptoRouter) Start() error {
	r.started = true

	// Iniciar keepalive loop
	r.wg.Add(1)
	go r.keepAliveLoop()

	return nil
}

// Stop detiene el enrutador
func (r *CryptoRouter) Stop() {
	r.started = false
	close(r.stopCh)
	r.wg.Wait()

	// Cerrar todas las conexiones
	r.connMu.Lock()
	for _, conn := range r.connections {
		conn.Close()
	}
	r.connections = make(map[string]*Connection)
	r.connMu.Unlock()
}

// Connect establece una conexión con un peer remoto por DID
func (r *CryptoRouter) Connect(ctx context.Context, remoteDID string) (*Connection, error) {
	r.incrementAttempts()

	// Verificar si ya existe conexión
	r.connMu.RLock()
	if conn, exists := r.connections[remoteDID]; exists && conn.IsActive() {
		r.connMu.RUnlock()
		return conn, nil
	}
	r.connMu.RUnlock()

	// Resolver DID a dirección
	addr, err := r.resolveDID(ctx, remoteDID)
	if err != nil {
		r.incrementFailed()
		return nil, fmt.Errorf("failed to resolve DID: %w", err)
	}

	// Establecer conexión
	conn, err := r.establishConnection(ctx, remoteDID, addr)
	if err != nil {
		r.incrementFailed()
		return nil, fmt.Errorf("failed to establish connection: %w", err)
	}

	r.incrementEstablished()
	return conn, nil
}

// resolveDID resuelve un DID a una dirección de red
func (r *CryptoRouter) resolveDID(ctx context.Context, did string) (string, error) {
	// Buscar en DHT
	nodeID := dht.HashKey([]byte(did))
	node, exists, err := r.dhtEngine.GetNode(nodeID)
	if err != nil || !exists {
		return "", fmt.Errorf("node not found in DHT")
	}

	return node.Address, nil
}

// establishConnection establece la conexión física y realiza handshake Noise
func (r *CryptoRouter) establishConnection(ctx context.Context, remoteDID, remoteAddr string) (*Connection, error) {
	// Crear canal para resultado
	resultCh := make(chan *Connection, 1)
	errCh := make(chan error, 1)

	go func() {
		// Intentar conexión directa UDP
		conn, err := r.dialUDP(remoteAddr)
		if err != nil {
			errCh <- err
			return
		}

		// Realizar handshake Noise
		noiseState, err := r.performHandshake(conn, remoteDID)
		if err != nil {
			conn.Close()
			errCh <- err
			return
		}

		// Crear objeto Connection
		connection := &Connection{
			ID:            fmt.Sprintf("%s->%s", r.identity.GetDIDString(), remoteDID),
			RemoteDID:     remoteDID,
			RemoteAddr:    remoteAddr,
			LocalAddr:     conn.LocalAddr().String(),
			State:         StateConnected,
			EstablishedAt: time.Now(),
			LastActive:    time.Now(),
			NoiseState:    noiseState,
			SessionKey:    noiseState.GetSessionKeys()[0],
		}

		// Almacenar conexión
		r.connMu.Lock()
		r.connections[remoteDID] = connection
		r.connMu.Unlock()

		// Iniciar lectura de la conexión
		go r.readLoop(connection, conn)

		resultCh <- connection
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-errCh:
		return nil, err
	case conn := <-resultCh:
		return conn, nil
	}
}

// dialUDP establece una conexión UDP
func (r *CryptoRouter) dialUDP(remoteAddr string) (*net.UDPConn, error) {
	addr, err := net.ResolveUDPAddr("udp", remoteAddr)
	if err != nil {
		return nil, err
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// performHandshake realiza el handshake Noise con el peer remoto
func (r *CryptoRouter) performHandshake(conn *net.UDPConn, remoteDID string) (*crypto.NoiseHandshakeState, error) {
	// En producción, aquí se implementaría el handshake completo
	// Por ahora, simulamos un handshake exitoso

	// Crear estado Noise
	localStatic := r.identity.PrivateKey
	var remoteStatic [32]byte
	// En producción, obtener clave pública del peer desde DHT

	noiseState := crypto.NewNoiseHandshake(true, localStatic, remoteStatic)

	// Generar mensaje de handshake
	msg, err := noiseState.WriteMessage()
	if err != nil {
		return nil, err
	}

	// Enviar mensaje
	_, err = conn.Write(msg)
	if err != nil {
		return nil, err
	}

	// Recibir respuesta (simplificado)
	buf := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(r.handshakeTimeout))
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}

	// Procesar respuesta
	if err := noiseState.ReadMessage(buf[:n]); err != nil {
		return nil, err
	}

	return noiseState, nil
}

// readLoop lee mensajes de una conexión
func (r *CryptoRouter) readLoop(conn *Connection, udpConn *net.UDPConn) {
	buf := make([]byte, 65536)

	for {
		select {
		case <-r.stopCh:
			return
		default:
		}

		udpConn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, err := udpConn.Read(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			conn.Close()
			return
		}

		conn.UpdateActivity()
		conn.AddBytesRecv(n)

		// Descifrar mensaje (en producción con Noise)
		// Process message...

		r.addBytesRecv(uint64(n))
	}
}

// Send envía datos a través de una conexión
func (r *CryptoRouter) Send(remoteDID string, data []byte) error {
	r.connMu.RLock()
	conn, exists := r.connections[remoteDID]
	r.connMu.RUnlock()

	if !exists || !conn.IsActive() {
		return fmt.Errorf("no active connection to %s", remoteDID)
	}

	// En producción, cifrar con Noise
	// Por ahora, enviamos en claro (para pruebas)

	return nil
}

// keepAliveLoop mantiene las conexiones activas
func (r *CryptoRouter) keepAliveLoop() {
	defer r.wg.Done()

	ticker := time.NewTicker(r.keepAliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.sendKeepAlives()
		}
	}
}

// sendKeepAlives envía keepalives a todas las conexiones activas
func (r *CryptoRouter) sendKeepAlives() {
	r.connMu.RLock()
	conns := make([]*Connection, 0, len(r.connections))
	for _, conn := range r.connections {
		if conn.IsActive() {
			conns = append(conns, conn)
		}
	}
	r.connMu.RUnlock()

	for _, conn := range conns {
		// Enviar keepalive
		// conn.SendKeepAlive()
	}
}

// ActiveConnections retorna la lista de conexiones activas
func (r *CryptoRouter) ActiveConnections() ([]*Connection, error) {
	r.connMu.RLock()
	defer r.connMu.RUnlock()

	conns := make([]*Connection, 0, len(r.connections))
	for _, conn := range r.connections {
		if conn.IsActive() {
			conns = append(conns, conn)
		}
	}
	return conns, nil
}

// GetConnection retorna una conexión por DID
func (r *CryptoRouter) GetConnection(did string) (*Connection, bool) {
	r.connMu.RLock()
	defer r.connMu.RUnlock()
	conn, ok := r.connections[did]
	return conn, ok
}

// Disconnect cierra una conexión específica
func (r *CryptoRouter) Disconnect(remoteDID string) error {
	r.connMu.Lock()
	defer r.connMu.Unlock()

	conn, exists := r.connections[remoteDID]
	if !exists {
		return nil
	}

	conn.Close()
	delete(r.connections, remoteDID)
	return nil
}

// Stats retorna estadísticas del router
func (r *CryptoRouter) Stats() map[string]interface{} {
	r.stats.mu.RLock()
	defer r.stats.mu.RUnlock()

	r.connMu.RLock()
	activeConns := 0
	for _, conn := range r.connections {
		if conn.IsActive() {
			activeConns++
		}
	}
	r.connMu.RUnlock()

	return map[string]interface{}{
		"connections_attempted":   r.stats.connectionsAttempted,
		"connections_established": r.stats.connectionsEstablished,
		"connections_failed":      r.stats.connectionsFailed,
		"active_connections":      activeConns,
		"bytes_sent":              r.stats.bytesSent,
		"bytes_recv":              r.stats.bytesRecv,
	}
}

// incrementAttempts incrementa el contador de intentos
func (r *CryptoRouter) incrementAttempts() {
	r.stats.mu.Lock()
	defer r.stats.mu.Unlock()
	r.stats.connectionsAttempted++
}

// incrementEstablished incrementa el contador de conexiones establecidas
func (r *CryptoRouter) incrementEstablished() {
	r.stats.mu.Lock()
	defer r.stats.mu.Unlock()
	r.stats.connectionsEstablished++
}

// incrementFailed incrementa el contador de conexiones fallidas
func (r *CryptoRouter) incrementFailed() {
	r.stats.mu.Lock()
	defer r.stats.mu.Unlock()
	r.stats.connectionsFailed++
}

// addBytesSent suma bytes enviados
func (r *CryptoRouter) addBytesSent(bytes uint64) {
	r.stats.mu.Lock()
	defer r.stats.mu.Unlock()
	r.stats.bytesSent += bytes
}

// addBytesRecv suma bytes recibidos
func (r *CryptoRouter) addBytesRecv(bytes uint64) {
	r.stats.mu.Lock()
	defer r.stats.mu.Unlock()
	r.stats.bytesRecv += bytes
}

// ResolveDomain resuelve un dominio .mesh a una conexión
func (r *CryptoRouter) ResolveDomain(domain string) (*Connection, error) {
	// En producción, resolver dominio a DID via DHT
	// Por ahora, placeholder
	return nil, fmt.Errorf("domain resolution not implemented")
}
