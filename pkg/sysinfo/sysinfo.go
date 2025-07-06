package sysinfo

import (
	"runtime"
	"strings"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
)

type SystemInfo struct {
	OS           string  `json:"os"`
	Architecture string  `json:"architecture"`
	CPUModel     string  `json:"cpu_model"`
	CPUCores     int     `json:"cpu_cores"`
	CPUThreads   int     `json:"cpu_threads"`
	TotalMemory  uint64  `json:"total_memory"`
	GoVersion    string  `json:"go_version"`
	Hostname     string  `json:"hostname"`
	Platform     string  `json:"platform"`
	LoadAverage  float64 `json:"load_average"`
}

func Collect() (*SystemInfo, error) {
	info := &SystemInfo{
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		GoVersion:    runtime.Version(),
	}

	// Get CPU info
	cpuInfo, err := cpu.Info()
	if err == nil && len(cpuInfo) > 0 {
		info.CPUModel = strings.TrimSpace(cpuInfo[0].ModelName)
	}

	info.CPUCores = runtime.NumCPU()

	// Get logical CPU count (threads)
	threads, err := cpu.Counts(true)
	if err == nil {
		info.CPUThreads = threads
	}

	// Get memory info
	memInfo, err := mem.VirtualMemory()
	if err == nil {
		info.TotalMemory = memInfo.Total
	}

	// Get host info
	hostInfo, err := host.Info()
	if err == nil {
		info.Hostname = hostInfo.Hostname
		info.Platform = hostInfo.Platform
	}

	// Get load average
	loadAvg, err := load.Avg()
	if err == nil {
		info.LoadAverage = loadAvg.Load1
	}

	return info, nil
}