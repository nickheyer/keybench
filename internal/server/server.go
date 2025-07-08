package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/user/keybench/internal/benchmark"
	"github.com/user/keybench/internal/storage"
	"github.com/user/keybench/pkg/sysinfo"
)

type Server struct {
	router      *mux.Router
	keyStore    *storage.KeyStore
	fileStorage *storage.FileStorage
	jobStore    *JobStore
	workerPool  *WorkerPool
	sysInfo     *sysinfo.SystemInfo
	upgrader    websocket.Upgrader
	port        string
}

type JobStore struct {
	mu   sync.RWMutex
	jobs map[string]*BenchmarkJob
}

type BenchmarkJob struct {
	ID          string                        `json:"id"`
	Config      benchmark.Config              `json:"config"`
	Status      string                        `json:"status"`
	StartedAt   time.Time                     `json:"started_at"`
	UpdatedAt   time.Time                     `json:"updated_at"`
	CompletedAt *time.Time                    `json:"completed_at,omitempty"`
	Results     []benchmark.WebResult         `json:"results,omitempty"`
	Error       string                        `json:"error,omitempty"`
	Progress    chan benchmark.ProgressUpdate `json:"-"`
}

func NewServer(port string) (*Server, error) {
	return NewServerWithWorkers(port, 1) // Default to 1 worker
}

func NewServerWithWorkers(port string, workers int) (*Server, error) {
	// Validate workers count
	if workers < 1 {
		workers = 1
	}

	sysInfo, err := sysinfo.Collect()
	if err != nil {
		return nil, fmt.Errorf("failed to collect system info: %w", err)
	}

	// Create file storage
	fileStorage, err := storage.NewFileStorage("./keybench_storage")
	if err != nil {
		return nil, fmt.Errorf("failed to create file storage: %w", err)
	}

	keyStore := storage.NewKeyStore()
	jobStore := &JobStore{
		jobs: make(map[string]*BenchmarkJob),
	}

	s := &Server{
		router:      mux.NewRouter(),
		keyStore:    keyStore,
		fileStorage: fileStorage,
		jobStore:    jobStore,
		sysInfo:     sysInfo,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		port: port,
	}

	// Create worker pool with specified number of workers
	s.workerPool = NewWorkerPool(workers, jobStore, keyStore, fileStorage)

	// Start file cleanup routine (cleanup files older than 24 hours)
	fileStorage.StartCleanupRoutine(1*time.Hour, 24*time.Hour)

	s.setupRoutes()
	return s, nil
}

func (s *Server) setupRoutes() {
	// API routes
	api := s.router.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/system-info", s.handleSystemInfo).Methods("GET")
	api.HandleFunc("/benchmarks", s.handleCreateBenchmark).Methods("POST")
	api.HandleFunc("/benchmarks", s.handleListBenchmarks).Methods("GET")
	api.HandleFunc("/benchmarks/{id}", s.handleGetBenchmark).Methods("GET")
	api.HandleFunc("/benchmarks/{id}/progress", s.handleBenchmarkProgress).Methods("GET")
	api.HandleFunc("/benchmarks/{id}/terminate", s.handleTerminateBenchmark).Methods("POST")
	api.HandleFunc("/keys", s.handleListKeys).Methods("GET")
	api.HandleFunc("/keys/{id}", s.handleGetKey).Methods("GET")
	api.HandleFunc("/keys/{id}/download", s.handleDownloadKey).Methods("GET")
	api.HandleFunc("/keys/{id}", s.handleDeleteKey).Methods("DELETE")
	api.HandleFunc("/keys/cleanup-all", s.handleCleanupAllKeys).Methods("POST")
	api.HandleFunc("/cleanup-status", s.handleCleanupStatus).Methods("GET")

	// Static files
	s.router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("./web/static"))))
	s.router.HandleFunc("/", s.handleIndex)
}

func (s *Server) Start() error {
	// Start worker pool
	s.workerPool.Start()

	log.Printf("KeyBench Web Server starting on http://localhost:%s", s.port)
	log.Printf("Worker pool started with %d workers", runtime.NumCPU())

	return http.ListenAndServe(":"+s.port, s.router)
}

func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.sysInfo)
}

func (s *Server) handleCreateBenchmark(w http.ResponseWriter, r *http.Request) {
	var config benchmark.Config
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Set defaults for optimal performance
	if config.Parallel == 0 {
		config.Parallel = runtime.NumCPU()
	}

	// Log the configuration for debugging
	log.Printf("Creating benchmark job with config - Parallel: %d, Workers: %d, Iterations: %d", 
		config.Parallel, config.Workers, config.Iterations)

	// Create job
	job := &BenchmarkJob{
		ID:        uuid.New().String(),
		Config:    config,
		Status:    "queued",
		StartedAt: time.Now(),
		UpdatedAt: time.Now(),
		Progress:  make(chan benchmark.ProgressUpdate, 100),
	}

	s.jobStore.mu.Lock()
	s.jobStore.jobs[job.ID] = job
	s.jobStore.mu.Unlock()

	// Submit to worker pool
	if err := s.workerPool.Submit(job); err != nil {
		http.Error(w, "Server is busy, please try again later", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"job_id": job.ID,
		"status": "started",
	})
}

// runBenchmark method removed - now handled by WorkerPool

func (s *Server) handleListBenchmarks(w http.ResponseWriter, r *http.Request) {
	s.jobStore.mu.RLock()
	jobs := make([]*BenchmarkJob, 0, len(s.jobStore.jobs))
	for _, job := range s.jobStore.jobs {
		jobs = append(jobs, job)
	}
	s.jobStore.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobs)
}

func (s *Server) handleGetBenchmark(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	s.jobStore.mu.RLock()
	job, exists := s.jobStore.jobs[id]
	s.jobStore.mu.RUnlock()

	if !exists {
		http.Error(w, "Benchmark not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

func (s *Server) handleTerminateBenchmark(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	s.jobStore.mu.Lock()
	job, exists := s.jobStore.jobs[id]
	if !exists {
		s.jobStore.mu.Unlock()
		http.Error(w, "Benchmark not found", http.StatusNotFound)
		return
	}

	// Update status to terminated
	job.Status = "terminated"
	job.UpdatedAt = time.Now()
	s.jobStore.mu.Unlock()

	// Terminate the job in the worker pool
	s.workerPool.TerminateJob(id)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "terminated",
		"message": "Benchmark termination initiated",
	})
}

func (s *Server) handleBenchmarkProgress(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// Get the job
	s.jobStore.mu.RLock()
	job, exists := s.jobStore.jobs[id]
	s.jobStore.mu.RUnlock()

	if !exists {
		return
	}

	// Send progress updates
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case update, ok := <-job.Progress:
			if ok {
				// Send actual progress update
				conn.WriteJSON(map[string]any{
					"status":     "running",
					"completed":  false,
					"current":    update.Current,
					"total":      update.Total,
					"percentage": update.Percentage,
					"rate":       update.Rate,
					"algorithm":  update.Algorithm,
					"keySize":    update.KeySize,
				})
			}
		case <-ticker.C:
			s.jobStore.mu.RLock()
			currentJob, exists := s.jobStore.jobs[id]
			s.jobStore.mu.RUnlock()

			if !exists {
				return
			}

			if currentJob.Status != "running" {
				// Send final status
				conn.WriteJSON(map[string]any{
					"status":    currentJob.Status,
					"completed": true,
				})
				return
			}

		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) handleListKeys(w http.ResponseWriter, r *http.Request) {
	benchmarkID := r.URL.Query().Get("benchmark_id")

	var keys []*storage.StoredKey
	if benchmarkID != "" {
		keys = s.keyStore.GetKeysByBenchmark(benchmarkID)
	} else {
		keys = s.keyStore.GetAllKeys()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(keys)
}

func (s *Server) handleGetKey(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	key, exists := s.keyStore.GetKey(id)
	if !exists {
		http.Error(w, "Key not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(key)
}

func (s *Server) handleDownloadKey(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	keyType := r.URL.Query().Get("type") // "private" or "public"

	key, exists := s.keyStore.GetKey(id)
	if !exists {
		http.Error(w, "Key not found", http.StatusNotFound)
		return
	}

	var content []byte
	var filename string
	var err error

	// Check if key is stored in file
	if key.FileStored && key.FilePath != "" {
		// Read from file
		content, err = os.ReadFile(key.FilePath)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to read key file: %v", err), http.StatusInternalServerError)
			return
		}

		// For file-stored keys, we have both private and public in the same file
		// If public key is requested, we need to extract it
		if keyType == "public" {
			// Extract public key from the file content
			publicContent := extractPublicKeyFromPEM(content)
			if publicContent != nil {
				content = publicContent
			}
		}

		if keyType == "public" {
			filename = fmt.Sprintf("%s_%s_%d_public.pem", key.Type, key.ID[:8], key.Size)
		} else {
			filename = fmt.Sprintf("%s_%s_%d_private.pem", key.Type, key.ID[:8], key.Size)
		}
	} else {
		// Use in-memory content
		if keyType == "public" {
			content = []byte(key.PublicKey)
			filename = fmt.Sprintf("%s_%s_%d_public.pem", key.Type, key.ID[:8], key.Size)
		} else {
			content = []byte(key.PrivateKey)
			filename = fmt.Sprintf("%s_%s_%d_private.pem", key.Type, key.ID[:8], key.Size)
		}
	}

	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.Write(content)
}

// extractPublicKeyFromPEM extracts the public key portion from a PEM file containing both private and public keys
func extractPublicKeyFromPEM(pemData []byte) []byte {
	// Look for PUBLIC KEY block
	startMarker := []byte("-----BEGIN PUBLIC KEY-----")
	endMarker := []byte("-----END PUBLIC KEY-----")

	startIdx := bytes.Index(pemData, startMarker)
	if startIdx == -1 {
		// Try RSA PUBLIC KEY format
		startMarker = []byte("-----BEGIN RSA PUBLIC KEY-----")
		endMarker = []byte("-----END RSA PUBLIC KEY-----")
		startIdx = bytes.Index(pemData, startMarker)
		if startIdx == -1 {
			return nil
		}
	}

	endIdx := bytes.Index(pemData[startIdx:], endMarker)
	if endIdx == -1 {
		return nil
	}

	// Include the end marker
	endIdx += startIdx + len(endMarker)

	return pemData[startIdx:endIdx]
}

func (s *Server) handleDeleteKey(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// Get the key first to check if it's file-stored
	key, exists := s.keyStore.GetKey(id)
	if exists && key.FileStored && key.FilePath != "" {
		// Delete the actual file
		err := os.Remove(key.FilePath)
		if err != nil {
			// Log the error but don't fail the request
			log.Printf("Failed to delete key file %s: %v", key.FilePath, err)
		}
	}

	// Delete from key store
	s.keyStore.DeleteKey(id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCleanupAllKeys(w http.ResponseWriter, r *http.Request) {
	filesDeleted := 0
	errors := 0

	// Clean up files in both temp and keys directories
	directories := []string{
		"./keybench_storage/temp",
		"./keybench_storage/keys",
	}

	for _, dir := range directories {
		entries, err := os.ReadDir(dir)
		if err != nil {
			log.Printf("Failed to read directory %s: %v", dir, err)
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				filePath := filepath.Join(dir, entry.Name())
				err := os.Remove(filePath)
				if err != nil {
					log.Printf("Failed to delete file %s: %v", filePath, err)
					errors++
				} else {
					filesDeleted++
				}
			}
		}
	}

	// Also clear all keys from the key store
	keys := s.keyStore.GetAllKeys()
	for _, key := range keys {
		s.keyStore.DeleteKey(key.ID)
	}

	response := map[string]interface{}{
		"files_deleted": filesDeleted,
		"keys_cleared":  len(keys),
		"errors":        errors,
		"message":       fmt.Sprintf("Cleaned up %d files and %d keys", filesDeleted, len(keys)),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleCleanupStatus(w http.ResponseWriter, r *http.Request) {
	// Get cleanup status from file storage
	activeFiles := 0
	pendingCleanup := 0

	if s.fileStorage != nil {
		// This would need to be implemented in FileStorage
		// For now, return a simple status
		activeFiles = s.keyStore.CountFileStoredKeys()
	}

	status := map[string]interface{}{
		"active_files":     activeFiles,
		"pending_cleanup":  pendingCleanup,
		"cleanup_interval": "30 minutes",
		"enabled":          true,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/test" {
		http.ServeFile(w, r, "./test_websocket.html")
		return
	}
	http.ServeFile(w, r, "./templates/index.html")
}
