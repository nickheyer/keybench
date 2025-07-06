package output

import (
	"fmt"
	"io"
	"time"

	"github.com/olekukonko/tablewriter"
)

type TableFormatter struct{}

func (t *TableFormatter) Format(w io.Writer, data Data) error {
	fmt.Fprintln(w, "\nBenchmark Results")
	fmt.Fprintln(w, "================")
	fmt.Fprintln(w)
	
	table := tablewriter.NewWriter(w)
	table.SetHeader([]string{
		"Algorithm",
		"Key Size",
		"Iterations",
		"Parallel",
		"Total Time",
		"Avg Time",
		"Min Time",
		"Max Time",
		"Keys/Sec",
		"CPU %",
		"Memory MB",
		"Errors",
	})
	
	table.SetBorder(false)
	table.SetCenterSeparator("|")
	table.SetColumnSeparator("|")
	table.SetRowSeparator("-")
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	
	for _, result := range data.Results {
		row := []string{
			result.Algorithm,
			fmt.Sprintf("%d", result.KeySize),
			fmt.Sprintf("%d", result.Iterations),
			fmt.Sprintf("%d", result.Parallel),
			formatDuration(result.TotalTime),
			formatDuration(result.AverageTime),
			formatDuration(result.MinTime),
			formatDuration(result.MaxTime),
			fmt.Sprintf("%.2f", result.KeysPerSecond),
			fmt.Sprintf("%.1f", result.CPUUsage),
			fmt.Sprintf("%.2f", float64(result.MemoryUsed)/(1024*1024)),
			fmt.Sprintf("%d", result.Errors),
		}
		table.Append(row)
	}
	
	table.Render()
	
	// Summary statistics
	fmt.Fprintln(w, "\nSummary")
	fmt.Fprintln(w, "-------")
	
	totalKeys := 0
	totalTime := time.Duration(0)
	for _, result := range data.Results {
		totalKeys += (result.Iterations * result.Parallel) - result.Errors
		totalTime += result.TotalTime
	}
	
	fmt.Fprintf(w, "Total keys generated: %d\n", totalKeys)
	fmt.Fprintf(w, "Total time: %s\n", formatDuration(totalTime))
	if totalTime > 0 {
		fmt.Fprintf(w, "Overall throughput: %.2f keys/sec\n", float64(totalKeys)/totalTime.Seconds())
	}
	
	return nil
}

func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%.2fµs", float64(d.Nanoseconds())/1000)
	} else if d < time.Second {
		return fmt.Sprintf("%.2fms", float64(d.Nanoseconds())/1e6)
	} else if d < time.Minute {
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
	return fmt.Sprintf("%.2fm", d.Minutes())
}