// ============================================================================
// src/core/node.go - SovereignNode - Complete Node State
// ============================================================================

package core

import (
        "net"
        "net/http"
        "context"
        "fmt"
        "log"
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

        // UDP P2P
        udpConn     *net.UDPConn

        // Protección de concurrencia
        mu sync.RWMutex
    peers       map[string]time.Time
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

        // Iniciar servidor UDP
        if err := node.startUDPListener(); err != nil {
                log.Printf("[WARN] UDP listener failed: %v", err)
        }

        return node, nil
}

// startUDPListener inicia el servidor UDP real para P2P
func (n *SovereignNode) startUDPListener() error {
        if n.udpConn != nil {
                return nil
        }

        addr := fmt.Sprintf("0.0.0.0:%d", n.config.Network.UDPPort)
        udpAddr, err := net.ResolveUDPAddr("udp", addr)
        if err != nil {
                return fmt.Errorf("resolve UDP failed: %w", err)
        }

        conn, err := net.ListenUDP("udp", udpAddr)
        if err != nil {
                return fmt.Errorf("listen UDP failed: %w", err)
        }

        n.udpConn = conn
        log.Printf("[UDP] Listening on %s", addr)

        go n.udpReceiver()
        go n.udpBroadcaster()
        return nil
}

func (n *SovereignNode) udpReceiver() {
        buf := make([]byte, 1024)
        for {
                if n.udpConn == nil || !n.isRunning {
                        return
                }
                n.udpConn.SetReadDeadline(time.Now().Add(5 * time.Second))
                nb, udpAddr, err := n.udpConn.ReadFromUDP(buf)
                if err != nil {
                        continue
                }
                log.Printf("[UDP] Received from %s (%d bytes)", udpAddr.IP.String(), nb)
                n.addPeer(udpAddr.IP.String())
        }
}

func (n *SovereignNode) udpBroadcaster() {
        if n.udpConn == nil || !n.isRunning {
                return
        }
        bcast := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 255), Port: n.config.Network.UDPPort}
        ticker := time.NewTicker(3 * time.Second)
        defer ticker.Stop()

        for range ticker.C {
                if n.udpConn == nil || !n.isRunning {
                        return
                }
                n.udpConn.WriteToUDP([]byte("ping"), bcast)
        }
}

// initIdentity inicializa la identidad criptográfica del nodo
func (n *SovereignNode) initIdentity() error {
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
        storeOpts := storage.DefaultStoreOptions(n.config.GetDataPath("badger"))
        store, err := storage.NewPersistenceStore(storeOpts)
        if err != nil {
                return fmt.Errorf("failed to create persistence store: %w", err)
        }
        n.persistence = store

        n.crdtStore = storage.NewCRDTStore()

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
        nodeID := n.generateNodeIDFromDID()
        localAddr := fmt.Sprintf("0.0.0.0:%d", n.config.Network.UDPPort)

        kadConfig := dht.DefaultKadConfig()
        kadConfig.BootstrapNodes = n.config.Bootstrap.SeedNodes
        kadConfig.Alpha = 3
        kadConfig.BucketSize = 16
        kadConfig.LookupTimeout = n.config.Network.LookupTimeout
        kadConfig.HeartbeatInterval = n.config.Network.HeartbeatInterval

        engine := dht.NewKadEngine(nodeID, localAddr, kadConfig)
        n.dhtEngine = engine

        return nil
}

// initRouter inicializa el enrutador criptográfico
func (n *SovereignNode) initRouter() error {
        relayAddr := ""
        if n.config.Network.NAT.RelayServer != "" {
                relayAddr = n.config.Network.NAT.RelayServer
        }

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

        if err := n.dhtEngine.Start(); err != nil {
                return fmt.Errorf("failed to start DHT: %w", err)
        }

        if err := n.router.Start(); err != nil {
                return fmt.Errorf("failed to start router: %w", err)
        }

        n.reputation.Start()
        n.isRunning = true

        go n.Bootstrap()

        return nil
}

// bootstrap conecta el nodo a la red
func (n *SovereignNode) Bootstrap() {
        ctx, cancel := context.WithTimeout(context.Background(), n.config.Bootstrap.BootstrapTimeout)
        defer cancel()

        if err := n.dhtEngine.Bootstrap(ctx); err != nil {
                _ = err
        }

        n.announcePresence()
}

// announcePresence anuncia el nodo a la red
func (n *SovereignNode) announcePresence() {}

// Stop detiene todos los componentes del nodo
func (n *SovereignNode) Stop() error {
        n.mu.Lock()
        defer n.mu.Unlock()

        if !n.isRunning {
                return nil
        }

        close(n.stopCh)
        n.wg.Wait()

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
        if n.udpConn != nil {
                n.udpConn.Close()
        }

        n.isRunning = false
        return nil
}

// GetDID retorna el DID del nodo
func (n *SovereignNode) GetDID() string { return n.did }

// GetIdentity retorna la identidad criptográfica
func (n *SovereignNode) GetIdentity() *crypto.Identity { return n.identity }

// GetConfig retorna la configuración del nodo
func (n *SovereignNode) GetConfig() *config.NodeConfig { return n.config }

// GetStartTime retorna la hora de inicio del nodo
func (n *SovereignNode) GetStartTime() time.Time { return n.startTime }

// IsRunning retorna true si el nodo está en ejecución
func (n *SovereignNode) IsRunning() bool {
        n.mu.RLock()
        defer n.mu.RUnlock()
        return n.isRunning
}

// GetHealthHandler retorna el handler HTTP para health checks
func (n *SovereignNode) GetHealthHandler() http.HandlerFunc {
        return n.healthServer.GetHealthHandler()
}

// GetLivenessHandler retorna el handler para liveness probes
func (n *SovereignNode) GetLivenessHandler() http.HandlerFunc {
        return n.healthServer.GetLivenessHandler()
}

// GetReadinessHandler retorna el handler para readiness probes
func (n *SovereignNode) GetReadinessHandler() http.HandlerFunc {
        return n.healthServer.GetReadinessHandler()
}

// GetDIDString retorna el DID como string
func (n *SovereignNode) GetDIDString() string { return n.did }

// GetNodeID retorna el NodeID para el DHT
func (n *SovereignNode) GetNodeID() dht.NodeID { return n.generateNodeIDFromDID() }

// GetDHTHandler retorna el motor DHT
func (n *SovereignNode) GetDHTHandler() *dht.KadEngine { return n.dhtEngine }

// GetRouter retorna el enrutador criptográfico
func (n *SovereignNode) GetRouter() *routing.CryptoRouter { return n.router }

// GetStorage retorna el sistema de archivos replicado
func (n *SovereignNode) GetStorage() *storage.ReplicatedFS { return n.storage }

// GetCRDTStore retorna el almacenamiento CRDT
func (n *SovereignNode) GetCRDTStore() *storage.CRDTStore { return n.crdtStore }

// GetReputation retorna el sistema de reputación
func (n *SovereignNode) GetReputation() *reputation.ReputationSystem { return n.reputation }

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

// PeerInfo almacena información de un peer descubierto
type PeerInfo struct {
        Address  string
        LastSeen time.Time
}

// GetActivePeers retorna la cantidad de peers activos

// addPeer agrega o actualiza un peer

// GetActivePeers retorna la cantidad de peers activos (últimos 30 segundos)


// GetActivePeers retorna la cantidad de peers activos (últimos 30 segundos)

// addPeer agrega o actualiza un peer

// GetActivePeers retorna la cantidad de peers activos (últimos 30 segundos)
func (n *SovereignNode) GetActivePeers() int {
    n.mu.Lock()
    defer n.mu.Unlock()
    
    if n.peers == nil {
        return 0
    }
    
    now := time.Now()
    count := 0
    for _, lastSeen := range n.peers {
        if now.Sub(lastSeen) < 30*time.Second {
            count++
        }
    }
    return count
}

// addPeer agrega o actualiza un peer
func (n *SovereignNode) addPeer(ip string) {
    n.mu.Lock()
    defer n.mu.Unlock()
    
    if n.peers == nil {
        n.peers = make(map[string]time.Time)
    }
    
    if _, exists := n.peers[ip]; !exists {
        log.Printf("[PEER] New peer discovered: %s", ip)
    }
    n.peers[ip] = time.Now()
}
