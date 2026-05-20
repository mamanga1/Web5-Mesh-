// ============================================================================
// src/domain_resolution/resolver.go - .mesh Domain Resolver
// ============================================================================
// Especificación:
// - Resolución de dominios .mesh descentralizada
// - Mapea nombres de dominio a DIDs usando el DHT
// - Cache local con TTL para resolución rápida
// ============================================================================

package domain_resolution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"web5-mesh/src/dht"
)

// DomainRecord representa un registro de dominio en el DHT
type DomainRecord struct {
	Domain      string    `json:"domain"`
	DID         string    `json:"did"`
	PublicKey   []byte    `json:"public_key,omitempty"`
	IP          string    `json:"ip,omitempty"`
	Port        int       `json:"port,omitempty"`
	TTL         int       `json:"ttl"` // Tiempo de vida en segundos
	RegisteredAt time.Time `json:"registered_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	Signature   []byte    `json:"signature"`
}

// IsExpired verifica si el registro ha expirado
func (dr *DomainRecord) IsExpired() bool {
	return time.Now().After(dr.ExpiresAt)
}

// Resolver es el resolvedor de dominios .mesh
type Resolver struct {
	dhtEngine *dht.KadEngine
	cache     map[string]*CacheEntry
	cacheMu   sync.RWMutex
	ttl       time.Duration
}

// CacheEntry representa una entrada en caché
type CacheEntry struct {
	Record    *DomainRecord
	ExpiresAt time.Time
}

// NewResolver crea un nuevo resolvedor de dominios
func NewResolver(dhtEngine *dht.KadEngine) *Resolver {
	return &Resolver{
		dhtEngine: dhtEngine,
		cache:     make(map[string]*CacheEntry),
		ttl:       24 * time.Hour, // TTL por defecto: 24 horas
	}
}

// Resolve resuelve un dominio .mesh a un DID
func (r *Resolver) Resolve(ctx context.Context, domain string) (*DomainRecord, error) {
	// Normalizar dominio
	domain = strings.ToLower(strings.TrimSpace(domain))
	
	// Verificar formato .mesh
	if !strings.HasSuffix(domain, ".mesh") {
		return nil, fmt.Errorf("invalid .mesh domain: %s", domain)
	}

	// Verificar caché
	if record := r.getFromCache(domain); record != nil {
		return record, nil
	}

	// Buscar en DHT
	key := r.domainToKey(domain)
	value, err := r.dhtEngine.LookupValue(ctx, []byte(key))
	if err != nil {
		return nil, fmt.Errorf("domain not found: %w", err)
	}

	// Parsear registro
	record, err := r.parseRecord(domain, value)
	if err != nil {
		return nil, fmt.Errorf("failed to parse record: %w", err)
	}

	// Almacenar en caché
	r.addToCache(domain, record)

	return record, nil
}

// Register registra un dominio .mesh en el DHT
func (r *Resolver) Register(ctx context.Context, domain, did string, ttlSeconds int) error {
	// Normalizar dominio
	domain = strings.ToLower(strings.TrimSpace(domain))

	// Verificar formato
	if !strings.HasSuffix(domain, ".mesh") {
		return fmt.Errorf("invalid .mesh domain: %s", domain)
	}

	// Crear registro
	record := &DomainRecord{
		Domain:      domain,
		DID:         did,
		TTL:         ttlSeconds,
		RegisteredAt: time.Now(),
		ExpiresAt:   time.Now().Add(time.Duration(ttlSeconds) * time.Second),
	}

	// Serializar registro
	data, err := r.serializeRecord(record)
	if err != nil {
		return fmt.Errorf("failed to serialize record: %w", err)
	}

	// Almacenar en DHT
	key := r.domainToKey(domain)
	if err := r.dhtEngine.StoreValue(ctx, []byte(key), data); err != nil {
		return fmt.Errorf("failed to store in DHT: %w", err)
	}

	return nil
}

// Unregister elimina un dominio del DHT
func (r *Resolver) Unregister(ctx context.Context, domain string) error {
	domain = strings.ToLower(strings.TrimSpace(domain))
	
	// Eliminar del DHT (almacenar registro vacío con TTL corto)
	key := r.domainToKey(domain)
	if err := r.dhtEngine.StoreValue(ctx, []byte(key), []byte("")); err != nil {
		return fmt.Errorf("failed to unregister: %w", err)
	}

	// Eliminar de caché
	r.cacheMu.Lock()
	delete(r.cache, domain)
	r.cacheMu.Unlock()

	return nil
}

// domainToKey convierte un dominio a clave de búsqueda en DHT
func (r *Resolver) domainToKey(domain string) string {
	hash := sha256.Sum256([]byte("mesh:" + domain))
	return hex.EncodeToString(hash[:])
}

// getFromCache obtiene un registro de la caché
func (r *Resolver) getFromCache(domain string) *DomainRecord {
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()

	entry, exists := r.cache[domain]
	if !exists {
		return nil
	}

	if time.Now().After(entry.ExpiresAt) {
		// Entrada expirada, eliminar en background
		go func() {
			r.cacheMu.Lock()
			delete(r.cache, domain)
			r.cacheMu.Unlock()
		}()
		return nil
	}

	return entry.Record
}

// addToCache agrega un registro a la caché
func (r *Resolver) addToCache(domain string, record *DomainRecord) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	ttl := r.ttl
	if record.TTL > 0 {
		ttl = time.Duration(record.TTL) * time.Second
	}

	r.cache[domain] = &CacheEntry{
		Record:    record,
		ExpiresAt: time.Now().Add(ttl),
	}
}

// parseRecord parsea un registro desde bytes
func (r *Resolver) parseRecord(domain string, data []byte) (*DomainRecord, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty record for domain: %s", domain)
	}

	// Formato simple: "did:maia:xxx|ttl"
	parts := strings.Split(string(data), "|")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid record format")
	}

	did := parts[0]
	var ttl int = 86400 // Default 24h
	if len(parts) > 1 {
		fmt.Sscanf(parts[1], "%d", &ttl)
	}

	return &DomainRecord{
		Domain:      domain,
		DID:         did,
		TTL:         ttl,
		RegisteredAt: time.Now().Add(-time.Duration(ttl/2) * time.Second),
		ExpiresAt:   time.Now().Add(time.Duration(ttl) * time.Second),
	}, nil
}

// serializeRecord serializa un registro a bytes
func (r *Resolver) serializeRecord(record *DomainRecord) ([]byte, error) {
	// Formato simple: "did:maia:xxx|ttl"
	data := fmt.Sprintf("%s|%d", record.DID, record.TTL)
	return []byte(data), nil
}

// ResolveDID resuelve un dominio directamente a un DID string
func (r *Resolver) ResolveDID(ctx context.Context, domain string) (string, error) {
	record, err := r.Resolve(ctx, domain)
	if err != nil {
		return "", err
	}
	return record.DID, nil
}

// RefreshCache refresca la caché para un dominio
func (r *Resolver) RefreshCache(ctx context.Context, domain string) error {
	r.cacheMu.Lock()
	delete(r.cache, domain)
	r.cacheMu.Unlock()
	
	_, err := r.Resolve(ctx, domain)
	return err
}

// ClearCache limpia toda la caché
func (r *Resolver) ClearCache() {
	r.cacheMu.Lock()
	r.cache = make(map[string]*CacheEntry)
	r.cacheMu.Unlock()
}

// GetCacheStats retorna estadísticas de la caché
func (r *Resolver) GetCacheStats() map[string]interface{} {
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()

	active := 0
	for _, entry := range r.cache {
		if time.Now().Before(entry.ExpiresAt) {
			active++
		}
	}

	return map[string]interface{}{
		"total_entries": len(r.cache),
		"active_entries": active,
		"ttl_seconds":   r.ttl.Seconds(),
	}
}

// WellKnownDomains resuelve dominios conocidos (seed nodes)
func (r *Resolver) WellKnownDomains() map[string]string {
	return map[string]string{
		"relay.argentina.mesh":  "did:maia:mamanga1-relay-argentina",
		"relay.us-east.mesh":     "did:maia:mamanga1-relay-us-east",
		"relay.europe.mesh":      "did:maia:mamanga1-relay-europe",
		"wallet.4sk.mesh":        "did:maia:4sk-wallet",
		"api.iap2p.mesh":         "did:maia:iap2p-api",
	}
}
