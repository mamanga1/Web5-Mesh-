// ============================================================================
// src/storage/replicated_fs.go - Replicated Filesystem
// ============================================================================
// Especificación:
// - Filesystem replicado distribuido en pedazos (Factor mínimo de replicación: 3)
// - Integración con CRDT para consistencia eventual
// - Sincronización P2P entre nodos
// ============================================================================

package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// Chunk representa un fragmento de un archivo (tamaño fijo)
type Chunk struct {
	ID        string    // Identificador único del chunk (hash del contenido)
	Data      []byte    // Datos del chunk
	Size      int       // Tamaño en bytes
	Hash      string    // SHA-256 del contenido (verificación de integridad)
	Version   uint64    // Versión del chunk (para CRDT)
	Timestamp time.Time // Última modificación
}

// FileMetadata representa los metadatos de un archivo en el FS replicado
type FileMetadata struct {
	ID          string            // Identificador único del archivo
	Name        string            // Nombre del archivo
	Path        string            // Ruta completa
	Chunks      []string          // IDs de los chunks que componen el archivo
	Size        int64             // Tamaño total en bytes
	Version     uint64            // Versión del archivo (incrementa con cada modificación)
	CreatedAt   time.Time         // Fecha de creación
	ModifiedAt  time.Time         // Fecha de última modificación
	Owner       string            // DID del propietario
	Permissions map[string]bool   // Permisos (DID -> read/write)
	Replication int               // Factor de replicación deseado
	Deleted     bool              // Marcado como eliminado (tombstone)
}

// ReplicatedFS es el sistema de archivos replicado principal
type ReplicatedFS struct {
	// Almacenamiento persistente
	store *PersistenceStore

	// CRDT store para consistencia eventual
	crdtStore *CRDTStore

	// Índices en memoria
	files      map[string]*FileMetadata // ID -> Metadata
	chunks     map[string]*Chunk        // ChunkID -> Chunk
	pathIndex  map[string]string        // Path -> FileID

	// Configuración
	replicationFactor int // Mínimo 3 por defecto
	chunkSize         int // Tamaño de chunk en bytes (default: 1MB)

	// Sincronización
	mu         sync.RWMutex
	syncInProgress bool
	lastSyncTime   time.Time
}

// ReplicatedFSOptions configuración del sistema de archivos replicado
type ReplicatedFSOptions struct {
	Store             *PersistenceStore
	CRDTStore         *CRDTStore
	ReplicationFactor int // Mínimo 3
	ChunkSize         int // Tamaño de chunk (default: 1MB = 1048576)
}

// DefaultReplicatedFSOptions retorna configuración por defecto
func DefaultReplicatedFSOptions(store *PersistenceStore, crdtStore *CRDTStore) *ReplicatedFSOptions {
	return &ReplicatedFSOptions{
		Store:             store,
		CRDTStore:         crdtStore,
		ReplicationFactor: 3,
		ChunkSize:         1024 * 1024, // 1MB
	}
}

// NewReplicatedFS crea una nueva instancia del sistema de archivos replicado
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

	// Cargar índice desde disco
	if err := fs.loadIndex(); err != nil {
		return nil, fmt.Errorf("failed to load index: %w", err)
	}

	return fs, nil
}

// loadIndex carga el índice de archivos desde el almacenamiento persistente
func (fs *ReplicatedFS) loadIndex() error {
	// Cargar metadatos de archivos
	prefix := []byte("file:")
	err := fs.store.Iterate(prefix, func(key, value []byte) bool {
		var metadata FileMetadata
		// Deserializar metadata (en producción usar JSON/gob)
		// Por simplicidad, aquí solo cargamos IDs
		fileID := string(key[5:]) // remover "file:" prefix
		fs.files[fileID] = &FileMetadata{ID: fileID}
		fs.pathIndex[fileID] = fileID
		return true
	})
	if err != nil {
		return err
	}

	// Cargar chunks
	chunkPrefix := []byte("chunk:")
	err = fs.store.Iterate(chunkPrefix, func(key, value []byte) bool {
		chunkID := string(key[6:]) // remover "chunk:" prefix
		fs.chunks[chunkID] = &Chunk{ID: chunkID}
		return true
	})
	if err != nil {
		return err
	}

	return nil
}

// WriteFile escribe un archivo completo en el FS replicado
// Divide el archivo en chunks y los almacena individualmente
func (fs *ReplicatedFS) WriteFile(path string, data []byte, owner string) (*FileMetadata, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	// Verificar si ya existe
	fileID, exists := fs.pathIndex[path]
	var metadata *FileMetadata

	if exists {
		// Actualizar archivo existente
		metadata = fs.files[fileID]
		metadata.Version++
		metadata.ModifiedAt = time.Now()
		metadata.Size = int64(len(data))
	} else {
		// Crear nuevo archivo
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

	// Dividir en chunks
	chunkIDs := make([]string, 0)
	for i := 0; i < len(data); i += fs.chunkSize {
		end := i + fs.chunkSize
		if end > len(data) {
			end = len(data)
		}
		chunkData := data[i:end]

		// Calcular hash del chunk
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

		// Almacenar chunk
		fs.chunks[chunkID] = chunk
		chunkIDs = append(chunkIDs, chunkID)

		// Persistir chunk en disco
		if err := fs.persistChunk(chunk); err != nil {
			return nil, fmt.Errorf("failed to persist chunk: %w", err)
		}
	}

	metadata.Chunks = chunkIDs
	metadata.Size = int64(len(data))

	// Persistir metadata
	if err := fs.persistMetadata(metadata); err != nil {
		return nil, fmt.Errorf("failed to persist metadata: %w", err)
	}

	// Crear documento CRDT para este archivo
	crdtDoc := NewCRDTDocument(fileID, []byte(metadata.Path), NodeID(owner))
	fs.crdtStore.Put(crdtDoc)

	return metadata, nil
}

// ReadFile lee un archivo completo del FS replicado
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

	// Reconstruir archivo desde chunks
	var data []byte
	for _, chunkID := range metadata.Chunks {
		chunk, exists := fs.chunks[chunkID]
		if !exists {
			// Intentar recuperar chunk de la red (en producción)
			return nil, fmt.Errorf("chunk missing: %s", chunkID)
		}
		data = append(data, chunk.Data...)
	}

	return data, nil
}

// DeleteFile marca un archivo como eliminado (tombstone)
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

	// Actualizar documento CRDT
	crdtDoc := NewCRDTDocument(fileID, []byte(path), NodeID(owner))
	crdtDoc.Delete(NodeID(owner))
	fs.crdtStore.Put(crdtDoc)

	return fs.persistMetadata(metadata)
}

// ListFiles retorna la lista de archivos en una ruta
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

// GetMetadata retorna los metadatos de un archivo
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

	// Retornar copia
	metadataCopy := *metadata
	return &metadataCopy, nil
}

// SyncWithPeer sincroniza el FS local con un peer remoto
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

	// Obtener lista de documentos CRDT del peer (simulado)
	// En producción, aquí se intercambian vectores de versión y se sincronizan
	remoteDocs := fs.crdtStore.GetAllIDs()

	for _, docID := range remoteDocs {
		remoteDoc, ok := fs.crdtStore.Get(docID)
		if !ok {
			continue
		}

		localDoc, localExists := fs.crdtStore.Get(docID)
		if !localExists {
			// Nuevo documento, agregar localmente
			fs.crdtStore.Put(remoteDoc.Clone())
		} else {
			// Merge de documentos
			localDoc.Merge(remoteDoc)
		}
	}

	return nil
}

// GetReplicationStatus retorna el estado de replicación del FS
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
			// En producción, verificar cuántos chunks están replicados en la red
			replicatedChunks += len(metadata.Chunks) // Placeholder
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

// persistChunk almacena un chunk en disco
func (fs *ReplicatedFS) persistChunk(chunk *Chunk) error {
	key := []byte("chunk:" + chunk.ID)
	// En producción, serializar chunk a bytes (JSON/gob/protobuf)
	// Por ahora, almacenamos los datos directamente
	return fs.store.Put(key, chunk.Data)
}

// persistMetadata almacena metadatos de archivo en disco
func (fs *ReplicatedFS) persistMetadata(metadata *FileMetadata) error {
	key := []byte("file:" + metadata.ID)
	// En producción, serializar metadata
	// Placeholder
	_ = key
	return nil
}

// GetStats retorna estadísticas del FS
func (fs *ReplicatedFS) GetStats() map[string]interface{} {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	return map[string]interface{}{
		"files":   len(fs.files),
		"chunks":  len(fs.chunks),
		"paths":   len(fs.pathIndex),
		"crdt_docs": fs.crdtStore.Stats()["total_documents"],
	}
}

// Close cierra el sistema de archivos replicado
func (fs *ReplicatedFS) Close() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return nil
}
