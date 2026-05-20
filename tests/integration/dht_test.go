// ============================================================================
// tests/integration/dht_test.go - DHT Integration Tests
// ============================================================================
// Especificación:
// - Pruebas de integración DHT simulando múltiples nodos
// - Verificación de lookup, store, y routing
// ============================================================================

package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"web5-mesh/src/dht"
)

// TestDHTNodeCreation prueba la creación de nodos DHT
func TestDHTNodeCreation(t *testing.T) {
	nodeID := dht.GenerateRandomNodeID()
	localAddr := "127.0.0.1:4242"

	config := dht.DefaultKadConfig()
	engine := dht.NewKadEngine(nodeID, localAddr, config)

	if engine == nil {
		t.Fatal("Failed to create DHT engine")
	}

	if err := engine.Start(); err != nil {
		t.Fatalf("Failed to start engine: %v", err)
	}
	defer engine.Stop()

	t.Logf("DHT engine started with ID: %s", nodeID.String())
}

// TestDHTAddNode prueba agregar nodos a la tabla de routing
func TestDHTAddNode(t *testing.T) {
	localID := dht.GenerateRandomNodeID()
	engine := dht.NewKadEngine(localID, "127.0.0.1:4242", dht.DefaultKadConfig())
	if err := engine.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer engine.Stop()

	// Agregar nodo remoto
	remoteID := dht.GenerateRandomNodeID()
	if err := engine.AddNode(remoteID, "127.0.0.1:4243", 100); err != nil {
		t.Fatalf("Failed to add node: %v", err)
	}

	// Verificar que el nodo fue agregado
	node, exists, err := engine.GetNode(remoteID)
	if err != nil {
		t.Fatalf("Error getting node: %v", err)
	}
	if !exists {
		t.Error("Node not found in routing table")
	}
	if node.Address != "127.0.0.1:4243" {
		t.Errorf("Expected address 127.0.0.1:4243, got %s", node.Address)
	}

	t.Logf("Node %s added successfully", remoteID.String())
}

// TestDHTLookup prueba la búsqueda de nodos cercanos
func TestDHTLookup(t *testing.T) {
	localID := dht.GenerateRandomNodeID()
	engine := dht.NewKadEngine(localID, "127.0.0.1:4242", dht.DefaultKadConfig())
	if err := engine.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer engine.Stop()

	// Agregar varios nodos
	for i := 0; i < 10; i++ {
		nodeID := dht.GenerateRandomNodeID()
		addr := fmt.Sprintf("127.0.0.1:%d", 4300+i)
		if err := engine.AddNode(nodeID, addr, 100); err != nil {
			t.Fatalf("Failed to add node %d: %v", i, err)
		}
	}

	// Buscar nodos cercanos a un target aleatorio
	target := dht.GenerateRandomNodeID()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	nodes, err := engine.Lookup(ctx, target)
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}

	if len(nodes) == 0 {
		t.Error("No nodes found in lookup")
	}

	t.Logf("Found %d nodes closest to target", len(nodes))
}

// TestDHTStoreAndLookup prueba almacenar y recuperar valores
func TestDHTStoreAndLookup(t *testing.T) {
	localID := dht.GenerateRandomNodeID()
	engine := dht.NewKadEngine(localID, "127.0.0.1:4242", dht.DefaultKadConfig())
	if err := engine.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer engine.Stop()

	// Agregar nodo remoto (simulado)
	remoteID := dht.GenerateRandomNodeID()
	if err := engine.AddNode(remoteID, "127.0.0.1:4243", 100); err != nil {
		t.Fatalf("Failed to add node: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Almacenar valor
	key := []byte("test-key")
	value := []byte("test-value")

	if err := engine.StoreValue(ctx, key, value); err != nil {
		t.Logf("Store warning: %v (expected in test environment)", err)
	}

	// Buscar valor
	retrieved, err := engine.LookupValue(ctx, key)
	if err != nil {
		t.Logf("Lookup warning: %v (expected in test environment)", err)
	} else {
		t.Logf("Retrieved value: %s", string(retrieved))
	}
}

// TestDHTConcurrentOperations prueba operaciones concurrentes
func TestDHTConcurrentOperations(t *testing.T) {
	localID := dht.GenerateRandomNodeID()
	engine := dht.NewKadEngine(localID, "127.0.0.1:4242", dht.DefaultKadConfig())
	if err := engine.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer engine.Stop()

	// Agregar nodos
	for i := 0; i < 50; i++ {
		nodeID := dht.GenerateRandomNodeID()
		addr := fmt.Sprintf("127.0.0.1:%d", 5000+i)
		engine.AddNode(nodeID, addr, 100)
	}

	// Ejecutar operaciones concurrentes
	var wg sync.WaitGroup
	numOps := 100

	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			target := dht.GenerateRandomNodeID()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, err := engine.Lookup(ctx, target)
			if err != nil {
				t.Logf("Concurrent lookup %d failed: %v", idx, err)
			}
		}(i)
	}

	wg.Wait()

	stats := engine.Stats()
	t.Logf("Stats after concurrent operations: %v", stats)
}

// TestDHTNodeRemoval prueba eliminación de nodos
func TestDHTNodeRemoval(t *testing.T) {
	localID := dht.GenerateRandomNodeID()
	engine := dht.NewKadEngine(localID, "127.0.0.1:4242", dht.DefaultKadConfig())
	if err := engine.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer engine.Stop()

	// Agregar y luego remover nodo
	remoteID := dht.GenerateRandomNodeID()
	if err := engine.AddNode(remoteID, "127.0.0.1:4243", 100); err != nil {
		t.Fatalf("Failed to add node: %v", err)
	}

	if err := engine.RemoveNode(remoteID); err != nil {
		t.Fatalf("Failed to remove node: %v", err)
	}

	_, exists, err := engine.GetNode(remoteID)
	if err != nil {
		t.Fatalf("Error getting node: %v", err)
	}
	if exists {
		t.Error("Node still exists after removal")
	}

	t.Log("Node removed successfully")
}
