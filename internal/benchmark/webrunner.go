package benchmark

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/user/keybench/internal/storage"
)

type WebRunner struct {
	config        Config
	keyStore      *storage.KeyStore
	progressChan  chan ProgressUpdate
}

type WebResult struct {
	Result
	BenchmarkID string              `json:"benchmark_id"`
	KeyIDs      []string            `json:"key_ids"`
	Progress    chan ProgressUpdate `json:"-"`
}

type ProgressUpdate struct {
	Current    int     `json:"current"`
	Total      int     `json:"total"`
	Percentage float64 `json:"percentage"`
	Rate       float64 `json:"rate"`
	Algorithm  string  `json:"algorithm"`
	KeySize    int     `json:"key_size"`
}

func NewWebRunner(config Config, keyStore *storage.KeyStore) *WebRunner {
	return &WebRunner{
		config:       config,
		keyStore:     keyStore,
		progressChan: make(chan ProgressUpdate, 100),
	}
}

func (w *WebRunner) SetProgressChannel(ch chan ProgressUpdate) {
	w.progressChan = ch
}

func (w *WebRunner) RunWithProgress() ([]WebResult, error) {
	var results []WebResult
	processedCombos := make(map[string]bool)

	fmt.Printf("WebRunner: Starting with config - Algorithms: %v, KeySizes: %v, Iterations: %d, Parallel: %d\n", 
		w.config.Algorithms, w.config.KeySizes, w.config.Iterations, w.config.Parallel)

	for _, algo := range w.config.Algorithms {
		bench, err := getAlgorithmBenchmark(algo)
		if err != nil {
			fmt.Printf("WebRunner: Error getting algorithm %s: %v\n", algo, err)
			return nil, err
		}

		for _, size := range w.config.KeySizes {
			if !isValidKeySize(bench, size) {
				fmt.Printf("WebRunner: Skipping invalid key size %d for %s\n", size, algo)
				continue
			}

			// Skip duplicate algorithm/size combinations
			comboKey := fmt.Sprintf("%s-%d", algo, size)
			if processedCombos[comboKey] {
				fmt.Printf("WebRunner: Skipping duplicate combo %s\n", comboKey)
				continue
			}
			processedCombos[comboKey] = true

			fmt.Printf("WebRunner: Running benchmark for %s-%d\n", algo, size)
			result, err := w.runSingleBenchmarkWithKeys(bench, size)
			if err != nil {
				fmt.Printf("WebRunner: Error in benchmark: %v\n", err)
				return nil, err
			}

			fmt.Printf("WebRunner: Benchmark completed - Keys generated: %d, Errors: %d\n", 
				len(result.KeyIDs), result.Errors)
			results = append(results, result)
		}
	}

	fmt.Printf("WebRunner: All benchmarks completed. Total results: %d\n", len(results))
	return results, nil
}

func (w *WebRunner) runSingleBenchmarkWithKeys(bench AlgorithmBenchmark, keySize int) (WebResult, error) {
	benchmarkID := uuid.New().String()

	result := WebResult{
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
	}

	totalIterations := w.config.Iterations * w.config.Parallel

	// Run benchmark
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(w.config.Timeout)*time.Second)
	defer cancel()

	var timings []time.Duration
	var errors int
	var mu sync.Mutex
	var keyIDsMu sync.Mutex
	var keyStoreWg sync.WaitGroup

	startTime := time.Now()
	completed := 0

	var wg sync.WaitGroup
	for i := 0; i < w.config.Parallel; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < w.config.Iterations; j++ {
				select {
				case <-ctx.Done():
					return
				default:
					iterStart := time.Now()
					keyData, err := bench.GenerateKey(keySize)
					elapsed := time.Since(iterStart)

					mu.Lock()
					if err != nil {
						errors++
					} else {
						timings = append(timings, elapsed)

						// Store the key
						keyStoreWg.Add(1)
						go func(key any) {
							defer keyStoreWg.Done()
							storedKey, storeErr := w.storeKey(bench.Name(), keySize, key, benchmarkID)
							if storeErr != nil {
								fmt.Printf("WebRunner: Failed to store key: %v\n", storeErr)
							} else if storedKey != nil {
								keyIDsMu.Lock()
								result.KeyIDs = append(result.KeyIDs, storedKey.ID)
								keyIDsMu.Unlock()
							}
						}(keyData)
					}
					completed++
					progress := float64(completed) / float64(totalIterations) * 100
					rate := float64(completed) / time.Since(startTime).Seconds()

					// Send progress update to both channels
					update := ProgressUpdate{
						Current:    completed,
						Total:      totalIterations,
						Percentage: progress,
						Rate:       rate,
						Algorithm:  bench.Name(),
						KeySize:    keySize,
					}
					
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

					mu.Unlock()
				}
			}
		}(i)
	}

	wg.Wait()
	
	// Wait for all keys to be stored
	keyStoreWg.Wait()
	
	close(result.Progress)

	result.TotalTime = time.Since(startTime)
	result.Errors = errors

	// Calculate statistics
	if len(timings) > 0 {
		result.AverageTime = calculateAverage(timings)
		result.MinTime = calculateMin(timings)
		result.MaxTime = calculateMax(timings)
		result.StdDev = calculateStdDev(timings, result.AverageTime)
		result.KeysPerSecond = float64(len(timings)) / result.TotalTime.Seconds()
	}

	return result, nil
}

func (w *WebRunner) storeKey(algorithm string, keySize int, keyData any, benchmarkID string) (*storage.StoredKey, error) {
	switch algorithm {
	case "RSA":
		if key, ok := keyData.(*rsa.PrivateKey); ok {
			return w.keyStore.StoreRSAKey(key, keySize, benchmarkID)
		}
		return nil, fmt.Errorf("invalid RSA key data")
	case "ECDSA":
		if key, ok := keyData.(*ecdsa.PrivateKey); ok {
			return w.keyStore.StoreECDSAKey(key, keySize, benchmarkID)
		}
		return nil, fmt.Errorf("invalid ECDSA key data")
	case "Ed25519":
		if keys, ok := keyData.([]any); ok && len(keys) == 2 {
			if pub, pubOk := keys[0].(ed25519.PublicKey); pubOk {
				if priv, privOk := keys[1].(ed25519.PrivateKey); privOk {
					return w.keyStore.StoreEd25519Key(pub, priv, benchmarkID)
				}
			}
		}
		return nil, fmt.Errorf("invalid Ed25519 key data")
	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", algorithm)
	}
}
