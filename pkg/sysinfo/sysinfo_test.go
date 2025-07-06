package sysinfo

import (
	"runtime"
	"testing"
)

func TestCollect(t *testing.T) {
	info, err := Collect()
	if err != nil {
		t.Fatalf("Failed to collect system info: %v", err)
	}

	// Check basic fields that should always be present
	if info.OS == "" {
		t.Error("OS field is empty")
	}

	if info.OS != runtime.GOOS {
		t.Errorf("OS mismatch: expected %s, got %s", runtime.GOOS, info.OS)
	}

	if info.Architecture == "" {
		t.Error("Architecture field is empty")
	}

	if info.Architecture != runtime.GOARCH {
		t.Errorf("Architecture mismatch: expected %s, got %s", runtime.GOARCH, info.Architecture)
	}

	if info.GoVersion == "" {
		t.Error("GoVersion field is empty")
	}

	if info.GoVersion != runtime.Version() {
		t.Errorf("Go version mismatch: expected %s, got %s", runtime.Version(), info.GoVersion)
	}

	if info.CPUCores == 0 {
		t.Error("CPUCores should not be 0")
	}

	if info.CPUCores != runtime.NumCPU() {
		t.Errorf("CPU cores mismatch: expected %d, got %d", runtime.NumCPU(), info.CPUCores)
	}

	// These fields might not be available on all systems, so just check they're not negative
	if info.TotalMemory <= 0 {
		t.Error("TotalMemory should not be negative")
	}

	if info.LoadAverage < 0 {
		t.Error("LoadAverage should not be negative")
	}
}
