package benchmark

import "time"

type Config struct {
	Algorithms   []string `json:"algorithms"`
	KeySizes     []int    `json:"key_sizes"`
	Iterations   int      `json:"iterations"`
	Parallel     int      `json:"parallel"`
	Workers      int      `json:"workers"`       // Number of worker threads (default: 1)
	ShowProgress bool     `json:"show_progress"`
	Timeout      int      `json:"timeout"`
	Verbose      bool     `json:"verbose"`
	ChunkSize    int      `json:"chunk_size"`     // For streaming large keys
	FileStorage  bool     `json:"file_storage"`   // Force file storage
	AutoCleanup  int      `json:"auto_cleanup"`   // Minutes before cleanup (0 = no cleanup)
}

type Result struct {
	Algorithm       string        `json:"algorithm"`
	KeySize         int           `json:"key_size"`
	Iterations      int           `json:"iterations"`
	Parallel        int           `json:"parallel"`
	TotalTime       time.Duration `json:"total_time"`
	AverageTime     time.Duration `json:"average_time"`
	MinTime         time.Duration `json:"min_time"`
	MaxTime         time.Duration `json:"max_time"`
	StdDev          time.Duration `json:"std_dev"`
	KeysPerSecond   float64       `json:"keys_per_second"`
	CPUUsage        float64       `json:"cpu_usage"`
	MemoryUsed      uint64        `json:"memory_used"`
	Errors          int           `json:"errors"`
	CompletedAt     time.Time     `json:"completed_at"`
	FileStorageUsed bool          `json:"file_storage_used"`
}

type KeyWriter interface {
	Write([]byte) (int, error)
}

type AlgorithmBenchmark interface {
	Name() string
	SupportedKeySizes() []int
	GenerateKey(size int) (any, error)
	GenerateKeyStreaming(size int, writer KeyWriter) error
}