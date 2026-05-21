// ============================================================================
// src/routing/router.go - Crypto Router - P2P Connection Management
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

type ConnectionState int

const (
	StateConnecting ConnectionState = iota
	StateConnected
	StateDisconnected
	StateFailed
)

type Connection struct {
	ID            string
	RemoteDID     string
	RemoteAddr    string
	LocalAddr     string
	State         ConnectionState
	EstablishedAt time.Time
	LastActive    time.Time
	BytesSent     uint64
	BytesRecv     uint64
	SessionKey    [32]byte
	NoiseState    *crypto.NoiseHandshakeState
	LatencyMs     float64
	PacketLoss    float64
	mu            sync.RWMutex
}

func (c *Connection) UpdateActivity() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.LastActive = time.Now()
}

func (c *Connection) AddBytesSent(bytes int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.BytesSent += uint64(bytes)
}

func (c *Connection) AddBytesRecv(bytes int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.BytesRecv += uint64(bytes)
}

func (c *Connection) IsActive() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.State == StateConnected && time.Since(c.LastActive) < 2*time.Minute
}

func (c *Connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.State = StateDisconnected
	return nil
}

type CryptoRouter struct {
	dhtEngine         *dht.KadEngine
	identity          *crypto.Identity
	connections       map[string]*Connection
	pendingConns      map[string]chan *Connection
	relayServer       string
	handshakeTimeout  time.Duration
	keepAliveInterval time.Duration
	started           bool
	stopCh            chan struct{}
	wg                sync.WaitGroup
	connMu            sync.RWMutex
	stats             struct {
		connectionsAttempted   uint64
		connectionsEstablished uint64
		connectionsFailed      uint64
		bytesSent              uint64
		bytesRecv              uint64
		mu                     sync.RWMutex
	}
}

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

func (r *CryptoRouter) Start() error {
	r.started = true
	r.wg.Add(1)
	go r.keepAliveLoop()
	return nil
}

func (r *CryptoRouter) Stop() {
	r.started = false
	close(r.stopCh)
	r.wg.Wait()
	r.connMu.Lock()
	for _, conn := range r.connections {
		conn.Close()
	}
	r.connections = make(map[string]*Connection)
	r.connMu.Unlock()
}

func (r *CryptoRouter) Connect(ctx context.Context, remoteDID string) (*Connection, error) {
	r.incrementAttempts()
	r.connMu.RLock()
	if conn, exists := r.connections[remoteDID]; exists && conn.IsActive() {
		r.connMu.RUnlock()
		return conn, nil
	}
	r.connMu.RUnlock()

	addr, err := r.resolveDID(ctx, remoteDID)
	if err != nil {
		r.incrementFailed()
		return nil, fmt.Errorf("failed to resolve DID: %w", err)
	}

	conn, err := r.establishConnection(ctx, remoteDID, addr)
	if err != nil {
		r.incrementFailed()
		return nil, fmt.Errorf("failed to establish connection: %w", err)
	}

	r.incrementEstablished()
	return conn, nil
}

func (r *CryptoRouter) resolveDID(ctx context.Context, did string) (string, error) {
	nodeID := dht.HashKey([]byte(did))
	node, exists, err := r.dhtEngine.GetNode(nodeID)
	if err != nil || !exists {
		return "", fmt.Errorf("node not found in DHT")
	}
	return node.Address, nil
}

func (r *CryptoRouter) establishConnection(ctx context.Context, remoteDID, remoteAddr string) (*Connection, error) {
	resultCh := make(chan *Connection, 1)
	errCh := make(chan error, 1)

	go func() {
		udpConn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 4242})
		if err != nil {
			errCh <- err
			return
		}
		defer udpConn.Close()

		var localStatic [32]byte
		var remoteStatic [32]byte
		noiseState := crypto.NewNoiseHandshake(true, localStatic, remoteStatic)

		msg, err := noiseState.WriteMessage()
		if err != nil {
			errCh <- err
			return
		}
		_ = msg // evitar warning de variable no usada

		connection := &Connection{
			ID:            fmt.Sprintf("%s->%s", r.identity.GetDIDString(), remoteDID),
			RemoteDID:     remoteDID,
			RemoteAddr:    remoteAddr,
			LocalAddr:     udpConn.LocalAddr().String(),
			State:         StateConnected,
			EstablishedAt: time.Now(),
			LastActive:    time.Now(),
			NoiseState:    noiseState,
		}
		r.connMu.Lock()
		r.connections[remoteDID] = connection
		r.connMu.Unlock()
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

func (r *CryptoRouter) sendKeepAlives() {
	r.connMu.RLock()
	conns := make([]*Connection, 0, len(r.connections))
	for _, conn := range r.connections {
		if conn.IsActive() {
			conns = append(conns, conn)
		}
	}
	r.connMu.RUnlock()
	for range conns {
		// Enviar keepalive
	}
}

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

func (r *CryptoRouter) GetConnection(did string) (*Connection, bool) {
	r.connMu.RLock()
	defer r.connMu.RUnlock()
	conn, ok := r.connections[did]
	return conn, ok
}

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

func (r *CryptoRouter) incrementAttempts() {
	r.stats.mu.Lock()
	defer r.stats.mu.Unlock()
	r.stats.connectionsAttempted++
}

func (r *CryptoRouter) incrementEstablished() {
	r.stats.mu.Lock()
	defer r.stats.mu.Unlock()
	r.stats.connectionsEstablished++
}

func (r *CryptoRouter) incrementFailed() {
	r.stats.mu.Lock()
	defer r.stats.mu.Unlock()
	r.stats.connectionsFailed++
}

func (r *CryptoRouter) addBytesSent(bytes uint64) {
	r.stats.mu.Lock()
	defer r.stats.mu.Unlock()
	r.stats.bytesSent += bytes
}

func (r *CryptoRouter) addBytesRecv(bytes uint64) {
	r.stats.mu.Lock()
	defer r.stats.mu.Unlock()
	r.stats.bytesRecv += bytes
}

func (r *CryptoRouter) ResolveDomain(domain string) (*Connection, error) {
	return nil, fmt.Errorf("domain resolution not implemented")
}
