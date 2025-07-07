package storage

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type KeyType string

const (
	KeyTypeRSA     KeyType = "RSA"
	KeyTypeECDSA   KeyType = "ECDSA"
	KeyTypeEd25519 KeyType = "Ed25519"
)

type StoredKey struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Size        int       `json:"size"`
	CreatedAt   time.Time `json:"created_at"`
	PublicKey   string    `json:"public_key"`
	PrivateKey  string    `json:"private_key"`
	BenchmarkID string    `json:"benchmark_id"`
	FileStored  bool      `json:"file_stored"`
	FilePath    string    `json:"file_path,omitempty"`
}

type KeyStore struct {
	mu   sync.RWMutex
	keys map[string]*StoredKey
}

func NewKeyStore() *KeyStore {
	return &KeyStore{
		keys: make(map[string]*StoredKey),
	}
}

func (ks *KeyStore) StoreRSAKey(key *rsa.PrivateKey, size int, benchmarkID string) (*StoredKey, error) {
	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	storedKey := &StoredKey{
		ID:          uuid.New().String(),
		Type:        string(KeyTypeRSA),
		Size:        size,
		CreatedAt:   time.Now(),
		PublicKey:   string(publicKeyPEM),
		PrivateKey:  string(privateKeyPEM),
		BenchmarkID: benchmarkID,
	}

	ks.mu.Lock()
	ks.keys[storedKey.ID] = storedKey
	ks.mu.Unlock()

	return storedKey, nil
}

func (ks *KeyStore) StoreECDSAKey(key *ecdsa.PrivateKey, size int, benchmarkID string) (*StoredKey, error) {
	privateKeyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	storedKey := &StoredKey{
		ID:          uuid.New().String(),
		Type:        string(KeyTypeECDSA),
		Size:        size,
		CreatedAt:   time.Now(),
		PublicKey:   string(publicKeyPEM),
		PrivateKey:  string(privateKeyPEM),
		BenchmarkID: benchmarkID,
	}

	ks.mu.Lock()
	ks.keys[storedKey.ID] = storedKey
	ks.mu.Unlock()

	return storedKey, nil
}

func (ks *KeyStore) StoreEd25519Key(publicKey ed25519.PublicKey, privateKey ed25519.PrivateKey, benchmarkID string) (*StoredKey, error) {
	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	storedKey := &StoredKey{
		ID:          uuid.New().String(),
		Type:        string(KeyTypeEd25519),
		Size:        256,
		CreatedAt:   time.Now(),
		PublicKey:   string(publicKeyPEM),
		PrivateKey:  string(privateKeyPEM),
		BenchmarkID: benchmarkID,
	}

	ks.mu.Lock()
	ks.keys[storedKey.ID] = storedKey
	ks.mu.Unlock()

	return storedKey, nil
}

func (ks *KeyStore) GetKey(id string) (*StoredKey, bool) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	
	key, exists := ks.keys[id]
	return key, exists
}

func (ks *KeyStore) GetKeysByBenchmark(benchmarkID string) []*StoredKey {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	
	var keys []*StoredKey
	for _, key := range ks.keys {
		if key.BenchmarkID == benchmarkID {
			keys = append(keys, key)
		}
	}
	return keys
}

func (ks *KeyStore) DeleteKey(id string) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	delete(ks.keys, id)
}

func (ks *KeyStore) StoreKeyReference(key *StoredKey) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	ks.keys[key.ID] = key
}

func (ks *KeyStore) GetAllKeys() []*StoredKey {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	
	keys := make([]*StoredKey, 0, len(ks.keys))
	for _, key := range ks.keys {
		keys = append(keys, key)
	}
	return keys
}

func (ks *KeyStore) CountFileStoredKeys() int {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	
	count := 0
	for _, key := range ks.keys {
		if key.FileStored {
			count++
		}
	}
	return count
}