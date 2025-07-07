package server

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/user/keybench/internal/benchmark"
	"github.com/user/keybench/internal/storage"
)

type WorkerPool struct {
	workers      int
	jobQueue     chan *BenchmarkJob
	wg           sync.WaitGroup
	ctx          context.Context
	cancel       context.CancelFunc
	jobStore     *JobStore
	keyStore     *storage.KeyStore
	fileStorage  *storage.FileStorage
	asyncRunner  *benchmark.AsyncWebRunner
	activeJobs   map[string]context.CancelFunc
	mu           sync.Mutex
}

func NewWorkerPool(numWorkers int, jobStore *JobStore, keyStore *storage.KeyStore, fileStorage *storage.FileStorage) *WorkerPool {
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	// Create a default config for the async runner
	config := benchmark.Config{
		Parallel: runtime.NumCPU() * 2, // Use 2x CPU cores
	}
	
	asyncRunner := benchmark.NewAsyncWebRunner(config, keyStore, fileStorage, "./keybench_storage/temp")
	
	return &WorkerPool{
		workers:     numWorkers,
		jobQueue:    make(chan *BenchmarkJob, numWorkers*2),
		ctx:         ctx,
		cancel:      cancel,
		jobStore:    jobStore,
		keyStore:    keyStore,
		fileStorage: fileStorage,
		asyncRunner: asyncRunner,
		activeJobs:  make(map[string]context.CancelFunc),
	}
}

func (wp *WorkerPool) Start() {
	log.Printf("Starting worker pool with %d workers", wp.workers)
	
	// Start the async runner
	wp.asyncRunner.Start()
	
	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}
}

func (wp *WorkerPool) Stop() {
	log.Println("Stopping worker pool...")
	wp.cancel()
	close(wp.jobQueue)
	wp.wg.Wait()
	
	// Stop the async runner
	wp.asyncRunner.Stop()
	
	log.Println("Worker pool stopped")
}

func (wp *WorkerPool) Submit(job *BenchmarkJob) error {
	select {
	case wp.jobQueue <- job:
		return nil
	case <-wp.ctx.Done():
		return fmt.Errorf("worker pool is shutting down")
	default:
		return fmt.Errorf("job queue is full")
	}
}

func (wp *WorkerPool) worker(id int) {
	defer wp.wg.Done()
	log.Printf("Worker %d started", id)
	
	for {
		select {
		case job, ok := <-wp.jobQueue:
			if !ok {
				log.Printf("Worker %d stopping", id)
				return
			}
			
			log.Printf("Worker %d processing job %s", id, job.ID)
			wp.processJob(job)
			
		case <-wp.ctx.Done():
			log.Printf("Worker %d stopping due to context cancellation", id)
			return
		}
	}
}

func (wp *WorkerPool) TerminateJob(jobID string) {
	wp.mu.Lock()
	if cancel, exists := wp.activeJobs[jobID]; exists {
		cancel()
		delete(wp.activeJobs, jobID)
	}
	wp.mu.Unlock()
}

func (wp *WorkerPool) processJob(job *BenchmarkJob) {
	// Create a cancellable context for this job
	jobCtx, jobCancel := context.WithCancel(wp.ctx)
	
	// Register the job
	wp.mu.Lock()
	wp.activeJobs[job.ID] = jobCancel
	wp.mu.Unlock()
	
	defer func() {
		// Cleanup when job completes
		wp.mu.Lock()
		delete(wp.activeJobs, job.ID)
		wp.mu.Unlock()
		jobCancel()
	}()
	// Update job status
	wp.jobStore.UpdateStatus(job.ID, "running")
	
	// Send initial progress
	if job.Progress != nil {
		select {
		case job.Progress <- benchmark.ProgressUpdate{
			Current:    0,
			Total:      1,
			Percentage: 0,
			Rate:       0,
		}:
		default:
		}
	}
	
	// Create a new async runner with the job's config
	runner := benchmark.NewAsyncWebRunner(job.Config, wp.keyStore, wp.fileStorage, "./keybench_storage/temp")
	runner.Start()
	defer runner.Stop()
	
	if job.Progress != nil {
		runner.SetProgressChannel(job.Progress)
	}
	
	// Check for cancellation before starting
	select {
	case <-jobCtx.Done():
		wp.jobStore.CompleteJob(job.ID, nil, fmt.Errorf("job terminated by user"))
		return
	default:
	}
	
	// Run the benchmark asynchronously with context
	results, err := runner.RunWithProgressContext(jobCtx)
	
	// Convert AsyncWebResult to WebResult
	var webResults []benchmark.WebResult
	for _, r := range results {
		webResults = append(webResults, r.WebResult)
	}
	
	// Update job with results
	if err != nil {
		wp.jobStore.CompleteJob(job.ID, nil, err)
		log.Printf("Job %s failed: %v", job.ID, err)
	} else {
		wp.jobStore.CompleteJob(job.ID, webResults, nil)
		log.Printf("Job %s completed successfully with %d results", job.ID, len(webResults))
	}
	
	// Close progress channel
	if job.Progress != nil {
		close(job.Progress)
	}
}

// Helper methods for JobStore
func (js *JobStore) UpdateStatus(jobID, status string) {
	js.mu.Lock()
	defer js.mu.Unlock()
	
	if job, exists := js.jobs[jobID]; exists {
		job.Status = status
		job.UpdatedAt = time.Now()
	}
}

func (js *JobStore) CompleteJob(jobID string, results []benchmark.WebResult, err error) {
	js.mu.Lock()
	defer js.mu.Unlock()
	
	if job, exists := js.jobs[jobID]; exists {
		completedAt := time.Now()
		job.CompletedAt = &completedAt
		job.UpdatedAt = completedAt
		
		if err != nil {
			job.Status = "failed"
			job.Error = err.Error()
		} else {
			job.Status = "completed"
			job.Results = results
		}
	}
}