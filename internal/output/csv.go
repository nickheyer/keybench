package output

import (
	"encoding/csv"
	"fmt"
	"io"
	"time"
)

type CSVFormatter struct{}

func (c *CSVFormatter) Format(w io.Writer, data Data) error {
	writer := csv.NewWriter(w)
	defer writer.Flush()
	
	// Write header
	header := []string{
		"Timestamp",
		"Algorithm",
		"KeySize",
		"Iterations",
		"Parallel",
		"TotalTime(ms)",
		"AverageTime(ms)",
		"MinTime(ms)",
		"MaxTime(ms)",
		"StdDev(ms)",
		"KeysPerSecond",
		"CPUUsage(%)",
		"MemoryUsed(MB)",
		"Errors",
		"OS",
		"Architecture",
		"CPUModel",
		"CPUCores",
		"TotalMemory(GB)",
	}
	
	if err := writer.Write(header); err != nil {
		return err
	}
	
	// Write data rows
	for _, result := range data.Results {
		row := []string{
			result.CompletedAt.Format(time.RFC3339),
			result.Algorithm,
			fmt.Sprintf("%d", result.KeySize),
			fmt.Sprintf("%d", result.Iterations),
			fmt.Sprintf("%d", result.Parallel),
			fmt.Sprintf("%.2f", float64(result.TotalTime.Nanoseconds())/1e6),
			fmt.Sprintf("%.2f", float64(result.AverageTime.Nanoseconds())/1e6),
			fmt.Sprintf("%.2f", float64(result.MinTime.Nanoseconds())/1e6),
			fmt.Sprintf("%.2f", float64(result.MaxTime.Nanoseconds())/1e6),
			fmt.Sprintf("%.2f", float64(result.StdDev.Nanoseconds())/1e6),
			fmt.Sprintf("%.2f", result.KeysPerSecond),
			fmt.Sprintf("%.2f", result.CPUUsage),
			fmt.Sprintf("%.2f", float64(result.MemoryUsed)/(1024*1024)),
			fmt.Sprintf("%d", result.Errors),
			data.SystemInfo.OS,
			data.SystemInfo.Architecture,
			data.SystemInfo.CPUModel,
			fmt.Sprintf("%d", data.SystemInfo.CPUCores),
			fmt.Sprintf("%.2f", float64(data.SystemInfo.TotalMemory)/(1024*1024*1024)),
		}
		
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	
	return nil
}