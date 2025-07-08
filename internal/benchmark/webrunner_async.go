package benchmark

import (
	"context"
	"crypto/rsa"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/user/keybench/internal/storage"
)

// AsyncWebRunner handles asynchronous key generation with progress tracking
type AsyncWebRunner struct {
	config       Config
	keyStore     *storage.KeyStore
	fileStorage  *storage.FileStorage
	progressChan chan ProgressUpdate
	keyGenWorker *KeyGenWorker
	tempDir      string
}

// AsyncWebResult extends WebResult with async-specific fields
type AsyncWebResult struct {
	WebResult
	FilesGenerated []string `json:"files_generated,omitempty"`
}

// NewAsyncWebRunner creates a new async web runner
func NewAsyncWebRunner(config Config, keyStore *storage.KeyStore, fileStorage *storage.FileStorage, tempDir string) *AsyncWebRunner {
	if tempDir == "" {
		tempDir = "./keybench_storage/temp"
	}

	// Use workers from config if specified, otherwise default to 1
	workers := config.Workers
	if workers < 1 {
		workers = 1
	}

	return &AsyncWebRunner{
		config:       config,
		keyStore:     keyStore,
		fileStorage:  fileStorage,
		progressChan: make(chan ProgressUpdate, 1000), // Larger buffer for async
		tempDir:      tempDir,
		keyGenWorker: NewKeyGenWorkerWithCount(tempDir, workers),
	}
}

// SetProgressChannel sets the progress channel
func (w *AsyncWebRunner) SetProgressChannel(ch chan ProgressUpdate) {
	w.progressChan = ch
}

// Start initializes the async runner
func (w *AsyncWebRunner) Start() {
	w.keyGenWorker.Start()
}

// Stop gracefully shuts down the async runner
func (w *AsyncWebRunner) Stop() {
	if w.keyGenWorker != nil {
		w.keyGenWorker.Stop()
	}
}

// RunWithProgress runs benchmarks asynchronously with progress updates
func (w *AsyncWebRunner) RunWithProgress() ([]AsyncWebResult, error) {
	return w.RunWithProgressContext(context.Background())
}

// RunWithProgressContext runs benchmarks asynchronously with progress updates and context
func (w *AsyncWebRunner) RunWithProgressContext(ctx context.Context) ([]AsyncWebResult, error) {
	var results []AsyncWebResult
	processedCombos := make(map[string]bool)

	// Create a timeout context that also respects the parent context
	var timeoutCtx context.Context
	var cancel context.CancelFunc
	if w.config.Timeout > 0 {
		timeoutCtx, cancel = context.WithTimeout(ctx, time.Duration(w.config.Timeout)*time.Second)
		defer cancel()
	} else {
		timeoutCtx = context.Background()
	}

	fmt.Printf("AsyncWebRunner: Starting with config - Algorithms: %v, KeySizes: %v, Iterations: %d, Parallel: %d, Workers: %d\n",
		w.config.Algorithms, w.config.KeySizes, w.config.Iterations, w.config.Parallel, w.config.Workers)

	for _, algo := range w.config.Algorithms {
		bench, err := getAlgorithmBenchmark(algo)
		if err != nil {
			fmt.Printf("AsyncWebRunner: Error getting algorithm %s: %v\n", algo, err)
			return nil, err
		}

		for _, size := range w.config.KeySizes {
			// Allow any key size - no validation
			comboKey := fmt.Sprintf("%s-%d", algo, size)
			if processedCombos[comboKey] {
				fmt.Printf("AsyncWebRunner: Skipping duplicate combo %s\n", comboKey)
				continue
			}
			processedCombos[comboKey] = true

			fmt.Printf("AsyncWebRunner: Running async benchmark for %s-%d\n", algo, size)

			// Check for context cancellation
			select {
			case <-timeoutCtx.Done():
				return nil, ctx.Err()
			default:
			}

			result, err := w.runAsyncBenchmarkWithContext(ctx, bench, size)
			if err != nil {
				fmt.Printf("AsyncWebRunner: Error in benchmark: %v\n", err)
				return nil, err
			}

			fmt.Printf("AsyncWebRunner: Benchmark completed - Keys generated: %d, Files: %d, Errors: %d\n",
				len(result.KeyIDs), len(result.FilesGenerated), result.Errors)
			results = append(results, result)
		}
	}

	fmt.Printf("AsyncWebRunner: All benchmarks completed. Total results: %d\n", len(results))
	return results, nil
}

// runAsyncBenchmarkWithContext runs a single benchmark asynchronously with context
func (w *AsyncWebRunner) runAsyncBenchmarkWithContext(ctx context.Context, bench AlgorithmBenchmark, keySize int) (AsyncWebResult, error) {
	benchmarkID := uuid.New().String()

	result := AsyncWebResult{
		WebResult: WebResult{
			Result: Result{
				Algorithm:   bench.Name(),
				KeySize:     keySize,
				Iterations:  w.config.Iterations,
				Parallel:    w.config.Parallel,
				CompletedAt: time.Now(),
			},
			BenchmarkID: benchmarkID,
			KeyIDs:      make([]string, 0),
			Progress:    make(chan ProgressUpdate, 100),
		},
		FilesGenerated: make([]string, 0),
	}

	totalIterations := w.config.Iterations * w.config.Parallel

	// Create a timeout context that also respects the parent context
	var timeoutCtx context.Context
	var cancel context.CancelFunc
	if w.config.Timeout > 0 {
		timeoutCtx, cancel = context.WithTimeout(ctx, time.Duration(w.config.Timeout)*time.Second)
		defer cancel()
	} else {
		timeoutCtx = context.Background()
	}

	// Tracking variables
	var (
		completed      int32
		errors         int32
		filesGenerated int32
		startTime      = time.Now()
		mu             sync.Mutex
		timings        []time.Duration
	)

	// Response channel for key generation
	responseChan := make(chan KeyGenResponse, w.config.Parallel*2)

	// Progress updater goroutine
	stopProgress := make(chan struct{})
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-stopProgress:
				return
			case <-timeoutCtx.Done():
				return
			case <-ticker.C:
				current := atomic.LoadInt32(&completed)
				elapsed := time.Since(startTime).Seconds()
				rate := float64(current) / elapsed
				percentage := float64(current) / float64(totalIterations) * 100

				update := ProgressUpdate{
					Current:    int(current),
					Total:      totalIterations,
					Percentage: percentage,
					Rate:       rate,
					Algorithm:  bench.Name(),
					KeySize:    keySize,
				}

				// Send to both channels
				select {
				case result.Progress <- update:
				default:
				}

				if w.progressChan != nil {
					select {
					case w.progressChan <- update:
					default:
					}
				}

				if int(current) >= totalIterations {
					return
				}
			}
		}
	}()

	// Submit all key generation requests
	fmt.Printf("AsyncWebRunner: Submitting %d key generation requests\n", totalIterations)
	for i := 0; i < totalIterations; i++ {
		keyID := uuid.New().String()

		// Determine if we should use file storage
		useFile := w.shouldUseFileStorage(bench.Name(), keySize)

		req := KeyGenRequest{
			ID:       keyID,
			Type:     bench.Name(),
			Size:     keySize,
			UseFile:  useFile,
			Response: responseChan,
			Context:  timeoutCtx,
		}

		err := w.keyGenWorker.Submit(req)
		if err != nil {
			atomic.AddInt32(&errors, 1)
			atomic.AddInt32(&completed, 1)
			fmt.Printf("AsyncWebRunner: Failed to submit request: %v\n", err)
		}
	}

	// Collect responses
	go func() {
		for i := 0; i < totalIterations; i++ {
			select {
			case <-timeoutCtx.Done():
				// Mark remaining as completed with errors
				remaining := totalIterations - int(atomic.LoadInt32(&completed))
				atomic.AddInt32(&completed, int32(remaining))
				atomic.AddInt32(&errors, int32(remaining))
				return

			case resp := <-responseChan:
				if resp.Error != nil {
					atomic.AddInt32(&errors, 1)
					fmt.Printf("AsyncWebRunner: Key generation error: %v\n", resp.Error)
				} else {
					mu.Lock()
					timings = append(timings, resp.Duration)

					// Store key reference
					storedKey := &storage.StoredKey{
						ID:          resp.ID,
						Type:        bench.Name(),
						Size:        keySize,
						BenchmarkID: benchmarkID,
						CreatedAt:   time.Now(),
						FileStored:  resp.FilePath != "",
					}

					if resp.FilePath != "" {
						storedKey.PrivateKey = "[Stored in file]"
						storedKey.PublicKey = "[Stored in file]"
						storedKey.FilePath = resp.FilePath
						result.FilesGenerated = append(result.FilesGenerated, resp.FilePath)
						atomic.AddInt32(&filesGenerated, 1)
					} else if resp.Key != nil {
						// Store in-memory key
						switch bench.Name() {
						case "RSA", "RSA-Parallel":
							if rsaKey, ok := resp.Key.(*rsa.PrivateKey); ok {
								stored, err := w.keyStore.StoreRSAKey(rsaKey, keySize, benchmarkID)
								if err == nil && stored != nil {
									storedKey = stored
								}
							}
							// Add other key types as needed
						}
					}

					result.KeyIDs = append(result.KeyIDs, storedKey.ID)
					if storedKey.FileStored {
						w.keyStore.StoreKeyReference(storedKey)
					}

					mu.Unlock()
				}

				atomic.AddInt32(&completed, 1)
			}
		}
	}()

	// Wait for all responses to be collected
	for int(atomic.LoadInt32(&completed)) < totalIterations {
		time.Sleep(10 * time.Millisecond)
	}

	// Stop progress updates
	close(stopProgress)
	// Give the progress goroutine time to exit cleanly
	time.Sleep(50 * time.Millisecond)

	// Now safe to close the progress channel
	close(result.Progress)

	// Calculate final statistics
	result.TotalTime = time.Since(startTime)
	result.Errors = int(atomic.LoadInt32(&errors))

	mu.Lock()
	if len(timings) > 0 {
		result.AverageTime = calculateAverage(timings)
		result.MinTime = calculateMin(timings)
		result.MaxTime = calculateMax(timings)
		result.StdDev = calculateStdDev(timings, result.AverageTime)
		result.KeysPerSecond = float64(len(timings)) / result.TotalTime.Seconds()
	}
	mu.Unlock()

	// Get worker stats
	activeReqs, totalGen, bytesGen := w.keyGenWorker.GetStats()
	fmt.Printf("AsyncWebRunner: Worker stats - Active: %d, Total generated: %d, Bytes: %d\n",
		activeReqs, totalGen, bytesGen)

	return result, nil
}

// shouldUseFileStorage determines if file storage should be used
func (w *AsyncWebRunner) shouldUseFileStorage(algorithm string, keySize int) bool {
	// Always use file storage if configured
	if w.config.FileStorage {
		return true
	}

	// Auto-enable for large keys
	switch algorithm {
	case "RSA":
		return keySize >= 8192
	case "ECDSA":
		return keySize >= 521 // P-521 curve
	case "Ed25519":
		return false // Ed25519 has fixed size
	default:
		return false
	}
}
