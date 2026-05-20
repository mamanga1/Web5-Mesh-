// ============================================================================
// src/storage/persistence.go - BadgerDB Embedded KV Store
// ============================================================================
// Especificación:
// - Integración nativa de BadgerDB (motor Key-Value optimizado en LSM-tree)
// - Almacenamiento de documentos locales y estado inmutable de la red
// - Métodos transaccionales atómicos Put, Get, Delete
// ============================================================================

package storage

import (
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	badger "github.com/dgraph-io/badger/v4"
)

// PersistenceStore representa el almacenamiento persistente BadgerDB
type PersistenceStore struct {
	db     *badger.DB
	path   string
	mu     sync.RWMutex
	closed bool
}

// StoreOptions configuración del almacenamiento
type StoreOptions struct {
	Path              string        // Ruta del directorio de datos
	InMemory          bool          // Modo en memoria (para pruebas)
	SyncWrites        bool          // Sincronizar escrituras a disco
	ValueLogFileSize  int64         // Tamaño del archivo de log de valores (default: 1GB)
	MemTableSize      int64         // Tamaño de la memtable (default: 64MB)
	NumMemtables      int           // Número de memtables (default: 2)
	Compression       bool          // Habilitar compresión
	GCIntervalSeconds int           // Intervalo de garbage collection (segundos)
}

// DefaultStoreOptions retorna configuración por defecto optimizada para TV boxes y Xeon
func DefaultStoreOptions(path string) *StoreOptions {
	return &StoreOptions{
		Path:              path,
		InMemory:          false,
		SyncWrites:        false,
		ValueLogFileSize:  1024 * 1024 * 1024, // 1GB
		MemTableSize:      64 * 1024 * 1024,   // 64MB
		NumMemtables:      2,
		Compression:       true,
		GCIntervalSeconds: 300, // 5 minutos
	}
}

// NewPersistenceStore crea e inicializa una nueva instancia de BadgerDB
func NewPersistenceStore(opts *StoreOptions) (*PersistenceStore, error) {
	if opts == nil {
		return nil, fmt.Errorf("options cannot be nil")
	}

	badgerOpts := badger.DefaultOptions(opts.Path)

	// Configurar opciones
	if opts.InMemory {
		badgerOpts = badgerOpts.WithInMemory(true)
	}

	badgerOpts = badgerOpts.WithSyncWrites(opts.SyncWrites)
	badgerOpts = badgerOpts.WithValueLogFileSize(opts.ValueLogFileSize)
	badgerOpts = badgerOpts.WithMemTableSize(opts.MemTableSize)
	badgerOpts = badgerOpts.WithNumMemtables(opts.NumMemtables)

	if !opts.Compression {
		badgerOpts = badgerOpts.WithCompression(badger.None)
	}

	// Abrir base de datos
	db, err := badger.Open(badgerOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to open BadgerDB: %w", err)
	}

	store := &PersistenceStore{
		db:     db,
		path:   opts.Path,
		closed: false,
	}

	// Iniciar garbage collection en background si no es in-memory
	if !opts.InMemory && opts.GCIntervalSeconds > 0 {
		go store.periodicGC(time.Duration(opts.GCIntervalSeconds) * time.Second)
	}

	return store, nil
}

// Put almacena un par clave-valor en la base de datos
func (s *PersistenceStore) Put(key []byte, value []byte) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	if len(key) == 0 {
		return fmt.Errorf("key cannot be empty")
	}

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, value)
	})
}

// PutWithTTL almacena un par clave-valor con tiempo de expiración (TTL en segundos)
func (s *PersistenceStore) PutWithTTL(key []byte, value []byte, ttlSeconds uint32) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	if len(key) == 0 {
		return fmt.Errorf("key cannot be empty")
	}

	return s.db.Update(func(txn *badger.Txn) error {
		entry := badger.NewEntry(key, value).WithTTL(time.Duration(ttlSeconds) * time.Second)
		return txn.SetEntry(entry)
	})
}

// Get recupera un valor por su clave
// Retorna (value, true) si existe, (nil, false) si no
func (s *PersistenceStore) Get(key []byte) ([]byte, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, false, fmt.Errorf("store is closed")
	}

	var result []byte
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return nil
			}
			return err
		}

		// Copiar el valor para evitar que sea inválido fuera de la transacción
		val, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		result = val
		return nil
	})

	if err != nil {
		return nil, false, err
	}

	return result, result != nil, nil
}

// Delete elimina una clave de la base de datos
func (s *PersistenceStore) Delete(key []byte) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
}

// Has verifica si una clave existe
func (s *PersistenceStore) Has(key []byte) (bool, error) {
	_, exists, err := s.Get(key)
	return exists, err
}

// BatchWrite realiza múltiples escrituras en una sola transacción
func (s *PersistenceStore) BatchWrite(operations map[string][]byte) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	return s.db.Update(func(txn *badger.Txn) error {
		for key, value := range operations {
			if err := txn.Set([]byte(key), value); err != nil {
				return err
			}
		}
		return nil
	})
}

// Iterate recorre todas las claves que coinciden con un prefijo
func (s *PersistenceStore) Iterate(prefix []byte, fn func(key []byte, value []byte) bool) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	return s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		// Si hay prefijo, posicionarse al inicio del prefijo
		if len(prefix) > 0 {
			it.Seek(prefix)
		} else {
			it.Rewind()
		}

		for it.Valid() {
			item := it.Item()
			key := item.KeyCopy(nil)

			// Verificar prefijo
			if len(prefix) > 0 && !hasPrefix(key, prefix) {
				break
			}

			val, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}

			// Llamar callback, si retorna false detener iteración
			if !fn(key, val) {
				break
			}

			it.Next()
		}

		return nil
	})
}

// GetSize retorna el tamaño aproximado de la base de datos en bytes
func (s *PersistenceStore) GetSize() (int64, error) {
	if s.closed {
		return 0, fmt.Errorf("store is closed")
	}

	lsmSize, vlogSize := s.db.Size()
	return lsmSize + vlogSize, nil
}

// Close cierra la base de datos correctamente
func (s *PersistenceStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	return s.db.Close()
}

// periodicGC ejecuta garbage collection periódicamente
func (s *PersistenceStore) periodicGC(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		if s.closed {
			return
		}

		s.mu.RLock()
		if s.closed {
			s.mu.RUnlock()
			return
		}
		s.mu.RUnlock()

		// Ejecutar GC
		s.runGC()
	}
}

// runGC ejecuta una ronda de garbage collection
func (s *PersistenceStore) runGC() {
	err := s.db.RunValueLogGC(0.5) // Eliminar 50% de logs muertos
	if err != nil && err != badger.ErrNoRewrite {
		// Log de error pero no fallar
		_ = err
	}
}

// hasPrefix verifica si una slice tiene un prefijo específico
func hasPrefix(key, prefix []byte) bool {
	if len(key) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if key[i] != prefix[i] {
			return false
		}
	}
	return true
}

// GetSequence crea o recupera una secuencia atómica (para IDs autoincrementales)
func (s *PersistenceStore) GetSequence(sequenceName []byte, initialValue uint64) (*badger.Sequence, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, fmt.Errorf("store is closed")
	}

	return s.db.GetSequence(sequenceName, initialValue)
}

// Uint64ToBytes convierte uint64 a bytes (big-endian)
func Uint64ToBytes(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

// BytesToUint64 convierte bytes a uint64 (big-endian)
func BytesToUint64(b []byte) uint64 {
	if len(b) < 8 {
		return 0
	}
	return binary.BigEndian.Uint64(b)
}
