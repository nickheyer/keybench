package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/user/keybench/internal/benchmark"
	"github.com/user/keybench/pkg/sysinfo"
)

func TestNewFormatter(t *testing.T) {
	tests := []struct {
		format    string
		expectErr bool
	}{
		{"table", false},
		{"json", false},
		{"csv", false},
		{"xml", true},
		{"invalid", true},
	}

	for _, test := range tests {
		_, err := NewFormatter(test.format)
		if test.expectErr && err == nil {
			t.Errorf("Expected error for format %s", test.format)
		}
		if !test.expectErr && err != nil {
			t.Errorf("Unexpected error for format %s: %v", test.format, err)
		}
	}
}

func TestJSONFormatter(t *testing.T) {
	formatter := &JSONFormatter{}
	buf := &bytes.Buffer{}

	data := Data{
		SystemInfo: &sysinfo.SystemInfo{
			OS:           "linux",
			Architecture: "amd64",
			CPUModel:     "Test CPU",
			CPUCores:     8,
			TotalMemory:  16000000000,
		},
		Results: []benchmark.Result{
			{
				Algorithm:     "RSA",
				KeySize:       2048,
				Iterations:    10,
				Parallel:      1,
				TotalTime:     5 * time.Second,
				AverageTime:   500 * time.Millisecond,
				KeysPerSecond: 2.0,
				Errors:        0,
				CompletedAt:   time.Now(),
			},
		},
		Config: benchmark.Config{
			Algorithms: []string{"rsa"},
			KeySizes:   []int{2048},
			Iterations: 10,
			Parallel:   1,
		},
	}

	err := formatter.Format(buf, data)
	if err != nil {
		t.Fatalf("JSON formatting failed: %v", err)
	}

	// Verify it's valid JSON
	var result map[string]any
	err = json.Unmarshal(buf.Bytes(), &result)
	if err != nil {
		t.Errorf("Invalid JSON output: %v", err)
	}

	// Check for required fields
	if _, ok := result["system_info"]; !ok {
		t.Error("Missing system_info in JSON output")
	}
	if _, ok := result["results"]; !ok {
		t.Error("Missing results in JSON output")
	}
	if _, ok := result["summary"]; !ok {
		t.Error("Missing summary in JSON output")
	}
}

func TestCSVFormatter(t *testing.T) {
	formatter := &CSVFormatter{}
	buf := &bytes.Buffer{}

	data := Data{
		SystemInfo: &sysinfo.SystemInfo{
			OS:           "linux",
			Architecture: "amd64",
			CPUModel:     "Test CPU",
			CPUCores:     8,
			TotalMemory:  16000000000,
		},
		Results: []benchmark.Result{
			{
				Algorithm:     "RSA",
				KeySize:       2048,
				Iterations:    10,
				Parallel:      1,
				TotalTime:     5 * time.Second,
				AverageTime:   500 * time.Millisecond,
				KeysPerSecond: 2.0,
				Errors:        0,
				CompletedAt:   time.Now(),
			},
		},
	}

	err := formatter.Format(buf, data)
	if err != nil {
		t.Fatalf("CSV formatting failed: %v", err)
	}

	lines := strings.Split(buf.String(), "\n")
	if len(lines) < 2 {
		t.Error("CSV output should have at least header and one data row")
	}

	// Check header
	if !strings.Contains(lines[0], "Algorithm") {
		t.Error("CSV header missing Algorithm field")
	}
	if !strings.Contains(lines[0], "KeySize") {
		t.Error("CSV header missing KeySize field")
	}
}

func TestTableFormatter(t *testing.T) {
	formatter := &TableFormatter{}
	buf := &bytes.Buffer{}

	data := Data{
		SystemInfo: &sysinfo.SystemInfo{
			OS:           "linux",
			Architecture: "amd64",
			CPUModel:     "Test CPU",
			CPUCores:     8,
			TotalMemory:  16000000000,
		},
		Results: []benchmark.Result{
			{
				Algorithm:     "RSA",
				KeySize:       2048,
				Iterations:    10,
				Parallel:      1,
				TotalTime:     5 * time.Second,
				AverageTime:   500 * time.Millisecond,
				MinTime:       400 * time.Millisecond,
				MaxTime:       600 * time.Millisecond,
				KeysPerSecond: 2.0,
				CPUUsage:      50.5,
				MemoryUsed:    1048576,
				Errors:        0,
				CompletedAt:   time.Now(),
			},
		},
	}

	err := formatter.Format(buf, data)
	if err != nil {
		t.Fatalf("Table formatting failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Benchmark Results") {
		t.Error("Table output missing title")
	}
	if !strings.Contains(output, "RSA") {
		t.Error("Table output missing algorithm name")
	}
	if !strings.Contains(output, "2048") {
		t.Error("Table output missing key size")
	}
	if !strings.Contains(output, "Summary") {
		t.Error("Table output missing summary section")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{500 * time.Nanosecond, "0.50µs"},
		{1500 * time.Microsecond, "1.50ms"},
		{2500 * time.Millisecond, "2.50s"},
		{150 * time.Second, "2.50m"},
	}

	for _, test := range tests {
		result := formatDuration(test.duration)
		if result != test.expected {
			t.Errorf("For duration %v, expected %s, got %s", test.duration, test.expected, result)
		}
	}
}
