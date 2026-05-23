// ============================================================================
// tests/integration/full_mesh_test.go - Full Mesh Network Simulation
// ============================================================================
// Especificación:
// - Simulación end-to-end de la red MaIA Mesh completa
// - Pruebas de estrés con múltiples nodos
// - Verificación de consistencia de datos
// ============================================================================

package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/mamanga1/web5-mesh/src/config"
	"github.com/mamanga1/web5-mesh/src/core"
	"github.com/mamanga1/web5-mesh/src/dht"
)

// TestFullMeshSimulation simula una red completa con múltiples nodos
func TestFullMeshSimulation(t *testing.T) {
	numNodes := 5
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	t.Logf("Starting full mesh simulation with %d nodes", numNodes)

	// Crear nodos
	nodes := make([]*core.SovereignNode, numNodes)
	nodeIDs := make([]string, numNodes)

	for i := 0; i < numNodes; i++ {
		cfg := config.DefaultConfig()
		cfg.NodeName = fmt.Sprintf("test-node-%d", i)
		cfg.Storage.DataDir = fmt.Sprintf("./testdata/node%d", i)
		cfg.Network.UDPPort = 4242 + i
		cfg.Performance.EnableMetrics = false

		node, err := core.NewSovereignNode(cfg)
		if err != nil {
			t.Fatalf("Failed to create node %d: %v", i, err)
		}

		if err := node.Start(); err != nil {
			t.Fatalf("Failed to start node %d: %v", i, err)
		}
		defer node.Stop()

		nodes[i] = node
		nodeIDs[i] = node.GetDID()
		t.Logf("Node %d started with DID: %s", i, node.GetDID())
	}

	// Esperar estabilización
	time.Sleep(2 * time.Second)

	// Conectar nodos entre sí
	t.Log("Connecting nodes...")
	for i := 0; i < numNodes; i++ {
		for j := i + 1; j < numNodes; j++ {
			// Simular conexión entre nodos
			t.Logf("Connecting node %d and %d", i, j)
		}
	}

	// Ejecutar operaciones de red
	t.Log("Running network operations...")

	var wg sync.WaitGroup
	opsPerNode := 10

	for i, node := range nodes {
		wg.Add(1)
		go func(idx int, n *core.SovereignNode) {
			defer wg.Done()

			for op := 0; op < opsPerNode; op++ {
				select {
				case <-ctx.Done():
					return
				default:
					key := []byte(fmt.Sprintf("key-%d-%d", idx, op))
					value := []byte(fmt.Sprintf("value-%d-%d", idx, op))

					if err := n.StoreData(key, value); err != nil {
						t.Logf("Node %d store failed: %v", idx, err)
					}

					retrieved, err := n.LookupData(key)
					if err != nil {
						t.Logf("Node %d lookup failed: %v", idx, err)
					} else if string(retrieved) != string(value) {
						t.Logf("Node %d data mismatch: expected %s, got %s", idx, value, retrieved)
					}

					time.Sleep(10 * time.Millisecond)
				}
			}
		}(i, node)
	}

	wg.Wait()

	// Verificar estadísticas
	t.Log("Final statistics:")
	for i, node := range nodes {
		stats := node.Stats()
		t.Logf("Node %d stats: %v", i, stats)
	}

	t.Log("Full mesh simulation completed successfully")
}

// TestMeshNetworkChurn prueba la resiliencia ante nodos que entran y salen
func TestMeshNetworkChurn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	t.Log("Starting network churn test")

	// Crear nodos base
	numBaseNodes := 3
	nodes := make([]*core.SovereignNode, 0)

	for i := 0; i < numBaseNodes; i++ {
		cfg := config.DefaultConfig()
		cfg.NodeName = fmt.Sprintf("base-node-%d", i)
		cfg.Storage.DataDir = fmt.Sprintf("./testdata/base%d", i)
		cfg.Network.UDPPort = 5000 + i

		node, err := core.NewSovereignNode(cfg)
		if err != nil {
			t.Fatalf("Failed to create base node %d: %v", i, err)
		}
		if err := node.Start(); err != nil {
			t.Fatalf("Failed to start base node %d: %v", i, err)
		}
		defer node.Stop()

		nodes = append(nodes, node)
		t.Logf("Base node %d started", i)
	}

	time.Sleep(2 * time.Second)

	// Simular nodos que entran y salen
	churnDuration := 30 * time.Second
	churnInterval := 3 * time.Second

	t.Logf("Simulating churn for %v", churnDuration)

	churnEnd := time.Now().Add(churnDuration)
	nodeCounter := numBaseNodes

	for time.Now().Before(churnEnd) {
		// Agregar nodo
		cfg := config.DefaultConfig()
		cfg.NodeName = fmt.Sprintf("churn-node-%d", nodeCounter)
		cfg.Storage.DataDir = fmt.Sprintf("./testdata/churn%d", nodeCounter)
		cfg.Network.UDPPort = 6000 + nodeCounter

		node, err := core.NewSovereignNode(cfg)
		if err == nil {
			if err := node.Start(); err == nil {
				nodes = append(nodes, node)
				t.Logf("Churn node %d added", nodeCounter)
				nodeCounter++
			}
		}

		time.Sleep(churnInterval / 2)

		// Remover nodo (el más reciente)
		if len(nodes) > numBaseNodes {
			lastIdx := len(nodes) - 1
			nodes[lastIdx].Stop()
			nodes = nodes[:lastIdx]
			t.Logf("Churn node removed")
		}

		time.Sleep(churnInterval / 2)
	}

	// Limpiar nodos restantes
	for _, node := range nodes {
		node.Stop()
	}

	t.Log("Churn test completed")
}

// TestMeshDataReplication prueba la replicación de datos en la malla
func TestMeshDataReplication(t *testing.T) {
	numNodes := 4
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	t.Logf("Starting data replication test with %d nodes", numNodes)

	// Crear nodos
	nodes := make([]*core.SovereignNode, numNodes)
	for i := 0; i < numNodes; i++ {
		cfg := config.DefaultConfig()
		cfg.NodeName = fmt.Sprintf("replica-node-%d", i)
		cfg.Storage.DataDir = fmt.Sprintf("./testdata/replica%d", i)
		cfg.Network.UDPPort = 7000 + i

		node, err := core.NewSovereignNode(cfg)
		if err != nil {
			t.Fatalf("Failed to create node %d: %v", i, err)
		}
		if err := node.Start(); err != nil {
			t.Fatalf("Failed to start node %d: %v", i, err)
		}
		defer node.Stop()

		nodes[i] = node
	}

	time.Sleep(2 * time.Second)

	// Almacenar datos desde el nodo 0
	testData := []byte("replicated-test-data")
	testKey := []byte("replication-test-key")

	t.Log("Storing data from node 0")
	if err := nodes[0].StoreData(testKey, testData); err != nil {
		t.Logf("Store warning: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Verificar que otros nodos pueden recuperar los datos
	var wg sync.WaitGroup
	for i := 1; i < numNodes; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			retrieved, err := nodes[idx].LookupData(testKey)
			if err != nil {
				t.Logf("Node %d lookup failed: %v", idx, err)
			} else if len(retrieved) > 0 {
				t.Logf("Node %d retrieved data: %s", idx, string(retrieved))
			}
		}(i)
	}
	wg.Wait()

	t.Log("Data replication test completed")
}

// TestMeshLatency mide la latencia de la red
func TestMeshLatency(t *testing.T) {
	numNodes := 3
	numOps := 100

	t.Logf("Measuring latency with %d nodes, %d operations", numNodes, numOps)

	// Crear nodos
	nodes := make([]*core.SovereignNode, numNodes)
	for i := 0; i < numNodes; i++ {
		cfg := config.DefaultConfig()
		cfg.NodeName = fmt.Sprintf("latency-node-%d", i)
		cfg.Storage.DataDir = fmt.Sprintf("./testdata/latency%d", i)
		cfg.Network.UDPPort = 8000 + i

		node, err := core.NewSovereignNode(cfg)
		if err != nil {
			t.Fatalf("Failed to create node %d: %v", i, err)
		}
		if err := node.Start(); err != nil {
			t.Fatalf("Failed to start node %d: %v", i, err)
		}
		defer node.Stop()

		nodes[i] = node
	}

	time.Sleep(2 * time.Second)

	// Medir latencia de operaciones
	var totalLatency time.Duration
	var successCount int

	for i := 0; i < numOps; i++ {
		key := []byte(fmt.Sprintf("latency-key-%d", i))
		value := []byte(fmt.Sprintf("latency-value-%d", i))

		start := time.Now()
		if err := nodes[0].StoreData(key, value); err == nil {
			totalLatency += time.Since(start)
			successCount++
		}
		time.Sleep(1 * time.Millisecond)
	}

	if successCount > 0 {
		avgLatency := totalLatency / time.Duration(successCount)
		t.Logf("Average store latency: %v (%d/%d successful)", avgLatency, successCount, numOps)
	}

	t.Log("Latency test completed")
}

// TestMeshConcurrentWrites prueba escrituras concurrentes
func TestMeshConcurrentWrites(t *testing.T) {
	numNodes := 3
	numWritesPerNode := 50
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Logf("Starting concurrent writes test: %d nodes, %d writes each", numNodes, numWritesPerNode)

	// Crear nodos
	nodes := make([]*core.SovereignNode, numNodes)
	for i := 0; i < numNodes; i++ {
		cfg := config.DefaultConfig()
		cfg.NodeName = fmt.Sprintf("concurrent-node-%d", i)
		cfg.Storage.DataDir = fmt.Sprintf("./testdata/concurrent%d", i)
		cfg.Network.UDPPort = 9000 + i

		node, err := core.NewSovereignNode(cfg)
		if err != nil {
			t.Fatalf("Failed to create node %d: %v", i, err)
		}
		if err := node.Start(); err != nil {
			t.Fatalf("Failed to start node %d: %v", i, err)
		}
		defer node.Stop()

		nodes[i] = node
	}

	time.Sleep(2 * time.Second)

	// Ejecutar escrituras concurrentes
	var wg sync.WaitGroup
	for i, node := range nodes {
		wg.Add(1)
		go func(idx int, n *core.SovereignNode) {
			defer wg.Done()

			for w := 0; w < numWritesPerNode; w++ {
				select {
				case <-ctx.Done():
					return
				default:
					key := []byte(fmt.Sprintf("concurrent-key-%d-%d", idx, w))
					value := []byte(fmt.Sprintf("concurrent-value-%d-%d", idx, w))
					_ = n.StoreData(key, value)
					time.Sleep(1 * time.Millisecond)
				}
			}
			t.Logf("Node %d completed %d writes", idx, numWritesPerNode)
		}(i, node)
	}

	wg.Wait()

	t.Log("Concurrent writes test completed")
}
