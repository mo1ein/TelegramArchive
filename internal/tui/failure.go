package tui

import (
	"fmt"
	"regexp"
	"strings"
)

var proxyUserinfo = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^\s/]*@`)

func redactOutput(text string) string {
	return proxyUserinfo.ReplaceAllString(text, "${1}<hidden>@")
}

func taskFailure(err error, output string) error {
	if err == nil {
		return nil
	}
	// Cobra's final Error line carries the actual transport/session failure.
	// Keep it ahead of the process exit status, which is only an implementation detail.
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if _, message, ok := strings.Cut(line, "Error:"); ok && strings.TrimSpace(message) != "" {
			return fmt.Errorf("%s: %w", strings.TrimSpace(redactOutput(safeText(message))), err)
		}
	}
	return err
}
