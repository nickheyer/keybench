package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// Keys larger than 10MB are automatically stored to file
	FileStorageThreshold = 10 * 1024 * 1024
	// Chunk size for streaming large keys
	DefaultChunkSize = 64 * 1024 // 64KB chunks
)

type FileStorage struct {
	basePath string
	mu       sync.RWMutex
	files    map[string]*FileKeyInfo
}

type FileKeyInfo struct {
	ID           string    `json:"id"`
	Path         string    `json:"path"`
	Size         int64     `json:"size"`
	Hash         string    `json:"hash"`
	CreatedAt    time.Time `json:"created_at"`
	LastAccessed time.Time `json:"last_accessed"`
	BenchmarkID  string    `json:"benchmark_id"`
}

func NewFileStorage(basePath string) (*FileStorage, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}
	
	// Create subdirectories
	for _, dir := range []string{"keys", "temp", "archive"} {
		if err := os.MkdirAll(filepath.Join(basePath, dir), 0755); err != nil {
			return nil, err
		}
	}
	
	return &FileStorage{
		basePath: basePath,
		files:    make(map[string]*FileKeyInfo),
	}, nil
}

// StreamingWriter for writing large keys in chunks
type StreamingKeyWriter struct {
	file     *os.File
	hash     hash.Hash
	size     int64
	keyID    string
	storage  *FileStorage
	tempPath string
	finalPath string
}

func (fs *FileStorage) CreateStreamingWriter(keyID, keyType string) (*StreamingKeyWriter, error) {
	tempPath := filepath.Join(fs.basePath, "temp", fmt.Sprintf("%s_%s.tmp", keyID, keyType))
	finalPath := filepath.Join(fs.basePath, "keys", fmt.Sprintf("%s_%s.pem", keyID, keyType))
	
	file, err := os.Create(tempPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	
	return &StreamingKeyWriter{
		file:      file,
		hash:      sha256.New(),
		size:      0,
		keyID:     keyID,
		storage:   fs,
		tempPath:  tempPath,
		finalPath: finalPath,
	}, nil
}

func (w *StreamingKeyWriter) Write(p []byte) (n int, err error) {
	n, err = w.file.Write(p)
	if err != nil {
		return n, err
	}
	
	w.hash.Write(p[:n])
	w.size += int64(n)
	return n, nil
}

func (w *StreamingKeyWriter) Close() error {
	if err := w.file.Close(); err != nil {
		return err
	}
	
	// Move from temp to final location
	if err := os.Rename(w.tempPath, w.finalPath); err != nil {
		os.Remove(w.tempPath)
		return fmt.Errorf("failed to move key file: %w", err)
	}
	
	// Register the file
	info := &FileKeyInfo{
		ID:           w.keyID,
		Path:         w.finalPath,
		Size:         w.size,
		Hash:         hex.EncodeToString(w.hash.Sum(nil)),
		CreatedAt:    time.Now(),
		LastAccessed: time.Now(),
	}
	
	w.storage.mu.Lock()
	w.storage.files[w.keyID] = info
	w.storage.mu.Unlock()
	
	return nil
}

// Cleanup old files
func (fs *FileStorage) CleanupOldFiles(olderThan time.Duration) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	
	cutoff := time.Now().Add(-olderThan)
	var toDelete []string
	
	for id, info := range fs.files {
		if info.LastAccessed.Before(cutoff) {
			toDelete = append(toDelete, id)
		}
	}
	
	for _, id := range toDelete {
		if info, exists := fs.files[id]; exists {
			os.Remove(info.Path)
			delete(fs.files, id)
		}
	}
	
	log.Printf("Cleaned up %d old key files", len(toDelete))
	return nil
}

// Get file reader
func (fs *FileStorage) GetReader(keyID string) (io.ReadCloser, error) {
	fs.mu.RLock()
	info, exists := fs.files[keyID]
	fs.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("key file not found: %s", keyID)
	}
	
	// Update last accessed time
	fs.mu.Lock()
	info.LastAccessed = time.Now()
	fs.mu.Unlock()
	
	return os.Open(info.Path)
}

// Get storage stats
func (fs *FileStorage) GetStats() map[string]interface{} {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	
	var totalSize int64
	for _, info := range fs.files {
		totalSize += info.Size
	}
	
	return map[string]interface{}{
		"total_files": len(fs.files),
		"total_size":  totalSize,
		"base_path":   fs.basePath,
	}
}

// Start cleanup goroutine
func (fs *FileStorage) StartCleanupRoutine(interval, maxAge time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		
		for range ticker.C {
			if err := fs.CleanupOldFiles(maxAge); err != nil {
				log.Printf("Cleanup error: %v", err)
			}
		}
	}()
}