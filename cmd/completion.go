package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/mo1ein/TelegramArchive/internal/archive"
	"github.com/spf13/cobra"
)

const flagResultFile = "result-file"

func completionPath(cmd *cobra.Command) string {
	flag := cmd.Root().PersistentFlags().Lookup(flagResultFile)
	if flag == nil {
		return ""
	}
	return flag.Value.String()
}
func writeCompletion(path, kind string, started time.Time, summary archive.Summary, warnings []string, err, contextErr error) error {
	if path == "" {
		return nil
	}
	report := archive.CompletionReport{Kind: kind, Status: archive.SummaryStatus(err, summary, warnings), Started: started, Finished: time.Now().UTC(), Summary: summary, Warnings: warnings}
	if errors.Is(contextErr, context.Canceled) {
		report.Status = "canceled"
	}
	if err != nil {
		report.Error = err.Error()
	}
	return archive.WriteData(path, report)
}
func readCompletion(path string) (archive.CompletionReport, error) {
	var report archive.CompletionReport
	raw, err := os.ReadFile(path)
	if err != nil {
		return report, err
	}
	err = json.Unmarshal(raw, &report)
	return report, err
}
