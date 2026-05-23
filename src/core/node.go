// ============================================================================
// src/core/node.go - SovereignNode - Complete Node State
// ============================================================================
// Especificación:
// - Estado mutativo del nodo soberano (estructura SovereignNode)
// - Centraliza todos los componentes: identidad, DHT, router, storage, reputación
// - Métodos para inicio, parada y operaciones principales
// ============================================================================

package core

import (
	"net/http"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mamanga1/web5-mesh/src/config"
	"github.com/mamanga1/web5-mesh/src/crypto"
	"github.com/mamanga1/web5-mesh/src/dht"
	"github.com/mamanga1/web5-mesh/src/reputation"
	"github.com/mamanga1/web5-mesh/src/routing"
	"github.com/mamanga1/web5-mesh/src/storage"
)

// SovereignNode representa un nodo completo de la red MaIA Mesh
type SovereignNode struct {
	// Identidad y autenticación
	identity   *crypto.Identity
	did        string
	config     *config.NodeConfig

	// Componentes principales
	dhtEngine  *dht.KadEngine
	router     *routing.CryptoRouter
	storage    *storage.ReplicatedFS
	crdtStore  *storage.CRDTStore
	persistence *storage.PersistenceStore
	reputation *reputation.ReputationSystem

	// Estado de red
	startTime  time.Time
	stopCh     chan struct{}
	wg         sync.WaitGroup
	isRunning  bool

	// Métricas y monitoreo
	healthServer *HealthServer

	// Protección de concurrencia
	mu sync.RWMutex
}

// NewSovereignNode crea una nueva instancia de SovereignNode
func NewSovereignNode(cfg *config.NodeConfig) (*SovereignNode, error) {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	cfg.Validate()

	node := &SovereignNode{
		config:    cfg,
		startTime: time.Now(),
		stopCh:    make(chan struct{}),
		isRunning: false,
	}

	// Inicializar componentes en orden
	if err := node.initIdentity(); err != nil {
		return nil, fmt.Errorf("failed to initialize identity: %w", err)
	}

	if err := node.initStorage(); err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	if err := node.initDHT(); err != nil {
		return nil, fmt.Errorf("failed to initialize DHT: %w", err)
	}

	if err := node.initRouter(); err != nil {
		return nil, fmt.Errorf("failed to initialize router: %w", err)
	}

	if err := node.initReputation(); err != nil {
		return nil, fmt.Errorf("failed to initialize reputation: %w", err)
	}

	// Inicializar health server
	node.healthServer = NewHealthServer(node, cfg)

	return node, nil
}

// initIdentity inicializa la identidad criptográfica del nodo
func (n *SovereignNode) initIdentity() error {
	// Si se provee archivo de identidad, cargarlo
	if n.config.Crypto.IdentityFile != "" {
		// En producción, cargar desde archivo
		// Por ahora, generar nueva identidad
	}

	// Generar nueva identidad
	name := n.config.NodeName
	if name == "" {
		name = "MaIA-Node"
	}

	identity, err := crypto.NewIdentity(name)
	if err != nil {
		return err
	}

	n.identity = identity
	n.did = identity.GetDIDString()

	return nil
}

// initStorage inicializa el almacenamiento persistente
func (n *SovereignNode) initStorage() error {
	// Crear persistence store con BadgerDB
	storeOpts := storage.DefaultStoreOptions(n.config.GetDataPath("badger"))
	store, err := storage.NewPersistenceStore(storeOpts)
	if err != nil {
		return fmt.Errorf("failed to create persistence store: %w", err)
	}
	n.persistence = store

	// Crear CRDT store
	n.crdtStore = storage.NewCRDTStore()

	// Crear ReplicatedFS
	fsOpts := storage.DefaultReplicatedFSOptions(n.persistence, n.crdtStore)
	fs, err := storage.NewReplicatedFS(fsOpts)
	if err != nil {
		return fmt.Errorf("failed to create replicated FS: %w", err)
	}
	n.storage = fs

	return nil
}

// initDHT inicializa el motor Kademlia
func (n *SovereignNode) initDHT() error {
	// Generar NodeID desde DID
	nodeID := n.generateNodeIDFromDID()

	// Construir dirección local
	localAddr := fmt.Sprintf("0.0.0.0:%d", n.config.Network.UDPPort)

	// Crear configuración Kademlia
	kadConfig := dht.DefaultKadConfig()
	kadConfig.BootstrapNodes = n.config.Bootstrap.SeedNodes
	kadConfig.Alpha = 3
	kadConfig.BucketSize = 16
	kadConfig.LookupTimeout = n.config.Network.LookupTimeout
	kadConfig.HeartbeatInterval = n.config.Network.HeartbeatInterval

	// Crear motor
	engine := dht.NewKadEngine(nodeID, localAddr, kadConfig)
	n.dhtEngine = engine

	return nil
}

// initRouter inicializa el enrutador criptográfico
func (n *SovereignNode) initRouter() error {
	// Configurar relay si está habilitado
	relayAddr := ""
	if n.config.Network.NAT.RelayServer != "" {
		relayAddr = n.config.Network.NAT.RelayServer
	}

	// Crear router
	router := routing.NewCryptoRouter(n.dhtEngine, n.identity, relayAddr)
	n.router = router

	return nil
}

// initReputation inicializa el sistema de reputación
func (n *SovereignNode) initReputation() error {
	n.reputation = reputation.NewReputationSystem()
	return nil
}

// generateNodeIDFromDID genera un NodeID a partir del DID
func (n *SovereignNode) generateNodeIDFromDID() dht.NodeID {
	// Usar el hash del DID como NodeID
	didHash := n.identity.DID.Hash
	var nodeID dht.NodeID
	copy(nodeID[:], didHash[:32])
	return nodeID
}

// Start inicia todos los componentes del nodo
func (n *SovereignNode) Start() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.isRunning {
		return fmt.Errorf("node already running")
	}

	// Iniciar DHT
	if err := n.dhtEngine.Start(); err != nil {
		return fmt.Errorf("failed to start DHT: %w", err)
	}

	// Iniciar router
	if err := n.router.Start(); err != nil {
		return fmt.Errorf("failed to start router: %w", err)
	}

	// Iniciar sistema de reputación
	n.reputation.Start()

	n.isRunning = true

	// Iniciar bootstrap en background
	go n.Bootstrap()

	return nil
}

// bootstrap conecta el nodo a la red
func (n *SovereignNode) Bootstrap() {
	ctx, cancel := context.WithTimeout(context.Background(), n.config.Bootstrap.BootstrapTimeout)
	defer cancel()

	if err := n.dhtEngine.Bootstrap(ctx); err != nil {
		// Log error but continue
		_ = err
	}

	// Anunciar presencia en la red
	n.announcePresence()
}

// announcePresence anuncia el nodo a la red
func (n *SovereignNode) announcePresence() {
	// En producción, aquí se publicaría un anuncio en el DHT
}

// Stop detiene todos los componentes del nodo
func (n *SovereignNode) Stop() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.isRunning {
		return nil
	}

	close(n.stopCh)
	n.wg.Wait()

	// Detener componentes en orden inverso
	if n.router != nil {
		n.router.Stop()
	}
	if n.dhtEngine != nil {
		n.dhtEngine.Stop()
	}
	if n.reputation != nil {
		n.reputation.Stop()
	}
	if n.persistence != nil {
		n.persistence.Close()
	}
	if n.storage != nil {
		n.storage.Close()
	}

	n.isRunning = false
	return nil
}

// GetDID retorna el DID del nodo
func (n *SovereignNode) GetDID() string {
	return n.did
}

// GetIdentity retorna la identidad criptográfica
func (n *SovereignNode) GetIdentity() *crypto.Identity {
	return n.identity
}

// GetConfig retorna la configuración del nodo
func (n *SovereignNode) GetConfig() *config.NodeConfig {
	return n.config
}

// GetStartTime retorna la hora de inicio del nodo
func (n *SovereignNode) GetStartTime() time.Time {
	return n.startTime
}

// IsRunning retorna true si el nodo está en ejecución
func (n *SovereignNode) IsRunning() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.isRunning
}

// GetHealthHandler retorna el handler HTTP para health checks
func (n *SovereignNode) GetHealthHandler() http.HandlerFunc {
	return n.healthServer.Handler
}

// GetLivenessHandler retorna el handler para liveness probes
func (n *SovereignNode) GetLivenessHandler() http.HandlerFunc {
	return n.healthServer.LivenessCheck
}

// GetReadinessHandler retorna el handler para readiness probes
func (n *SovereignNode) GetReadinessHandler() http.HandlerFunc {
	return n.healthServer.ReadinessCheck
}

// GetDIDString retorna el DID como string (conveniencia)
func (n *SovereignNode) GetDIDString() string {
	return n.did
}

// GetNodeID retorna el NodeID para el DHT
func (n *SovereignNode) GetNodeID() dht.NodeID {
	return n.generateNodeIDFromDID()
}

// GetDHTHandler retorna el motor DHT (para acceso interno)
func (n *SovereignNode) GetDHTHandler() *dht.KadEngine {
	return n.dhtEngine
}

// GetRouter retorna el enrutador criptográfico
func (n *SovereignNode) GetRouter() *routing.CryptoRouter {
	return n.router
}

// GetStorage retorna el sistema de archivos replicado
func (n *SovereignNode) GetStorage() *storage.ReplicatedFS {
	return n.storage
}

// GetCRDTStore retorna el almacenamiento CRDT
func (n *SovereignNode) GetCRDTStore() *storage.CRDTStore {
	return n.crdtStore
}

// GetReputation retorna el sistema de reputación
func (n *SovereignNode) GetReputation() *reputation.ReputationSystem {
	return n.reputation
}

// Stats retorna estadísticas agregadas del nodo
func (n *SovereignNode) Stats() map[string]interface{} {
	stats := map[string]interface{}{
		"did":        n.did,
		"uptime":     time.Since(n.startTime).String(),
		"is_running": n.isRunning,
	}

	if n.dhtEngine != nil {
		stats["dht"] = n.dhtEngine.Stats()
	}
	if n.reputation != nil {
		stats["reputation"] = n.reputation.Stats()
	}
	if n.storage != nil {
		stats["storage"] = n.storage.GetStats()
	}
	if n.crdtStore != nil {
		stats["crdt"] = n.crdtStore.Stats()
	}
	if n.router != nil {
		conns, _ := n.router.ActiveConnections()
		stats["active_connections"] = len(conns)
	}

	return stats
}

// ResolveDomain resuelve un dominio .mesh a una conexión
func (n *SovereignNode) ResolveDomain(domain string) (*routing.Connection, error) {
	return n.router.ResolveDomain(domain)
}

// StoreData almacena datos en el DHT
func (n *SovereignNode) StoreData(key, value []byte) error {
	return n.dhtEngine.StoreValue(context.Background(), key, value)
}

// LookupData recupera datos del DHT
func (n *SovereignNode) LookupData(key []byte) ([]byte, error) {
	return n.dhtEngine.LookupValue(context.Background(), key)
}
