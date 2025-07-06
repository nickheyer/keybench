package output

import (
	"fmt"
	"io"

	"github.com/user/keybench/internal/benchmark"
	"github.com/user/keybench/pkg/sysinfo"
)

type Data struct {
	SystemInfo *sysinfo.SystemInfo
	Results    []benchmark.Result
	Config     benchmark.Config
}

type Formatter interface {
	Format(w io.Writer, data Data) error
}

func NewFormatter(format string) (Formatter, error) {
	switch format {
	case "table":
		return &TableFormatter{}, nil
	case "json":
		return &JSONFormatter{}, nil
	case "csv":
		return &CSVFormatter{}, nil
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}