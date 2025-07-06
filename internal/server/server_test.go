package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/user/keybench/internal/benchmark"
)

func TestServerBenchmarkFlow(t *testing.T) {
	// Create test server
	srv, err := NewServer("8080")
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Test creating a benchmark
	config := benchmark.Config{
		Algorithms:   []string{"ed25519"},
		KeySizes:     []int{256},
		Iterations:   2,
		Parallel:     1,
		ShowProgress: true,
		Timeout:      30,
		Verbose:      true,
	}

	configJSON, _ := json.Marshal(config)
	req := httptest.NewRequest("POST", "/api/v1/benchmarks", bytes.NewBuffer(configJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]string
	json.NewDecoder(w.Body).Decode(&response)

	jobID, ok := response["job_id"]
	if !ok {
		t.Fatal("No job_id in response")
	}

	// Wait for benchmark to complete
	time.Sleep(500 * time.Millisecond)

	// Get benchmark results
	req = httptest.NewRequest("GET", "/api/v1/benchmarks/"+jobID, nil)
	w = httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var job BenchmarkJob
	json.NewDecoder(w.Body).Decode(&job)

	if job.Status != "completed" {
		t.Errorf("Expected status 'completed', got '%s'", job.Status)
	}

	if len(job.Results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(job.Results))
	}

	// Check that keys were generated
	req = httptest.NewRequest("GET", "/api/v1/keys", nil)
	w = httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var keys []interface{}
	json.NewDecoder(w.Body).Decode(&keys)

	if len(keys) != 2 {
		t.Errorf("Expected 2 keys, got %d", len(keys))
	}
}