# KeyBench - Cryptographic Key Generation Benchmark Tool

KeyBench is a production-ready system benchmarking tool that uses cryptographic key generation to stress test CPU performance. It runs as a web server by default with a browser-based interface, or can be used as a CLI tool.

## Features

- **Web Interface**: Browser-based UI for easy benchmark configuration and monitoring
- **Key Storage**: Generated keys are stored and downloadable in PEM format
- **Real-time Progress**: Live updates during benchmark execution
- **Benchmark History**: Track and compare previous benchmark runs
- **Multiple Algorithms**: RSA (1024-16384 bits), ECDSA (P-224/256/384/521), Ed25519
- **Parallel Execution**: Configurable number of parallel workers
- **Multiple Output Formats**: Table (default), JSON, CSV (CLI mode)
- **System Information**: Displays detailed CPU, memory, and system metrics
- **REST API**: Full API for programmatic access
- **Docker Support**: Pre-built container image available

## Installation

### From Source

```bash
git clone https://github.com/user/keybench.git
cd keybench
go mod download
go build -o keybench ./cmd/main.go
```

### Using Make

```bash
make build
make install
```

### Docker

```bash
# Build image
docker build -t keybench .

# Run web server
docker run -p 8080:8080 keybench

# Run CLI mode
docker run --rm keybench -a rsa -k 2048 -i 10
```

## Usage

### Web Server Mode (Default)

```bash
# Start web server on default port 8080
./keybench

# Start on custom port
./keybench --port 9000

# Access the web interface
# Open http://localhost:8080 in your browser
```

### CLI Mode

```bash
# Run CLI benchmark (provide any benchmark arguments)
./keybench -a rsa -k 2048

# Benchmark all algorithms
./keybench -a rsa,ecdsa,ed25519 -k 2048,256,256

# Custom key sizes and iterations
./keybench -a rsa -k 2048,4096,8192 -i 20

# Parallel execution
./keybench -a rsa -k 4096 -i 100 -p 4

# Output to JSON file
./keybench -a rsa -f json -o results.json
```

### Command Line Options

```
Flags:
  -a, --algorithms strings   Algorithms to benchmark (rsa, ecdsa, ed25519) (default [rsa])
  -k, --key-sizes ints      Key sizes to test (varies by algorithm) (default [2048,4096])
  -i, --iterations int      Number of iterations per test (default 10)
  -p, --parallel int        Number of parallel workers (default 1)
  -f, --format string       Output format (table, json, csv) (default "table")
  -o, --output string       Output file (default: stdout)
  -v, --verbose             Verbose output
      --progress            Show progress bar (default true)
  -t, --timeout int         Timeout in seconds per test (default 300)
  -w, --web                 Run in web server mode
      --port string         Web server port (default "8080")
  -h, --help               help for keybench
```

### Web Interface Features

When running in web server mode (default), KeyBench provides:

1. **System Information Dashboard**: Real-time display of CPU, memory, and system details
2. **Benchmark Configuration**: Easy-to-use form for selecting algorithms and parameters
3. **Live Progress Monitoring**: Real-time progress updates during benchmark execution
4. **Key Management**: 
   - Download generated keys in PEM format (public/private)
   - Delete individual keys or clear all
   - Filter keys by benchmark run
5. **Benchmark History**: View and compare previous benchmark results
6. **REST API**: Full API access at `/api/v1/` for automation

### REST API Endpoints

- `GET /api/v1/system-info` - Get system information
- `POST /api/v1/benchmarks` - Start a new benchmark
- `GET /api/v1/benchmarks` - List all benchmarks
- `GET /api/v1/benchmarks/{id}` - Get benchmark details
- `GET /api/v1/keys` - List all generated keys
- `GET /api/v1/keys/{id}` - Get key details
- `GET /api/v1/keys/{id}/download?type=public|private` - Download key
- `DELETE /api/v1/keys/{id}` - Delete a key

### Supported Key Sizes

- **RSA**: 1024, 2048, 3072, 4096, 8192, 16384 bits
- **ECDSA**: 224 (P-224), 256 (P-256), 384 (P-384), 521 (P-521) bits
- **Ed25519**: 256 bits (fixed)

## Examples

### Benchmark RSA Key Generation

```bash
./keybench -a rsa -k 2048,4096 -i 50 -v
```

### Compare All Algorithms

```bash
./keybench -a rsa,ecdsa,ed25519 -k 2048,256,256 -i 100 -p 4
```

### Export Results to CSV

```bash
./keybench -a rsa -k 1024,2048,4096,8192 -i 20 -f csv -o benchmark_results.csv
```

### JSON Output for Automation

```bash
./keybench -a ecdsa -k 256,384,521 -f json | jq '.summary'
```

## Output Formats

### Table Format (Default)

```
Benchmark Results
================

Algorithm | Key Size | Iterations | Parallel | Total Time | Avg Time | Min Time | Max Time | Keys/Sec | CPU % | Memory MB | Errors
----------|----------|------------|----------|------------|----------|----------|----------|----------|-------|-----------|--------
RSA       | 2048     | 10         | 1        | 1.52s      | 152.35ms | 145.12ms | 165.89ms | 6.57     | 98.5  | 12.45     | 0
RSA       | 4096     | 10         | 1        | 5.89s      | 589.23ms | 567.34ms | 612.45ms | 1.70     | 99.2  | 18.23     | 0

Summary
-------
Total keys generated: 20
Total time: 7.41s
Overall throughput: 2.70 keys/sec
```

### JSON Format

```json
{
  "timestamp": "2024-01-15T10:23:45Z",
  "system_info": {
    "os": "linux",
    "architecture": "amd64",
    "cpu_model": "Intel(R) Core(TM) i7-9750H CPU @ 2.60GHz",
    "cpu_cores": 12,
    "total_memory": 16777216000,
    "go_version": "go1.21.5"
  },
  "results": [...],
  "summary": {
    "total_keys": 20,
    "total_time": 7410000000,
    "throughput_keys_per_sec": 2.699595
  }
}
```

## Performance Considerations

1. **RSA Key Generation**: CPU-intensive, scales with key size exponentially
2. **ECDSA**: Faster than RSA for equivalent security levels
3. **Ed25519**: Fastest algorithm, fixed key size, recommended for modern applications
4. **Parallel Execution**: Scales linearly with CPU cores for most workloads

## System Requirements

- Go 1.21 or higher (for building from source)
- Linux, macOS, or Windows
- Minimum 1GB RAM
- Multi-core CPU recommended for parallel benchmarks

## Development

### Running Tests

```bash
make test
make test-coverage
```

### Building

```bash
make build
make docker-build
```

### Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Acknowledgments

- Uses [cobra](https://github.com/spf13/cobra) for CLI
- Uses [gopsutil](https://github.com/shirou/gopsutil) for system information
- Uses [progressbar](https://github.com/schollz/progressbar) for progress tracking
- Uses [tablewriter](https://github.com/olekukonko/tablewriter) for table formatting