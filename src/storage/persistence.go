// ============================================================================
// src/storage/persistence.go - BadgerDB Embedded KV Store
// ============================================================================

package storage

import (
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	badger "github.com/dgraph-io/badger/v4"
)

type PersistenceStore struct {
	db     *badger.DB
	path   string
	mu     sync.RWMutex
	closed bool
}

type StoreOptions struct {
	Path              string
	InMemory          bool
	SyncWrites        bool
	ValueLogFileSize  int64
	MemTableSize      int64
	NumMemtables      int
	Compression       bool
	GCIntervalSeconds int
}

func DefaultStoreOptions(path string) *StoreOptions {
	return &StoreOptions{
		Path:              path,
		InMemory:          false,
		SyncWrites:        false,
		ValueLogFileSize:  1024 * 1024 * 1024,
		MemTableSize:      64 * 1024 * 1024,
		NumMemtables:      2,
		Compression:       true,
		GCIntervalSeconds: 300,
	}
}

func NewPersistenceStore(opts *StoreOptions) (*PersistenceStore, error) {
	if opts == nil {
		return nil, fmt.Errorf("options cannot be nil")
	}

	badgerOpts := badger.DefaultOptions(opts.Path)

	if opts.InMemory {
		badgerOpts = badgerOpts.WithInMemory(true)
	}

	badgerOpts = badgerOpts.WithSyncWrites(opts.SyncWrites)
	badgerOpts = badgerOpts.WithValueLogFileSize(opts.ValueLogFileSize)
	badgerOpts = badgerOpts.WithMemTableSize(opts.MemTableSize)
	badgerOpts = badgerOpts.WithNumMemtables(opts.NumMemtables)

	// Si no hay compresión, no hacer nada (badger usa zstd por defecto)
	// Simplemente omitimos la configuración de compresión

	db, err := badger.Open(badgerOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to open BadgerDB: %w", err)
	}

	store := &PersistenceStore{
		db:     db,
		path:   opts.Path,
		closed: false,
	}

	if !opts.InMemory && opts.GCIntervalSeconds > 0 {
		go store.periodicGC(time.Duration(opts.GCIntervalSeconds) * time.Second)
	}

	return store, nil
}

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

func (s *PersistenceStore) Has(key []byte) (bool, error) {
	_, exists, err := s.Get(key)
	return exists, err
}

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

func (s *PersistenceStore) Iterate(prefix []byte, fn func(key []byte, value []byte) bool) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	return s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		if len(prefix) > 0 {
			it.Seek(prefix)
		} else {
			it.Rewind()
		}

		for it.Valid() {
			item := it.Item()
			key := item.KeyCopy(nil)

			if len(prefix) > 0 && !hasPrefix(key, prefix) {
				break
			}

			val, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}

			if !fn(key, val) {
				break
			}

			it.Next()
		}

		return nil
	})
}

func (s *PersistenceStore) GetSize() (int64, error) {
	if s.closed {
		return 0, fmt.Errorf("store is closed")
	}

	lsmSize, vlogSize := s.db.Size()
	return lsmSize + vlogSize, nil
}

func (s *PersistenceStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	return s.db.Close()
}

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

		s.runGC()
	}
}

func (s *PersistenceStore) runGC() {
	err := s.db.RunValueLogGC(0.5)
	if err != nil && err != badger.ErrNoRewrite {
		_ = err
	}
}

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

func (s *PersistenceStore) GetSequence(sequenceName []byte, initialValue uint64) (*badger.Sequence, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, fmt.Errorf("store is closed")
	}

	return s.db.GetSequence(sequenceName, initialValue)
}

func Uint64ToBytes(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

func BytesToUint64(b []byte) uint64 {
	if len(b) < 8 {
		return 0
	}
	return binary.BigEndian.Uint64(b)
}
