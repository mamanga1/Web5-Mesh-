// ============================================================================
// src/core/main.go - Main Entry Point - Node Orchestrator
// ============================================================================
// Especificación:
// - Entry point principal del nodo
// - Orquesta la inicialización de todos los componentes
// - Parsea flags de línea de comandos
// - Levanta servidor de métricas Prometheus
// - Maneja señales de shutdown (SIGINT, SIGTERM)
// ============================================================================

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"web5-mesh/src/config"
	"web5-mesh/src/core"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// Variables de build (inyectadas en tiempo de compilación)
	Version   = "2.0.0-production"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

// main es el punto de entrada principal
func main() {
	// Configurar logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetOutput(os.Stdout)

	// Mostrar banner
	printBanner()

	// Parsear configuración
	cfg := config.DefaultConfig()
	cfg.ParseFlags()

	// Validar configuración
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	// Configurar límites de memoria si se especificó
	if cfg.Performance.MemoryLimitMB > 0 {
		setMemoryLimit(cfg.Performance.MemoryLimitMB)
	}

	// Configurar GC
	debug.SetGCPercent(cfg.Performance.GCPercent)

	// Log de inicio
	log.Printf("Starting MaIA Mesh Node v%s", Version)
	log.Printf("Mode: %s | Hardware: %s", cfg.Mode, cfg.Hardware)
	log.Printf("UDP Port: %d | Data Dir: %s", cfg.Network.UDPPort, cfg.Storage.DataDir)
	log.Printf("Max Goroutines: %d | Memory Limit: %d MB", cfg.Performance.MaxGoroutines, cfg.Performance.MemoryLimitMB)

	// Crear nodo soberano
	node, err := core.NewSovereignNode(cfg)
	if err != nil {
		log.Fatalf("Failed to create node: %v", err)
	}

	// Iniciar nodo
	if err := node.Start(); err != nil {
		log.Fatalf("Failed to start node: %v", err)
	}
	log.Printf("Node started successfully")
	log.Printf("DID: %s", node.GetDID())

	// Iniciar servidor de métricas
	if cfg.Performance.EnableMetrics {
		startMetricsServer(cfg.Performance.MetricsPort, node)
	}

	// Iniciar bootstrap en background
	go func() {
		time.Sleep(2 * time.Second) // Esperar que el nodo se estabilice
		if err := node.Bootstrap(); err != nil {
			log.Printf("Warning: Bootstrap failed: %v", err)
		} else {
			log.Printf("Node bootstrapped successfully")
		}
	}()

	// Esperar señal de shutdown
	waitForShutdown(node)

	log.Println("Node shutdown complete")
}

// printBanner muestra el banner de inicio
func printBanner() {
	banner := `
╔══════════════════════════════════════════════════════════════════╗
║                                                                  ║
║   ███╗   ███╗ █████╗ ██╗ █████╗     ███╗   ███╗███████╗███████╗██╗  ██╗
║   ████╗ ████║██╔══██╗██║██╔══██╗    ████╗ ████║██╔════╝██╔════╝██║  ██║
║   ██╔████╔██║███████║██║███████║    ██╔████╔██║█████╗  ███████╗███████║
║   ██║╚██╔╝██║██╔══██║██║██╔══██║    ██║╚██╔╝██║██╔══╝  ╚════██║██╔══██║
║   ██║ ╚═╝ ██║██║  ██║██║██║  ██║    ██║ ╚═╝ ██║███████╗███████║██║  ██║
║   ╚═╝     ╚═╝╚═╝  ╚═╝╚═╝╚═╝  ╚═╝    ╚═╝     ╚═╝╚══════╝╚══════╝╚═╝  ╚═╝
║                                                                  ║
║                    Sovereign Web5 Mesh Network                    ║
║                         v%s                                      ║
║                   Made with ❤️ in Corrientes, AR                  ║
╚══════════════════════════════════════════════════════════════════╝
`
	fmt.Printf(banner, Version)
	fmt.Printf("\nBuild: %s | Commit: %s\n\n", BuildTime, GitCommit)
}

// startMetricsServer inicia el servidor de métricas Prometheus
func startMetricsServer(port int, node *core.SovereignNode) {
	mux := http.NewServeMux()

	// Endpoint de métricas Prometheus
	mux.Handle("/metrics", promhttp.Handler())

	// Endpoints de health check
	mux.HandleFunc("/health", node.GetHealthHandler())
	mux.HandleFunc("/health/live", node.GetLivenessHandler())
	mux.HandleFunc("/health/ready", node.GetReadinessHandler())

	// Endpoint de estadísticas
	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(node.Stats())
	})

	addr := fmt.Sprintf(":%d", port)
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	go func() {
		log.Printf("Metrics server listening on %s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Metrics server error: %v", err)
		}
	}()
}

// waitForShutdown espera señales de terminación y realiza shutdown graceful
func waitForShutdown(node *core.SovereignNode) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	log.Printf("Received signal: %v", sig)

	log.Println("Shutting down gracefully...")

	// Crear contexto con timeout para shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Detener nodo (en goroutine para no bloquear)
	done := make(chan struct{})
	go func() {
		if err := node.Stop(); err != nil {
			log.Printf("Error during shutdown: %v", err)
		}
		close(done)
	}()

	// Esperar shutdown o timeout
	select {
	case <-done:
		log.Println("Node stopped successfully")
	case <-ctx.Done():
		log.Println("Shutdown timeout, forcing exit")
	}
}

// setMemoryLimit establece un límite suave de memoria (Go)
func setMemoryLimit(limitMB int) {
	// Configurar límite de memoria para el garbage collector
	// Esto es una aproximación, no un límite estricto
	memLimit := int64(limitMB) * 1024 * 1024
	debug.SetMemoryLimit(memLimit)
	log.Printf("Memory limit set to %d MB", limitMB)
}
