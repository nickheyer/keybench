package benchmark

import (
	"testing"
	"time"

	"github.com/user/keybench/internal/storage"
)

func TestWebRunnerKeyGeneration(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		wantKeys  int
		wantError bool
	}{
		{
			name: "RSA 2048 single iteration",
			config: Config{
				Algorithms:   []string{"rsa"},
				KeySizes:     []int{2048},
				Iterations:   1,
				Parallel:     1,
				ShowProgress: false,
				Timeout:      30,
			},
			wantKeys: 1,
		},
		{
			name: "Multiple algorithms",
			config: Config{
				Algorithms:   []string{"rsa", "ecdsa", "ed25519"},
				KeySizes:     []int{2048, 256},
				Iterations:   2,
				Parallel:     1,
				ShowProgress: false,
				Timeout:      30,
			},
			wantKeys: 6, // RSA-2048(2), ECDSA-256(2), Ed25519-256(2), total 6 keys
		},
		{
			name: "Parallel generation",
			config: Config{
				Algorithms:   []string{"ed25519"},
				KeySizes:     []int{256},
				Iterations:   5,
				Parallel:     2,
				ShowProgress: false,
				Timeout:      30,
			},
			wantKeys: 10, // 5 iterations * 2 parallel = 10 keys
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyStore := storage.NewKeyStore()
			runner := NewWebRunner(tt.config, keyStore)

			results, err := runner.RunWithProgress()
			if (err != nil) != tt.wantError {
				t.Errorf("RunWithProgress() error = %v, wantError %v", err, tt.wantError)
				return
			}

			// Wait a bit for async key storage to complete
			time.Sleep(100 * time.Millisecond)

			totalKeys := len(keyStore.GetAllKeys())
			if totalKeys != tt.wantKeys {
				t.Errorf("Generated %d keys, want %d", totalKeys, tt.wantKeys)
			}

			// Verify each result has the correct number of key IDs
			for _, result := range results {
				expectedKeys := result.Iterations * result.Parallel
				if len(result.KeyIDs) != expectedKeys {
					t.Errorf("%s-%d: got %d key IDs, want %d", 
						result.Algorithm, result.KeySize, len(result.KeyIDs), expectedKeys)
				}
			}
		})
	}
}

func TestWebRunnerProgress(t *testing.T) {
	config := Config{
		Algorithms:   []string{"ed25519"},
		KeySizes:     []int{256},
		Iterations:   10,
		Parallel:     1,
		ShowProgress: false,
		Timeout:      30,
	}

	keyStore := storage.NewKeyStore()
	runner := NewWebRunner(config, keyStore)
	
	progressChan := make(chan ProgressUpdate, 100)
	runner.SetProgressChannel(progressChan)

	done := make(chan bool)
	go func() {
		runner.RunWithProgress()
		close(done)
	}()

	progressCount := 0
	timeout := time.After(5 * time.Second)

	for {
		select {
		case update, ok := <-runner.progressChan:
			if !ok {
				continue
			}
			progressCount++
			if update.Current > update.Total {
				t.Errorf("Current progress %d exceeds total %d", update.Current, update.Total)
			}
			if update.Percentage < 0 || update.Percentage > 100 {
				t.Errorf("Invalid percentage: %f", update.Percentage)
			}
		case <-done:
			if progressCount == 0 {
				t.Error("No progress updates received")
			}
			return
		case <-timeout:
			t.Fatal("Progress updates timed out")
		}
	}
}