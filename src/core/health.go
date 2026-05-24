package core

import (
        "encoding/json"
        "net/http"
        "sync"
        "time"

        "github.com/mamanga1/web5-mesh/src/config"
)

type HealthServer struct {
        node  *SovereignNode
        cfg   *config.NodeConfig
        mu    sync.RWMutex
        start time.Time
}

func NewHealthServer(node *SovereignNode, cfg *config.NodeConfig) *HealthServer {
        return &HealthServer{
                node:  node,
                cfg:   cfg,
                start: time.Now(),
        }
}

func (h *HealthServer) GetHealthHandler() http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                status := map[string]interface{}{
                        "status":      "healthy",
                        "uptime":      time.Since(h.start).String(),
                        "is_running":  h.node.IsRunning(),
                        "node_id":     h.node.GetDID(),
                        "active_peers": 0,
                }
                w.Header().Set("Content-Type", "application/json")
                json.NewEncoder(w).Encode(status)
        }
}

func (h *HealthServer) GetLivenessHandler() http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                if h.node.IsRunning() {
                        w.WriteHeader(http.StatusOK)
                        w.Write([]byte(`{"status":"alive"}`))
                } else {
                        w.WriteHeader(http.StatusServiceUnavailable)
                        w.Write([]byte(`{"status":"dead"}`))
                }
        }
}

func (h *HealthServer) GetReadinessHandler() http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                if h.node.IsRunning() {
                        w.WriteHeader(http.StatusOK)
                        w.Write([]byte(`{"status":"ready"}`))
                } else {
                        w.WriteHeader(http.StatusServiceUnavailable)
                        w.Write([]byte(`{"status":"not ready"}`))
                }
        }
}
