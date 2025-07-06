package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
	router   *mux.Router
	keyStore *storage.KeyStore
	jobStore *JobStore
	sysInfo  *sysinfo.SystemInfo
	upgrader websocket.Upgrader
	port     string
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
	CompletedAt *time.Time                    `json:"completed_at,omitempty"`
	Results     []benchmark.WebResult         `json:"results,omitempty"`
	Error       string                        `json:"error,omitempty"`
	Progress    chan benchmark.ProgressUpdate `json:"-"`
}

func NewServer(port string) (*Server, error) {
	sysInfo, err := sysinfo.Collect()
	if err != nil {
		return nil, fmt.Errorf("failed to collect system info: %w", err)
	}

	s := &Server{
		router:   mux.NewRouter(),
		keyStore: storage.NewKeyStore(),
		jobStore: &JobStore{
			jobs: make(map[string]*BenchmarkJob),
		},
		sysInfo: sysInfo,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		port: port,
	}

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
	api.HandleFunc("/keys", s.handleListKeys).Methods("GET")
	api.HandleFunc("/keys/{id}", s.handleGetKey).Methods("GET")
	api.HandleFunc("/keys/{id}/download", s.handleDownloadKey).Methods("GET")
	api.HandleFunc("/keys/{id}", s.handleDeleteKey).Methods("DELETE")

	// Static files
	s.router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("./web/static"))))
	s.router.HandleFunc("/", s.handleIndex)
}

func (s *Server) Start() error {
	log.Printf("KeyBench Web Server starting on http://localhost:%s", s.port)
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

	// Create job
	job := &BenchmarkJob{
		ID:        uuid.New().String(),
		Config:    config,
		Status:    "running",
		StartedAt: time.Now(),
		Progress:  make(chan benchmark.ProgressUpdate, 100),
	}

	s.jobStore.mu.Lock()
	s.jobStore.jobs[job.ID] = job
	s.jobStore.mu.Unlock()

	// Run benchmark in background
	go s.runBenchmark(job)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"job_id": job.ID,
		"status": "started",
	})
}

func (s *Server) runBenchmark(job *BenchmarkJob) {
	if job.Progress != nil {
		defer close(job.Progress)
	}
	log.Printf("Starting benchmark job %s with config: %+v", job.ID, job.Config)
	
	runner := benchmark.NewWebRunner(job.Config, s.keyStore)
	if job.Progress != nil {
		runner.SetProgressChannel(job.Progress)
	}
	results, err := runner.RunWithProgress()

	completedAt := time.Now()
	s.jobStore.mu.Lock()
	job.CompletedAt = &completedAt
	if err != nil {
		log.Printf("Benchmark job %s failed: %v", job.ID, err)
		job.Status = "failed"
		job.Error = err.Error()
	} else {
		log.Printf("Benchmark job %s completed with %d results", job.ID, len(results))
		for i, r := range results {
			log.Printf("  Result %d: %s-%d, Keys: %d, Errors: %d", i, r.Algorithm, r.KeySize, len(r.KeyIDs), r.Errors)
		}
		job.Status = "completed"
		job.Results = results
	}
	s.jobStore.mu.Unlock()
}

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

	var content string
	var filename string

	if keyType == "public" {
		content = key.PublicKey
		filename = fmt.Sprintf("%s_%s_%d_public.pem", key.Type, key.ID[:8], key.Size)
	} else {
		content = key.PrivateKey
		filename = fmt.Sprintf("%s_%s_%d_private.pem", key.Type, key.ID[:8], key.Size)
	}

	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.Write([]byte(content))
}

func (s *Server) handleDeleteKey(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	s.keyStore.DeleteKey(id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./templates/index.html")
}
