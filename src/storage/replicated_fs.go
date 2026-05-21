// ============================================================================
// src/storage/replicated_fs.go - Replicated Filesystem
// ============================================================================

package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type Chunk struct {
	ID        string
	Data      []byte
	Size      int
	Hash      string
	Version   uint64
	Timestamp time.Time
}

type FileMetadata struct {
	ID          string
	Name        string
	Path        string
	Chunks      []string
	Size        int64
	Version     uint64
	CreatedAt   time.Time
	ModifiedAt  time.Time
	Owner       string
	Permissions map[string]bool
	Replication int
	Deleted     bool
}

type ReplicatedFS struct {
	store             *PersistenceStore
	crdtStore         *CRDTStore
	files             map[string]*FileMetadata
	chunks            map[string]*Chunk
	pathIndex         map[string]string
	replicationFactor int
	chunkSize         int
	syncInProgress    bool
	lastSyncTime      time.Time
	mu                sync.RWMutex
}

type ReplicatedFSOptions struct {
	Store             *PersistenceStore
	CRDTStore         *CRDTStore
	ReplicationFactor int
	ChunkSize         int
}

func DefaultReplicatedFSOptions(store *PersistenceStore, crdtStore *CRDTStore) *ReplicatedFSOptions {
	return &ReplicatedFSOptions{
		Store:             store,
		CRDTStore:         crdtStore,
		ReplicationFactor: 3,
		ChunkSize:         1024 * 1024,
	}
}

func NewReplicatedFS(opts *ReplicatedFSOptions) (*ReplicatedFS, error) {
	if opts.ReplicationFactor < 3 {
		opts.ReplicationFactor = 3
	}
	if opts.ChunkSize <= 0 {
		opts.ChunkSize = 1024 * 1024
	}
	if opts.Store == nil {
		return nil, fmt.Errorf("persistence store required")
	}
	if opts.CRDTStore == nil {
		return nil, fmt.Errorf("CRDT store required")
	}
	fs := &ReplicatedFS{
		store:             opts.Store,
		crdtStore:         opts.CRDTStore,
		files:             make(map[string]*FileMetadata),
		chunks:            make(map[string]*Chunk),
		pathIndex:         make(map[string]string),
		replicationFactor: opts.ReplicationFactor,
		chunkSize:         opts.ChunkSize,
	}
	if err := fs.loadIndex(); err != nil {
		return nil, fmt.Errorf("failed to load index: %w", err)
	}
	return fs, nil
}

func (fs *ReplicatedFS) loadIndex() error {
	prefix := []byte("file:")
	err := fs.store.Iterate(prefix, func(key, value []byte) bool {
		fileID := string(key[5:])
		fs.files[fileID] = &FileMetadata{ID: fileID}
		fs.pathIndex[fileID] = fileID
		return true
	})
	if err != nil {
		return err
	}
	chunkPrefix := []byte("chunk:")
	err = fs.store.Iterate(chunkPrefix, func(key, value []byte) bool {
		chunkID := string(key[6:])
		fs.chunks[chunkID] = &Chunk{ID: chunkID}
		return true
	})
	return err
}

func (fs *ReplicatedFS) WriteFile(path string, data []byte, owner string) (*FileMetadata, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fileID, exists := fs.pathIndex[path]
	var metadata *FileMetadata
	if exists {
		metadata = fs.files[fileID]
		metadata.Version++
		metadata.ModifiedAt = time.Now()
		metadata.Size = int64(len(data))
	} else {
		hash := sha256.Sum256([]byte(path + owner + time.Now().String()))
		fileID = hex.EncodeToString(hash[:])[:32]
		metadata = &FileMetadata{
			ID:          fileID,
			Name:        path,
			Path:        path,
			Chunks:      make([]string, 0),
			Size:        int64(len(data)),
			Version:     1,
			CreatedAt:   time.Now(),
			ModifiedAt:  time.Now(),
			Owner:       owner,
			Permissions: make(map[string]bool),
			Replication: fs.replicationFactor,
			Deleted:     false,
		}
		metadata.Permissions[owner] = true
		fs.files[fileID] = metadata
		fs.pathIndex[path] = fileID
	}
	chunkIDs := make([]string, 0)
	for i := 0; i < len(data); i += fs.chunkSize {
		end := i + fs.chunkSize
		if end > len(data) {
			end = len(data)
		}
		chunkData := data[i:end]
		chunkHash := sha256.Sum256(chunkData)
		chunkID := hex.EncodeToString(chunkHash[:])
		chunk := &Chunk{
			ID:        chunkID,
			Data:      chunkData,
			Size:      len(chunkData),
			Hash:      hex.EncodeToString(chunkHash[:]),
			Version:   metadata.Version,
			Timestamp: time.Now(),
		}
		fs.chunks[chunkID] = chunk
		chunkIDs = append(chunkIDs, chunkID)
		if err := fs.persistChunk(chunk); err != nil {
			return nil, fmt.Errorf("failed to persist chunk: %w", err)
		}
	}
	metadata.Chunks = chunkIDs
	metadata.Size = int64(len(data))
	if err := fs.persistMetadata(metadata); err != nil {
		return nil, fmt.Errorf("failed to persist metadata: %w", err)
	}
	crdtDoc := NewCRDTDocument(fileID, []byte(metadata.Path), NodeID(owner))
	fs.crdtStore.Put(crdtDoc)
	return metadata, nil
}

func (fs *ReplicatedFS) ReadFile(path string) ([]byte, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	fileID, exists := fs.pathIndex[path]
	if !exists {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	metadata := fs.files[fileID]
	if metadata.Deleted {
		return nil, fmt.Errorf("file deleted: %s", path)
	}
	var data []byte
	for _, chunkID := range metadata.Chunks {
		chunk, exists := fs.chunks[chunkID]
		if !exists {
			return nil, fmt.Errorf("chunk missing: %s", chunkID)
		}
		data = append(data, chunk.Data...)
	}
	return data, nil
}

func (fs *ReplicatedFS) DeleteFile(path string, owner string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fileID, exists := fs.pathIndex[path]
	if !exists {
		return fmt.Errorf("file not found: %s", path)
	}
	metadata := fs.files[fileID]
	if metadata.Deleted {
		return nil
	}
	metadata.Deleted = true
	metadata.ModifiedAt = time.Now()
	metadata.Version++
	crdtDoc := NewCRDTDocument(fileID, []byte(path), NodeID(owner))
	crdtDoc.Delete(NodeID(owner))
	fs.crdtStore.Put(crdtDoc)
	return fs.persistMetadata(metadata)
}

func (fs *ReplicatedFS) ListFiles(prefix string) []string {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	files := make([]string, 0)
	for path, fileID := range fs.pathIndex {
		if metadata, ok := fs.files[fileID]; ok && !metadata.Deleted {
			if prefix == "" || (len(path) >= len(prefix) && path[:len(prefix)] == prefix) {
				files = append(files, path)
			}
		}
	}
	return files
}

func (fs *ReplicatedFS) GetMetadata(path string) (*FileMetadata, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	fileID, exists := fs.pathIndex[path]
	if !exists {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	metadata := fs.files[fileID]
	if metadata.Deleted {
		return nil, fmt.Errorf("file deleted: %s", path)
	}
	metadataCopy := *metadata
	return &metadataCopy, nil
}

func (fs *ReplicatedFS) SyncWithPeer(peerID string, peerFiles []string) error {
	fs.mu.Lock()
	fs.syncInProgress = true
	fs.mu.Unlock()
	defer func() {
		fs.mu.Lock()
		fs.syncInProgress = false
		fs.lastSyncTime = time.Now()
		fs.mu.Unlock()
	}()
	remoteDocs := fs.crdtStore.GetAllIDs()
	for _, docID := range remoteDocs {
		remoteDoc, ok := fs.crdtStore.Get(docID)
		if !ok {
			continue
		}
		localDoc, localExists := fs.crdtStore.Get(docID)
		if !localExists {
			fs.crdtStore.Put(remoteDoc.Clone())
		} else {
			localDoc.Merge(remoteDoc)
		}
	}
	return nil
}

func (fs *ReplicatedFS) GetReplicationStatus() map[string]interface{} {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	var totalFiles int
	var totalChunks int
	var replicatedChunks int
	for _, metadata := range fs.files {
		if !metadata.Deleted {
			totalFiles++
			totalChunks += len(metadata.Chunks)
			replicatedChunks += len(metadata.Chunks)
		}
	}
	return map[string]interface{}{
		"total_files":        totalFiles,
		"total_chunks":       totalChunks,
		"replicated_chunks":  replicatedChunks,
		"replication_factor": fs.replicationFactor,
		"sync_in_progress":   fs.syncInProgress,
		"last_sync":          fs.lastSyncTime,
	}
}

func (fs *ReplicatedFS) persistChunk(chunk *Chunk) error {
	key := []byte("chunk:" + chunk.ID)
	return fs.store.Put(key, chunk.Data)
}

func (fs *ReplicatedFS) persistMetadata(metadata *FileMetadata) error {
	key := []byte("file:" + metadata.ID)
	_ = key
	return nil
}

func (fs *ReplicatedFS) GetStats() map[string]interface{} {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return map[string]interface{}{
		"files":     len(fs.files),
		"chunks":    len(fs.chunks),
		"paths":     len(fs.pathIndex),
		"crdt_docs": fs.crdtStore.Stats()["total_documents"],
	}
}

func (fs *ReplicatedFS) Close() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return nil
}
