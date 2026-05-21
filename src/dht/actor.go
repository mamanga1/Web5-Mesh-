// ============================================================================
// src/dht/actor.go - Actor Model for DHT Concurrency (Lock-Free)
// ============================================================================
// Especificación:
// - Patrón Actor para evitar bloqueos mutuos (deadlocks)
// - Centraliza toda la mutación de la tabla de enrutamiento en un único bucle
// - Mensajes con canales para comunicación segura entre goroutines
// ============================================================================

// ============================================================================
// src/dht/actor.go - Actor Model for DHT Concurrency (Lock-Free)
// ============================================================================

package dht

import (
	"fmt"
	"sync"
	"time"
)

type MsgType int

const (
	MsgFindClosest MsgType = iota
	MsgGetNode
	MsgTotalNodes
	MsgGetBucketStats
	MsgAddNode
	MsgRemoveNode
	MsgUpdateNode
	MsgPingNode
	MsgRefreshBuckets
	MsgShutdown
	MsgResponse
)

func (m MsgType) String() string {
	switch m {
	case MsgFindClosest:
		return "FindClosest"
	case MsgGetNode:
		return "GetNode"
	case MsgTotalNodes:
		return "TotalNodes"
	case MsgGetBucketStats:
		return "GetBucketStats"
	case MsgAddNode:
		return "AddNode"
	case MsgRemoveNode:
		return "RemoveNode"
	case MsgUpdateNode:
		return "UpdateNode"
	case MsgPingNode:
		return "PingNode"
	case MsgRefreshBuckets:
		return "RefreshBuckets"
	case MsgShutdown:
		return "Shutdown"
	case MsgResponse:
		return "Response"
	default:
		return "Unknown"
	}
}

type Request struct {
	Type    MsgType
	Data    interface{}
	RespCh  chan Response
	Timeout time.Duration
}

type Response struct {
	Type  MsgType
	Data  interface{}
	Error error
}

type FindClosestRequest struct {
	Target NodeID
	K      int
}

type FindClosestResponse struct {
	Nodes []*NodeEntry
}

type GetNodeRequest struct {
	ID NodeID
}

type GetNodeResponse struct {
	Node  *NodeEntry
	Found bool
}

type AddNodeRequest struct {
	Node *NodeEntry
}

type RemoveNodeRequest struct {
	ID NodeID
}

type UpdateNodeRequest struct {
	ID         NodeID
	Address    string
	Reputation uint64
}

type DHTActor struct {
	routingTable *RoutingTable
	requestCh    chan *Request
	stopCh       chan struct{}
	wg           sync.WaitGroup
	started      bool
	mu           sync.RWMutex
}

func NewDHTActor(localID NodeID) *DHTActor {
	return &DHTActor{
		routingTable: NewRoutingTable(localID),
		requestCh:    make(chan *Request, 1000),
		stopCh:       make(chan struct{}),
		started:      false,
	}
}

func (a *DHTActor) Start() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.started {
		return
	}
	a.started = true
	a.wg.Add(1)
	go a.run()
}

func (a *DHTActor) Stop() {
	a.mu.Lock()
	if !a.started {
		a.mu.Unlock()
		return
	}
	a.started = false
	a.mu.Unlock()
	close(a.stopCh)
	a.wg.Wait()
}

func (a *DHTActor) Send(req *Request) (*Response, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}
	if req.Timeout == 0 {
		req.Timeout = 30 * time.Second
	}
	if req.RespCh == nil {
		req.RespCh = make(chan Response, 1)
	}
	select {
	case a.requestCh <- req:
		select {
		case resp := <-req.RespCh:
			return &resp, nil
		case <-time.After(req.Timeout):
			return nil, fmt.Errorf("request timeout after %v", req.Timeout)
		}
	case <-a.stopCh:
		return nil, fmt.Errorf("actor is stopped")
	}
}

func (a *DHTActor) run() {
	defer a.wg.Done()
	for {
		select {
		case <-a.stopCh:
			return
		case req := <-a.requestCh:
			resp := a.processRequest(req)
			if req.RespCh != nil {
				select {
				case req.RespCh <- *resp:
				default:
				}
			}
		}
	}
}

func (a *DHTActor) processRequest(req *Request) *Response {
	var resp Response
	resp.Type = req.Type

	switch req.Type {
	case MsgFindClosest:
		data, ok := req.Data.(FindClosestRequest)
		if !ok {
			resp.Error = fmt.Errorf("invalid data type for FindClosest")
			return &resp
		}
		nodes := a.routingTable.FindClosest(data.Target, data.K)
		resp.Data = FindClosestResponse{Nodes: nodes}

	case MsgGetNode:
		data, ok := req.Data.(GetNodeRequest)
		if !ok {
			resp.Error = fmt.Errorf("invalid data type for GetNode")
			return &resp
		}
		node, found := a.routingTable.GetNode(data.ID)
		resp.Data = GetNodeResponse{Node: node, Found: found}

	case MsgTotalNodes:
		total := a.routingTable.TotalNodes()
		resp.Data = total

	case MsgGetBucketStats:
		stats := a.routingTable.GetBucketStats()
		resp.Data = stats

	case MsgAddNode:
		data, ok := req.Data.(AddNodeRequest)
		if !ok {
			resp.Error = fmt.Errorf("invalid data type for AddNode")
			return &resp
		}
		a.routingTable.AddNode(data.Node)
		resp.Data = true

	case MsgRemoveNode:
		data, ok := req.Data.(RemoveNodeRequest)
		if !ok {
			resp.Error = fmt.Errorf("invalid data type for RemoveNode")
			return &resp
		}
		removed := a.routingTable.RemoveNode(data.ID)
		resp.Data = removed

	case MsgUpdateNode:
		data, ok := req.Data.(UpdateNodeRequest)
		if !ok {
			resp.Error = fmt.Errorf("invalid data type for UpdateNode")
			return &resp
		}
		node, exists := a.routingTable.GetNode(data.ID)
		if exists {
			if data.Address != "" {
				node.Address = data.Address
			}
			if data.Reputation > 0 {
				node.Reputation = data.Reputation
			}
			node.LastSeen = time.Now()
			a.routingTable.AddNode(node)
			resp.Data = true
		} else {
			resp.Data = false
		}

	case MsgPingNode:
		data, ok := req.Data.(GetNodeRequest)
		if !ok {
			resp.Error = fmt.Errorf("invalid data type for PingNode")
			return &resp
		}
		node, exists := a.routingTable.GetNode(data.ID)
		if exists && time.Since(node.LastSeen) < 5*time.Minute {
			node.LastSeen = time.Now()
			resp.Data = true
		} else {
			if exists {
				a.routingTable.RemoveNode(data.ID)
			}
			resp.Data = false
		}

	case MsgRefreshBuckets:
		resp.Data = true

	case MsgShutdown:
		resp.Data = true

	default:
		resp.Error = fmt.Errorf("unknown message type: %v", req.Type)
	}
	return &resp
}

func (a *DHTActor) FindClosest(target NodeID, k int) ([]*NodeEntry, error) {
	resp, err := a.Send(&Request{
		Type: MsgFindClosest,
		Data: FindClosestRequest{Target: target, K: k},
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, resp.Error
	}
	result, ok := resp.Data.(FindClosestResponse)
	if !ok {
		return nil, fmt.Errorf("invalid response type")
	}
	return result.Nodes, nil
}

func (a *DHTActor) GetNode(id NodeID) (*NodeEntry, bool, error) {
	resp, err := a.Send(&Request{
		Type: MsgGetNode,
		Data: GetNodeRequest{ID: id},
	})
	if err != nil {
		return nil, false, err
	}
	if resp.Error != nil {
		return nil, false, resp.Error
	}
	result, ok := resp.Data.(GetNodeResponse)
	if !ok {
		return nil, false, fmt.Errorf("invalid response type")
	}
	return result.Node, result.Found, nil
}

func (a *DHTActor) TotalNodes() (int, error) {
	resp, err := a.Send(&Request{Type: MsgTotalNodes})
	if err != nil {
		return 0, err
	}
	if resp.Error != nil {
		return 0, resp.Error
	}
	total, ok := resp.Data.(int)
	if !ok {
		return 0, fmt.Errorf("invalid response type")
	}
	return total, nil
}

func (a *DHTActor) AddNode(node *NodeEntry) error {
	resp, err := a.Send(&Request{
		Type: MsgAddNode,
		Data: AddNodeRequest{Node: node},
	})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return resp.Error
	}
	return nil
}

func (a *DHTActor) RemoveNode(id NodeID) (bool, error) {
	resp, err := a.Send(&Request{
		Type: MsgRemoveNode,
		Data: RemoveNodeRequest{ID: id},
	})
	if err != nil {
		return false, err
	}
	if resp.Error != nil {
		return false, resp.Error
	}
	removed, ok := resp.Data.(bool)
	if !ok {
		return false, fmt.Errorf("invalid response type")
	}
	return removed, nil
}

func (a *DHTActor) UpdateNode(id NodeID, address string, reputation uint64) (bool, error) {
	resp, err := a.Send(&Request{
		Type: MsgUpdateNode,
		Data: UpdateNodeRequest{
			ID:         id,
			Address:    address,
			Reputation: reputation,
		},
	})
	if err != nil {
		return false, err
	}
	if resp.Error != nil {
		return false, resp.Error
	}
	updated, ok := resp.Data.(bool)
	if !ok {
		return false, fmt.Errorf("invalid response type")
	}
	return updated, nil
}

func (a *DHTActor) PingNode(id NodeID) (bool, error) {
	resp, err := a.Send(&Request{
		Type: MsgPingNode,
		Data: GetNodeRequest{ID: id},
	})
	if err != nil {
		return false, err
	}
	if resp.Error != nil {
		return false, resp.Error
	}
	alive, ok := resp.Data.(bool)
	if !ok {
		return false, fmt.Errorf("invalid response type")
	}
	return alive, nil
}

func (a *DHTActor) Shutdown() error {
	resp, err := a.Send(&Request{Type: MsgShutdown})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return resp.Error
	}
	a.Stop()
	return nil
}

func (a *DHTActor) GetRoutingTable() *RoutingTable {
	return a.routingTable
}

func (a *DHTActor) IsRunning() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.started
}
