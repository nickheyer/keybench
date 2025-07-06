package benchmark

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/schollz/progressbar/v3"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

type Runner struct {
	config Config
}

func NewRunner(config Config) *Runner {
	return &Runner{config: config}
}

func (r *Runner) Run() ([]Result, error) {
	var results []Result

	for _, algo := range r.config.Algorithms {
		bench, err := getAlgorithmBenchmark(algo)
		if err != nil {
			return nil, err
		}

		for _, size := range r.config.KeySizes {
			if !isValidKeySize(bench, size) {
				if r.config.Verbose {
					fmt.Printf("Skipping invalid key size %d for %s\n", size, algo)
				}
				continue
			}

			result, err := r.runSingleBenchmark(bench, size)
			if err != nil {
				return nil, err
			}

			results = append(results, result)
		}
	}

	return results, nil
}

func (r *Runner) runSingleBenchmark(bench AlgorithmBenchmark, keySize int) (Result, error) {
	result := Result{
		Algorithm:   bench.Name(),
		KeySize:     keySize,
		Iterations:  r.config.Iterations,
		Parallel:    r.config.Parallel,
		CompletedAt: time.Now(),
	}

	totalIterations := r.config.Iterations * r.config.Parallel
	var progress *progressbar.ProgressBar

	if r.config.ShowProgress {
		progress = progressbar.NewOptions(totalIterations,
			progressbar.OptionSetDescription(fmt.Sprintf("[%s-%d]", bench.Name(), keySize)),
			progressbar.OptionSetWidth(40),
			progressbar.OptionShowCount(),
			progressbar.OptionShowIts(),
			progressbar.OptionOnCompletion(func() {
				fmt.Println()
			}),
		)
	}

	// Collect initial CPU and memory stats
	initialCPU, _ := cpu.Percent(100*time.Millisecond, false)
	initialMem, _ := mem.VirtualMemory()

	// Run benchmark
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(r.config.Timeout)*time.Second)
	defer cancel()

	var timings []time.Duration
	var errors int
	var mu sync.Mutex

	startTime := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < r.config.Parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < r.config.Iterations; j++ {
				select {
				case <-ctx.Done():
					return
				default:
					iterStart := time.Now()
					_, err := bench.GenerateKey(keySize)
					elapsed := time.Since(iterStart)

					mu.Lock()
					if err != nil {
						errors++
					} else {
						timings = append(timings, elapsed)
					}
					mu.Unlock()

					if progress != nil {
						progress.Add(1)
					}
				}
			}
		}()
	}

	wg.Wait()

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

	// Collect final CPU and memory stats
	finalCPU, _ := cpu.Percent(100*time.Millisecond, false)
	finalMem, _ := mem.VirtualMemory()

	if len(initialCPU) > 0 && len(finalCPU) > 0 {
		result.CPUUsage = finalCPU[0] - initialCPU[0]
	}

	if finalMem.Used > initialMem.Used {
		result.MemoryUsed = finalMem.Used - initialMem.Used
	} else {
		result.MemoryUsed = 0
	}

	// Force garbage collection to get more accurate memory readings
	runtime.GC()

	return result, nil
}

func getAlgorithmBenchmark(name string) (AlgorithmBenchmark, error) {
	switch name {
	case "rsa":
		return &RSABenchmark{}, nil
	case "ecdsa":
		return &ECDSABenchmark{}, nil
	case "ed25519":
		return &Ed25519Benchmark{}, nil
	default:
		return nil, fmt.Errorf("unknown algorithm: %s", name)
	}
}

func isValidKeySize(bench AlgorithmBenchmark, size int) bool {
	for _, validSize := range bench.SupportedKeySizes() {
		if size == validSize {
			return true
		}
	}
	return false
}

func calculateAverage(timings []time.Duration) time.Duration {
	if len(timings) == 0 {
		return 0
	}

	var sum time.Duration
	for _, t := range timings {
		sum += t
	}
	return sum / time.Duration(len(timings))
}

func calculateMin(timings []time.Duration) time.Duration {
	if len(timings) == 0 {
		return 0
	}

	min := timings[0]
	for _, t := range timings[1:] {
		if t < min {
			min = t
		}
	}
	return min
}

func calculateMax(timings []time.Duration) time.Duration {
	if len(timings) == 0 {
		return 0
	}

	max := timings[0]
	for _, t := range timings[1:] {
		if t > max {
			max = t
		}
	}
	return max
}

func calculateStdDev(timings []time.Duration, avg time.Duration) time.Duration {
	if len(timings) <= 1 {
		return 0
	}

	var sum float64
	avgFloat := float64(avg)

	for _, t := range timings {
		diff := float64(t) - avgFloat
		sum += diff * diff
	}

	variance := sum / float64(len(timings)-1)
	stdDev := math.Sqrt(variance)

	return time.Duration(stdDev)
}
