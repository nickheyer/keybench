package benchmark

import (
	"testing"
	"time"
)

func TestCalculateStatistics(t *testing.T) {
	timings := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		150 * time.Millisecond,
		180 * time.Millisecond,
		170 * time.Millisecond,
	}
	
	avg := calculateAverage(timings)
	expectedAvg := 160 * time.Millisecond
	if avg != expectedAvg {
		t.Errorf("Expected average %v, got %v", expectedAvg, avg)
	}
	
	min := calculateMin(timings)
	if min != 100*time.Millisecond {
		t.Errorf("Expected min %v, got %v", 100*time.Millisecond, min)
	}
	
	max := calculateMax(timings)
	if max != 200*time.Millisecond {
		t.Errorf("Expected max %v, got %v", 200*time.Millisecond, max)
	}
	
	stdDev := calculateStdDev(timings, avg)
	// Allow for some floating point error
	if stdDev < 35*time.Millisecond || stdDev > 40*time.Millisecond {
		t.Errorf("Expected stdDev around 37ms, got %v", stdDev)
	}
}

func TestIsValidKeySize(t *testing.T) {
	rsa := &RSABenchmark{}
	
	tests := []struct {
		size     int
		expected bool
	}{
		{1024, true},
		{2048, true},
		{4096, true},
		{512, false},
		{1536, false},
		{16384, true},
		{32768, false},
	}
	
	for _, test := range tests {
		result := isValidKeySize(rsa, test.size)
		if result != test.expected {
			t.Errorf("For size %d, expected %v, got %v", test.size, test.expected, result)
		}
	}
}

func TestRunnerBasic(t *testing.T) {
	config := Config{
		Algorithms:   []string{"ed25519"},
		KeySizes:     []int{256},
		Iterations:   2,
		Parallel:     1,
		ShowProgress: false,
		Timeout:      10,
		Verbose:      false,
	}
	
	runner := NewRunner(config)
	results, err := runner.Run()
	
	if err != nil {
		t.Fatalf("Runner failed: %v", err)
	}
	
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
	
	if results[0].Algorithm != "Ed25519" {
		t.Errorf("Expected Ed25519 algorithm, got %s", results[0].Algorithm)
	}
	
	if results[0].Iterations != 2 {
		t.Errorf("Expected 2 iterations, got %d", results[0].Iterations)
	}
	
	if results[0].Errors != 0 {
		t.Errorf("Expected 0 errors, got %d", results[0].Errors)
	}
}