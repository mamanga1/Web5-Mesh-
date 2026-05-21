// ============================================================================
// src/config/config.go - Node Configuration & Flags Parser
// ============================================================================
// Especificación:
// - Flags de línea de comandos para configuración del nodo
// - Modos de operación: full, relay, bunker
// - Configuración de puertos UDP y límites de memoria
// ============================================================================

// ============================================================================
// src/config/config.go - Node Configuration & Flags Parser
// ============================================================================

package config

import (
	"flag"
	"fmt"
	"runtime"
	"strings"
	"time"
)

type NodeMode string

const (
	ModeFull   NodeMode = "full"
	ModeRelay  NodeMode = "relay"
	ModeBunker NodeMode = "bunker"
	ModeLight  NodeMode = "light"
)

type HardwareProfile string

const (
	HardwareXeon    HardwareProfile = "xeon"
	HardwareDesktop HardwareProfile = "desktop"
	HardwareMobile  HardwareProfile = "mobile"
	HardwareTVBox   HardwareProfile = "tvbox"
	HardwareUnknown HardwareProfile = "unknown"
)

type NetworkConfig struct {
	UDPPort           int
	TCPPort           int
	MaxConnections    int
	HandshakeTimeout  time.Duration
	HeartbeatInterval time.Duration
	LookupTimeout     time.Duration
	NAT               struct {
		AutoDiscover bool
		STUNServers  []string
		EnableUPnP   bool
		RelayServer  string
	}
}

type StorageConfig struct {
	DataDir           string
	ReplicationFactor int
	MaxBlockSize      int
	CRDTCacheSize     int
	BadgerOptions     struct {
		MemTableSize int64
		ValueLogSize int64
		NumMemtables int
		Compression  bool
	}
}

type CryptoConfig struct {
	IdentityFile        string
	PoWDifficulty       int
	EnableNoise         bool
	EnableChaCha20      bool
	SessionKeyLifetime  time.Duration
}

type PerformanceConfig struct {
	MaxGoroutines   int
	MemoryLimitMB   int
	GCPercent       int
	EnableProfiling bool
	EnableMetrics   bool
	MetricsPort     int
}

type BootstrapConfig struct {
	SeedNodes             []string
	AutoDiscover          bool
	MaxBootstrapAttempts  int
	BootstrapTimeout      time.Duration
}

type NodeConfig struct {
	NodeID      string
	NodeName    string
	Mode        NodeMode
	Hardware    HardwareProfile
	HardwareAuto bool
	Network     NetworkConfig
	Storage     StorageConfig
	Crypto      CryptoConfig
	Performance PerformanceConfig
	Bootstrap   BootstrapConfig
	LogLevel    string
	Verbose     bool
	ConfigFile  string
}

func DefaultConfig() *NodeConfig {
	cfg := &NodeConfig{
		NodeName:    "MaIA-Mesh-Node",
		Mode:        ModeFull,
		Hardware:    HardwareUnknown,
		HardwareAuto: true,
		LogLevel:    "info",
		Verbose:     false,
	}
	cfg.Network.UDPPort = 4242
	cfg.Network.TCPPort = 4243
	cfg.Network.MaxConnections = 1024
	cfg.Network.HandshakeTimeout = 30 * time.Second
	cfg.Network.HeartbeatInterval = 15 * time.Second
	cfg.Network.LookupTimeout = 3 * time.Second
	cfg.Network.NAT.AutoDiscover = true
	cfg.Network.NAT.STUNServers = []string{
		"stun.l.google.com:19302",
		"stun1.l.google.com:19302",
		"stun2.l.google.com:19302",
	}
	cfg.Network.NAT.EnableUPnP = true
	cfg.Network.NAT.RelayServer = ""

	cfg.Storage.DataDir = "./data"
	cfg.Storage.ReplicationFactor = 3
	cfg.Storage.MaxBlockSize = 1024 * 1024
	cfg.Storage.CRDTCacheSize = 100
	cfg.Storage.BadgerOptions.MemTableSize = 64 * 1024 * 1024
	cfg.Storage.BadgerOptions.ValueLogSize = 1024 * 1024 * 1024
	cfg.Storage.BadgerOptions.NumMemtables = 2
	cfg.Storage.BadgerOptions.Compression = true

	cfg.Crypto.PoWDifficulty = 16
	cfg.Crypto.EnableNoise = true
	cfg.Crypto.EnableChaCha20 = true
	cfg.Crypto.SessionKeyLifetime = 24 * time.Hour

	cfg.Performance.MaxGoroutines = 1000
	cfg.Performance.MemoryLimitMB = 0
	cfg.Performance.GCPercent = 100
	cfg.Performance.EnableProfiling = false
	cfg.Performance.EnableMetrics = true
	cfg.Performance.MetricsPort = 2112

	cfg.Bootstrap.SeedNodes = []string{}
	cfg.Bootstrap.AutoDiscover = true
	cfg.Bootstrap.MaxBootstrapAttempts = 5
	cfg.Bootstrap.BootstrapTimeout = 10 * time.Second

	return cfg
}

func (c *NodeConfig) ParseFlags() {
	modeStr := flag.String("mode", string(ModeFull), "Node mode: full, relay, bunker, light")
	udpPort := flag.Int("udp-port", c.Network.UDPPort, "UDP port for DHT")
	tcpPort := flag.Int("tcp-port", c.Network.TCPPort, "TCP port for relay")
	maxConns := flag.Int("max-connections", c.Network.MaxConnections, "Maximum concurrent connections")
	natAuto := flag.Bool("nat-auto", c.Network.NAT.AutoDiscover, "Auto-detect NAT using STUN")
	relayServer := flag.String("relay", c.Network.NAT.RelayServer, "Relay server address for fallback")
	dataDir := flag.String("data-dir", c.Storage.DataDir, "Data directory for storage")
	replication := flag.Int("replication", c.Storage.ReplicationFactor, "Replication factor (min 3)")
	identityFile := flag.String("identity", c.Crypto.IdentityFile, "Identity file path")
	powDifficulty := flag.Int("pow-difficulty", c.Crypto.PoWDifficulty, "Proof of Work difficulty (bits)")
	maxGoroutines := flag.Int("max-goroutines", c.Performance.MaxGoroutines, "Maximum concurrent goroutines")
	memoryLimit := flag.Int("memory-limit", c.Performance.MemoryLimitMB, "Memory limit in MB (0 = unlimited)")
	metricsPort := flag.Int("metrics-port", c.Performance.MetricsPort, "Prometheus metrics port")
	seedNodes := flag.String("seeds", "", "Comma-separated list of seed node DIDs/IPs")
	logLevel := flag.String("log-level", c.LogLevel, "Log level: debug, info, warn, error")
	verbose := flag.Bool("verbose", c.Verbose, "Verbose output")

	flag.Parse()

	c.Mode = NodeMode(*modeStr)
	c.Network.UDPPort = *udpPort
	c.Network.TCPPort = *tcpPort
	c.Network.MaxConnections = *maxConns
	c.Network.NAT.AutoDiscover = *natAuto
	c.Network.NAT.RelayServer = *relayServer
	c.Storage.DataDir = *dataDir
	c.Storage.ReplicationFactor = *replication
	c.Crypto.IdentityFile = *identityFile
	c.Crypto.PoWDifficulty = *powDifficulty
	c.Performance.MaxGoroutines = *maxGoroutines
	c.Performance.MemoryLimitMB = *memoryLimit
	c.Performance.MetricsPort = *metricsPort
	c.LogLevel = *logLevel
	c.Verbose = *verbose

	if *seedNodes != "" {
		c.Bootstrap.SeedNodes = strings.Split(*seedNodes, ",")
	}

	if c.HardwareAuto {
		c.detectHardware()
	}
}

func (c *NodeConfig) detectHardware() {
	numCPU := runtime.NumCPU()
	totalMem := getTotalMemory()

	switch {
	case numCPU >= 16 && totalMem >= 64:
		c.Hardware = HardwareXeon
	case numCPU >= 4 && totalMem >= 8:
		c.Hardware = HardwareDesktop
	case numCPU >= 4 && totalMem >= 4:
		c.Hardware = HardwareMobile
	case numCPU >= 2 && totalMem >= 1:
		c.Hardware = HardwareTVBox
	default:
		c.Hardware = HardwareUnknown
	}
}

func (c *NodeConfig) Validate() error {
	switch c.Mode {
	case ModeFull, ModeRelay, ModeBunker, ModeLight:
	default:
		c.Mode = ModeFull
	}
	if c.Network.UDPPort < 1024 || c.Network.UDPPort > 65535 {
		c.Network.UDPPort = 4242
	}
	if c.Network.TCPPort < 1024 || c.Network.TCPPort > 65535 {
		c.Network.TCPPort = 4243
	}
	if c.Storage.ReplicationFactor < 1 {
		c.Storage.ReplicationFactor = 3
	}
	if c.Storage.ReplicationFactor > 10 {
		c.Storage.ReplicationFactor = 10
	}
	if c.Crypto.PoWDifficulty < 4 {
		c.Crypto.PoWDifficulty = 4
	}
	if c.Crypto.PoWDifficulty > 32 {
		c.Crypto.PoWDifficulty = 32
	}
	if c.Performance.MemoryLimitMB > 0 && c.Performance.MemoryLimitMB < 256 {
		c.Performance.MemoryLimitMB = 256
	}
	return nil
}

func (c *NodeConfig) String() string {
	return fmt.Sprintf("NodeConfig{Mode=%s, Hardware=%s, UDPPort=%d, DataDir=%s, Replication=%d, PoWDifficulty=%d}",
		c.Mode, c.Hardware, c.Network.UDPPort, c.Storage.DataDir, c.Storage.ReplicationFactor, c.Crypto.PoWDifficulty)
}

func getTotalMemory() int64 {
	return 8192
}

func (c *NodeConfig) IsRelayNode() bool {
	return c.Mode == ModeRelay
}

func (c *NodeConfig) IsBunkerMode() bool {
	return c.Mode == ModeBunker
}

func (c *NodeConfig) IsLightNode() bool {
	return c.Mode == ModeLight
}

func (c *NodeConfig) GetDataPath(subdir string) string {
	if subdir == "" {
		return c.Storage.DataDir
	}
	return c.Storage.DataDir + "/" + subdir
}
