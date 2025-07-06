package main

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
	"github.com/user/keybench/internal/benchmark"
	"github.com/user/keybench/internal/output"
	"github.com/user/keybench/internal/server"
	"github.com/user/keybench/pkg/sysinfo"
)

var (
	algorithms   []string
	keySizes     []int
	iterations   int
	parallel     int
	outputFormat string
	outputFile   string
	verbose      bool
	showProgress bool
	timeout      int
	webMode      bool
	webPort      string
)

var rootCmd = &cobra.Command{
	Use:   "keybench",
	Short: "A comprehensive cryptographic key generation benchmark tool",
	Long: `KeyBench is a production-ready system benchmarking tool that uses
cryptographic key generation to stress test CPU performance.

It supports multiple algorithms (RSA, ECDSA, Ed25519) with various key sizes
and provides detailed performance metrics including throughput, latency, and
system resource utilization.`,
	RunE: runBenchmark,
}

func init() {
	rootCmd.Flags().StringSliceVarP(&algorithms, "algorithms", "a", []string{"rsa"}, "Algorithms to benchmark (rsa, ecdsa, ed25519)")
	rootCmd.Flags().IntSliceVarP(&keySizes, "key-sizes", "k", []int{2048, 4096}, "Key sizes to test (varies by algorithm)")
	rootCmd.Flags().IntVarP(&iterations, "iterations", "i", 10, "Number of iterations per test")
	rootCmd.Flags().IntVarP(&parallel, "parallel", "p", 1, "Number of parallel workers")
	rootCmd.Flags().StringVarP(&outputFormat, "format", "f", "table", "Output format (table, json, csv)")
	rootCmd.Flags().StringVarP(&outputFile, "output", "o", "", "Output file (default: stdout)")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")
	rootCmd.Flags().BoolVar(&showProgress, "progress", true, "Show progress bar")
	rootCmd.Flags().IntVarP(&timeout, "timeout", "t", 300, "Timeout in seconds per test")
	rootCmd.Flags().BoolVarP(&webMode, "web", "w", false, "Run in web server mode")
	rootCmd.Flags().StringVar(&webPort, "port", "8080", "Web server port")
}

func runBenchmark(cmd *cobra.Command, args []string) error {
	// Check if we should run in web mode
	if webMode || (len(os.Args) == 1) {
		// Run web server
		srv, err := server.NewServer(webPort)
		if err != nil {
			return fmt.Errorf("failed to create server: %w", err)
		}
		
		log.Printf("Starting KeyBench web server on http://localhost:%s", webPort)
		log.Println("Press Ctrl+C to stop")
		
		return srv.Start()
	}

	// CLI mode
	if verbose {
		fmt.Println("KeyBench - Cryptographic Key Generation Benchmark")
		fmt.Println("================================================")
		fmt.Println()
	}

	// Collect system information
	sysInfo, err := sysinfo.Collect()
	if err != nil {
		return fmt.Errorf("failed to collect system info: %w", err)
	}

	if verbose {
		fmt.Printf("System Information:\n")
		fmt.Printf("  OS: %s\n", sysInfo.OS)
		fmt.Printf("  Architecture: %s\n", sysInfo.Architecture)
		fmt.Printf("  CPU: %s (%d cores)\n", sysInfo.CPUModel, sysInfo.CPUCores)
		fmt.Printf("  Memory: %.2f GB\n", float64(sysInfo.TotalMemory)/(1024*1024*1024))
		fmt.Printf("  Go Version: %s\n", sysInfo.GoVersion)
		fmt.Println()
	}

	// Create benchmark configuration
	config := benchmark.Config{
		Algorithms:   algorithms,
		KeySizes:     keySizes,
		Iterations:   iterations,
		Parallel:     parallel,
		ShowProgress: showProgress,
		Timeout:      timeout,
		Verbose:      verbose,
	}

	// Run benchmarks
	runner := benchmark.NewRunner(config)
	results, err := runner.Run()
	if err != nil {
		return fmt.Errorf("benchmark failed: %w", err)
	}

	// Format and output results
	formatter, err := output.NewFormatter(outputFormat)
	if err != nil {
		return fmt.Errorf("invalid output format: %w", err)
	}

	outputData := output.Data{
		SystemInfo: sysInfo,
		Results:    results,
		Config:     config,
	}

	var writer *os.File
	if outputFile != "" {
		writer, err = os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer writer.Close()
	} else {
		writer = os.Stdout
	}

	if err := formatter.Format(writer, outputData); err != nil {
		return fmt.Errorf("failed to format output: %w", err)
	}

	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}