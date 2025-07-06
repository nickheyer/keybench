package output

import (
	"encoding/json"
	"io"
	"time"
)

type JSONFormatter struct{}

type JSONOutput struct {
	Timestamp  time.Time `json:"timestamp"`
	SystemInfo any       `json:"system_info"`
	Config     any       `json:"config"`
	Results    any       `json:"results"`
	Summary    struct {
		TotalKeys       int           `json:"total_keys"`
		TotalTime       time.Duration `json:"total_time"`
		TotalTimeString string        `json:"total_time_string"`
		Throughput      float64       `json:"throughput_keys_per_sec"`
	} `json:"summary"`
}

func (j *JSONFormatter) Format(w io.Writer, data Data) error {
	output := JSONOutput{
		Timestamp:  time.Now(),
		SystemInfo: data.SystemInfo,
		Config: map[string]any{
			"algorithms":    data.Config.Algorithms,
			"key_sizes":     data.Config.KeySizes,
			"iterations":    data.Config.Iterations,
			"parallel":      data.Config.Parallel,
			"timeout":       data.Config.Timeout,
			"show_progress": data.Config.ShowProgress,
			"verbose":       data.Config.Verbose,
		},
		Results: data.Results,
	}

	// Calculate summary
	totalKeys := 0
	totalTime := time.Duration(0)
	for _, result := range data.Results {
		totalKeys += (result.Iterations * result.Parallel) - result.Errors
		totalTime += result.TotalTime
	}

	output.Summary.TotalKeys = totalKeys
	output.Summary.TotalTime = totalTime
	output.Summary.TotalTimeString = totalTime.String()
	if totalTime > 0 {
		output.Summary.Throughput = float64(totalKeys) / totalTime.Seconds()
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}
