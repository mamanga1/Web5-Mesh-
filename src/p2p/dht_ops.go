package p2p

// Store guarda un valor en el DHT local
func (k *Kademlia) Store(key string, value []byte) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.dataStore == nil {
		k.dataStore = make(map[string][]byte)
	}
	k.dataStore[key] = value
	return nil
}

// FindValue busca un valor en el DHT local
func (k *Kademlia) FindValue(key string) ([]byte, bool) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	if k.dataStore == nil {
		return nil, false
	}
	val, ok := k.dataStore[key]
	return val, ok
}
