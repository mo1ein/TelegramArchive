// Package taskui carries bounded progress frames from isolated task workers.
package taskui

import (
	"encoding/json"
	"fmt"
	"io"
)

const ProgressPrefix = "ta-progress;"

// Progress carries measurements separately from the terminal's presentation.
// Percent is a fraction in [0, 1]; scans have no reliable total yet.
type Progress struct {
	Label          string  `json:"label"`
	Detail         string  `json:"detail"`
	Summary        string  `json:"summary"`
	Percent        float64 `json:"percent"`
	Determinate    bool    `json:"determinate"`
	CompletedFiles int64   `json:"completed_files"`
	TotalFiles     int64   `json:"total_files"`
}

func WriteStatus(out io.Writer, status Progress) {
	writeFrame(out, status)
}

func WriteProgress(out io.Writer, lines []string) {
	writeFrame(out, lines)
}

func writeFrame(out io.Writer, value any) {
	data, err := json.Marshal(value)
	if err != nil || len(data) > 4096 {
		return
	}
	_, _ = fmt.Fprintf(out, "\x1b]777;%s%s\a", ProgressPrefix, data)
}
