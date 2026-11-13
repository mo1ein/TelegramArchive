// Package taskui carries bounded progress frames from isolated task workers.
package taskui

import (
	"encoding/json"
	"fmt"
	"io"
)

const ProgressPrefix = "ta-progress;"

func WriteProgress(out io.Writer, lines []string) {
	data, err := json.Marshal(lines)
	if err != nil || len(data) > 4096 {
		return
	}
	_, _ = fmt.Fprintf(out, "\x1b]777;%s%s\a", ProgressPrefix, data)
}
