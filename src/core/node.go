package core

import (
        "fmt"
        "log"
        "net"
        "net/http"
        "sync"
        "time"

        "github.com/mamanga1/web5-mesh/src/config"
        "github.com/mamanga1/web5-mesh/src/crypto"
        "github.com/mamanga1/web5-mesh/src/p2p"
        "github.com/mamanga1/web5-mesh/src/storage"
)

type SovereignNode struct {
        identity    *crypto.Identity
        did         string
        config      *config.NodeConfig
        storage     *storage.ReplicatedFS
        crdtStore   *storage.CRDTStore
        persistence *storage.PersistenceStore
        startTime   time.Time
        stopCh      chan struct{}
        wg          sync.WaitGroup
        isRunning   bool
        mu          sync.RWMutex

        // P2P
        p2pTransport *p2p.TransportUDP
        kademlia     *p2p.Kademlia
}

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

        if err := node.initIdentity(); err != nil {
                return nil, fmt.Errorf("failed to initialize identity: %w", err)
        }

        if err := node.initStorage(); err != nil {
                return nil, fmt.Errorf("failed to initialize storage: %w", err)
        }

        // Iniciar P2P
        node.initP2P()

        return node, nil
}

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

func (n *SovereignNode) Start() error {
        n.mu.Lock()
        defer n.mu.Unlock()

        if n.isRunning {
                return fmt.Errorf("node already running")
        }

        n.isRunning = true
        log.Printf("[NODE] Started successfully, DID: %s", n.did)
        return nil
}

func (n *SovereignNode) Stop() error {
        n.mu.Lock()
        defer n.mu.Unlock()

        if !n.isRunning {
                return nil
        }

        close(n.stopCh)
        n.wg.Wait()

        if n.kademlia != nil {
                n.kademlia.Stop()
        }
        if n.p2pTransport != nil {
                n.p2pTransport.Close()
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

func (n *SovereignNode) Bootstrap() {
        log.Printf("[NODE] Bootstrap completed")
}

func (n *SovereignNode) GetDID() string { return n.did }
func (n *SovereignNode) IsRunning() bool {
        n.mu.RLock()
        defer n.mu.RUnlock()
        return n.isRunning
}

func (n *SovereignNode) GetHealthHandler() http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                w.Header().Set("Content-Type", "application/json")
                fmt.Fprintf(w, `{"status":"healthy","did":"%s"}`, n.did)
        }
}

func (n *SovereignNode) GetLivenessHandler() http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                w.WriteHeader(http.StatusOK)
                w.Write([]byte(`{"status":"alive"}`))
        }
}

func (n *SovereignNode) GetReadinessHandler() http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                w.WriteHeader(http.StatusOK)
                w.Write([]byte(`{"status":"ready"}`))
        }
}

func (n *SovereignNode) Stats() map[string]interface{} {
        return map[string]interface{}{
                "did":        n.did,
                "is_running": n.isRunning,
                "uptime":     time.Since(n.startTime).String(),
        }
}

func (n *SovereignNode) initP2P() {
        transport, err := p2p.NewTransportUDP(n.config.Network.UDPPort, 10*time.Second, 5*time.Second)
        if err != nil {
                log.Printf("[P2P] Failed to create transport: %v", err)
                return
        }
        n.p2pTransport = transport
        n.kademlia = p2p.NewKademlia(transport)
        n.kademlia.Start()
        log.Printf("[P2P] Kademlia started with Node ID: %x", n.kademlia.LocalID())

        // Iniciar handshake con el faro
        handshake := p2p.NewHandshake(n.p2pTransport, n.identity)
        seedAddr, err := net.ResolveUDPAddr("udp", "192.168.1.110:4245")
        if err == nil {
                go handshake.Initiate(seedAddr)
                log.Printf("[P2P] Handshake initiated with faro")
        }

        // Descubrir IP pública con STUN
        nat := p2p.NewNATTraversal(n.p2pTransport, "stun.l.google.com:19302")
        if err := nat.DiscoverPublicIP(); err != nil {
                log.Printf("[NAT] Failed to discover public IP: %v", err)
        } else {
                log.Printf("[NAT] Public IP: %s:%d", nat.PublicIP.String(), nat.PublicPort)
                
                // Registrar en el faro
                holepuncher := p2p.NewHolePuncher(n.p2pTransport, "192.168.1.110:4245")
                holepuncher.SetNodeInfo(n.did, nat.PublicIP.String(), nat.PublicPort)
                holepuncher.RegisterPublicIP(nat.PublicIP.String(), nat.PublicPort)
        }

        // BOOTSTRAP: conectar al faro (TV Box)
        seeds := []string{"192.168.1.110:4245"}
        bootstrapper := p2p.NewBootstrapper(n.p2pTransport, n.kademlia, seeds)
        bootstrapper.Start()
        go bootstrapper.BootstrapLoop(5 * time.Minute)
}
