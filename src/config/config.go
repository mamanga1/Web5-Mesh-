// ============================================================================
// src/config/config.go - Node Configuration & Flags Parser
// ============================================================================
// Especificación:
// - Flags de línea de comandos para configuración del nodo
// - Modos de operación: full, relay, bunker
// - Configuración de puertos UDP y límites de memoria
// ============================================================================

package config

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// NodeMode define el modo de operación del nodo
type NodeMode string

const (
	// ModeFull: nodo completo con todas las capacidades
	ModeFull NodeMode = "full"
	// ModeRelay: nodo optimizado para reenviar tráfico (mayor reputación)
	ModeRelay NodeMode = "relay"
	// ModeBunker: modo aislado, solo operaciones locales (sin conectividad saliente)
	ModeBunker NodeMode = "bunker"
	// ModeLight: modo ligero para dispositivos con recursos limitados
	ModeLight NodeMode = "light"
)

// HardwareProfile define el perfil de hardware detectado
type HardwareProfile string

const (
	HardwareXeon   HardwareProfile = "xeon"    // Server-grade (64GB+ RAM, multi-core)
	HardwareDesktop HardwareProfile = "desktop" // Standard PC (8-16GB RAM)
	HardwareMobile  HardwareProfile = "mobile"  // Phone/Poco F1 (4-6GB RAM)
	HardwareTVBox   HardwareProfile = "tvbox"   // TV Box (1-2GB RAM, ARM)
	HardwareUnknown HardwareProfile = "unknown"
)

// NetworkConfig configuración de red
type NetworkConfig struct {
	UDPPort          int           // Puerto UDP para DHT (default: 4242)
	TCPPort          int           // Puerto TCP para relay fallback (default: 4243)
	MaxConnections   int           // Máximo de conexiones simultáneas (default: 1024)
	HandshakeTimeout time.Duration // Timeout para handshake (default: 30s)
	HeartbeatInterval time.Duration // Intervalo de heartbeat (default: 15s)
	LookupTimeout    time.Duration // Timeout para lookups DHT (default: 3s)

	NAT struct {
		AutoDiscover   bool     // Auto-detectar NAT usando STUN
		STUNServers    []string // Servidores STUN para descubrimiento
		EnableUPnP     bool     // Intentar UPnP para abrir puertos
		RelayServer    string   // Relay fallback (TURN descentralizado)
	}
}

// StorageConfig configuración de almacenamiento
type StorageConfig struct {
	DataDir           string // Directorio de datos (default: "./data")
	ReplicationFactor int    // Factor de replicación (min: 3)
	MaxBlockSize      int    // Tamaño máximo de bloque en bytes (default: 1MB)
	CRDTCacheSize     int    // Tamaño de caché CRDT en MB (default: 100)
	BadgerOptions     struct {
		MemTableSize   int64 // Tamaño de memtable (default: 64MB)
		ValueLogSize   int64 // Tamaño del log de valores (default: 1GB)
		NumMemtables   int   // Número de memtables (default: 2)
		Compression    bool  // Habilitar compresión
	}
}

// CryptoConfig configuración criptográfica
type CryptoConfig struct {
	IdentityFile      string // Archivo con identidad persistente
	PoWDifficulty     int    // Dificultad de Proof of Work (default: 16)
	EnableNoise       bool   // Habilitar Noise Protocol
	EnableChaCha20    bool   // Usar ChaCha20-Poly1305 (default: true)
	SessionKeyLifetime time.Duration // Duración de claves de sesión
}

// PerformanceConfig configuración de performance
type PerformanceConfig struct {
	MaxGoroutines     int  // Máximo de goroutines simultáneas (default: 1000)
	MemoryLimitMB     int  // Límite de memoria en MB (0 = sin límite)
	GCPercent         int  // Porcentaje de GC (default: 100)
	EnableProfiling   bool // Habilitar profiling (solo desarrollo)
	EnableMetrics     bool // Habilitar métricas Prometheus (default: true)
	MetricsPort       int  // Puerto para métricas (default: 2112)
}

// BootstrapConfig configuración de bootstrap
type BootstrapConfig struct {
	SeedNodes      []string // DIDs/IPs de nodos semilla
	AutoDiscover   bool     // Auto-descubrimiento de peers
	MaxBootstrapAttempts int // Intentos máximos de bootstrap (default: 5)
	BootstrapTimeout   time.Duration // Timeout por intento (default: 10s)
}

// NodeConfig es la configuración completa del nodo
type NodeConfig struct {
	// Identidad
	NodeID      string // DID del nodo (generado automáticamente si no se provee)
	NodeName    string // Nombre legible del nodo
	Mode        NodeMode

	// Hardware
	Hardware     HardwareProfile
	HardwareAuto bool // Auto-detectar hardware

	// Subconfiguraciones
	Network     NetworkConfig
	Storage     StorageConfig
	Crypto      CryptoConfig
	Performance PerformanceConfig
	Bootstrap   BootstrapConfig

	// Logging
	LogLevel    string // debug, info, warn, error (default: info)
	LogFile     string // Archivo de log (vacío = stdout)
	Verbose     bool   // Modo verbose

	// Archivos
	ConfigFile  string // Archivo de configuración (opcional)
}

// DefaultConfig retorna la configuración por defecto
func DefaultConfig() *NodeConfig {
	cfg := &NodeConfig{
		NodeName:    "MaIA-Mesh-Node",
		Mode:        ModeFull,
		Hardware:    HardwareUnknown,
		HardwareAuto: true,
		LogLevel:    "info",
		Verbose:     false,
	}

	// Configuración de red por defecto
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

	// Configuración de almacenamiento por defecto
	cfg.Storage.DataDir = "./data"
	cfg.Storage.ReplicationFactor = 3
	cfg.Storage.MaxBlockSize = 1024 * 1024 // 1MB
	cfg.Storage.CRDTCacheSize = 100
	cfg.Storage.BadgerOptions.MemTableSize = 64 * 1024 * 1024 // 64MB
	cfg.Storage.BadgerOptions.ValueLogSize = 1024 * 1024 * 1024 // 1GB
	cfg.Storage.BadgerOptions.NumMemtables = 2
	cfg.Storage.BadgerOptions.Compression = true

	// Configuración criptográfica por defecto
	cfg.Crypto.PoWDifficulty = 16
	cfg.Crypto.EnableNoise = true
	cfg.Crypto.EnableChaCha20 = true
	cfg.Crypto.SessionKeyLifetime = 24 * time.Hour

	// Configuración de performance por defecto
	cfg.Performance.MaxGoroutines = 1000
	cfg.Performance.MemoryLimitMB = 0
	cfg.Performance.GCPercent = 100
	cfg.Performance.EnableProfiling = false
	cfg.Performance.EnableMetrics = true
	cfg.Performance.MetricsPort = 2112

	// Configuración de bootstrap por defecto
	cfg.Bootstrap.SeedNodes = []string{}
	cfg.Bootstrap.AutoDiscover = true
	cfg.Bootstrap.MaxBootstrapAttempts = 5
	cfg.Bootstrap.BootstrapTimeout = 10 * time.Second

	return cfg
}

// ParseFlags parsea los flags de línea de comandos y actualiza la configuración
func (c *NodeConfig) ParseFlags() {
	// Modo
	modeStr := flag.String("mode", string(ModeFull), "Node mode: full, relay, bunker, light")
	
	// Red
	udpPort := flag.Int("udp-port", c.Network.UDPPort, "UDP port for DHT")
	tcpPort := flag.Int("tcp-port", c.Network.TCPPort, "TCP port for relay")
	maxConns := flag.Int("max-connections", c.Network.MaxConnections, "Maximum concurrent connections")
	
	// NAT
	natAuto := flag.Bool("nat-auto", c.Network.NAT.AutoDiscover, "Auto-detect NAT using STUN")
	relayServer := flag.String("relay", c.Network.NAT.RelayServer, "Relay server address for fallback")
	
	// Almacenamiento
	dataDir := flag.String("data-dir", c.Storage.DataDir, "Data directory for storage")
	replication := flag.Int("replication", c.Storage.ReplicationFactor, "Replication factor (min 3)")
	
	// Crypto
	identityFile := flag.String("identity", c.Crypto.IdentityFile, "Identity file path")
	powDifficulty := flag.Int("pow-difficulty", c.Crypto.PoWDifficulty, "Proof of Work difficulty (bits)")
	
	// Performance
	maxGoroutines := flag.Int("max-goroutines", c.Performance.MaxGoroutines, "Maximum concurrent goroutines")
	memoryLimit := flag.Int("memory-limit", c.Performance.MemoryLimitMB, "Memory limit in MB (0 = unlimited)")
	metricsPort := flag.Int("metrics-port", c.Performance.MetricsPort, "Prometheus metrics port")
	
	// Bootstrap
	seedNodes := flag.String("seeds", "", "Comma-separated list of seed node DIDs/IPs")
	
	// Logging
	logLevel := flag.String("log-level", c.LogLevel, "Log level: debug, info, warn, error")
	verbose := flag.Bool("verbose", c.Verbose, "Verbose output")
	
	flag.Parse()

	// Aplicar valores
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

	// Auto-detectar hardware si está habilitado
	if c.HardwareAuto {
		c.detectHardware()
	}
}

// detectHardware detecta automáticamente el perfil de hardware
func (c *NodeConfig) detectHardware() {
	numCPU := runtime.NumCPU()
	totalMem := getTotalMemory()

	switch {
	case numCPU >= 16 && totalMem >= 64:
		c.Hardware = HardwareXeon
		c.applyHardwareProfile(HardwareXeon)
	case numCPU >= 4 && totalMem >= 8:
		c.Hardware = HardwareDesktop
		c.applyHardwareProfile(HardwareDesktop)
	case numCPU >= 4 && totalMem >= 4:
		c.Hardware = HardwareMobile
		c.applyHardwareProfile(HardwareMobile)
	case numCPU >= 2 && totalMem >= 1:
		c.Hardware = HardwareTVBox
		c.applyHardwareProfile(HardwareTVBox)
	default:
		c.Hardware = HardwareUnknown
	}
}

// applyHardwareProfile aplica optimizaciones según el perfil de hardware
func (c *NodeConfig) applyHardwareProfile(profile HardwareProfile) {
	switch profile {
	case HardwareXeon:
		// Servidor: máximo rendimiento
		c.Performance.MaxGoroutines = 5000
		c.Network.MaxConnections = 5000
		c.Storage.CRDTCacheSize = 500
		c.Storage.BadgerOptions.MemTableSize = 128 * 1024 * 1024
		
	case HardwareDesktop:
		// PC estándar: balance
		c.Performance.MaxGoroutines = 2000
		c.Network.MaxConnections = 2000
		c.Storage.CRDTCacheSize = 200
		
	case HardwareMobile:
		// Móvil: conservador
		c.Performance.MaxGoroutines = 500
		c.Network.MaxConnections = 500
		c.Storage.CRDTCacheSize = 50
		c.Performance.GCPercent = 200
		
	case HardwareTVBox:
		// TV Box: muy limitado
		c.Performance.MaxGoroutines = 200
		c.Network.MaxConnections = 200
		c.Storage.CRDTCacheSize = 25
		c.Performance.GCPercent = 300
		c.Crypto.PoWDifficulty = 12 // Menor dificultad para hardware débil
	}
}

// Validate valida la configuración y ajusta valores inválidos
func (c *NodeConfig) Validate() error {
	// Validar modo
	switch c.Mode {
	case ModeFull, ModeRelay, ModeBunker, ModeLight:
		// OK
	default:
		c.Mode = ModeFull
	}

	// Validar puertos
	if c.Network.UDPPort < 1024 || c.Network.UDPPort > 65535 {
		c.Network.UDPPort = 4242
	}
	if c.Network.TCPPort < 1024 || c.Network.TCPPort > 65535 {
		c.Network.TCPPort = 4243
	}

	// Validar factor de replicación
	if c.Storage.ReplicationFactor < 1 {
		c.Storage.ReplicationFactor = 3
	}
	if c.Storage.ReplicationFactor > 10 {
		c.Storage.ReplicationFactor = 10
	}

	// Validar PoW difficulty
	if c.Crypto.PoWDifficulty < 4 {
		c.Crypto.PoWDifficulty = 4
	}
	if c.Crypto.PoWDifficulty > 32 {
		c.Crypto.PoWDifficulty = 32
	}

	// Validar límites de memoria
	if c.Performance.MemoryLimitMB > 0 && c.Performance.MemoryLimitMB < 256 {
		c.Performance.MemoryLimitMB = 256
	}

	return nil
}

// String retorna representación string de la configuración
func (c *NodeConfig) String() string {
	return fmt.Sprintf(
		"NodeConfig{Mode=%s, Hardware=%s, UDPPort=%d, DataDir=%s, Replication=%d, PoWDifficulty=%d}",
		c.Mode, c.Hardware, c.Network.UDPPort, c.Storage.DataDir, c.Storage.ReplicationFactor, c.Crypto.PoWDifficulty,
	)
}

// getTotalMemory retorna la memoria total en MB (aproximada)
func getTotalMemory() int64 {
	// En producción, usar syscall o gopsutil
	// Por ahora, retornar un valor por defecto
	return 8192 // 8GB asumido
}

// IsRelayNode retorna true si el nodo debe actuar como relay
func (c *NodeConfig) IsRelayNode() bool {
	return c.Mode == ModeRelay
}

// IsBunkerMode retorna true si el nodo está en modo bunker
func (c *NodeConfig) IsBunkerMode() bool {
	return c.Mode == ModeBunker
}

// IsLightNode retorna true si el nodo está en modo ligero
func (c *NodeConfig) IsLightNode() bool {
	return c.Mode == ModeLight
}

// GetDataPath retorna la ruta completa para un subdirectorio de datos
func (c *NodeConfig) GetDataPath(subdir string) string {
	if subdir == "" {
		return c.Storage.DataDir
	}
	return c.Storage.DataDir + "/" + subdir
}
