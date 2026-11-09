package archive

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type AccountSection struct {
	Name  string   `json:"name"`
	Data  any      `json:"data,omitempty"`
	Files []string `json:"files,omitempty"`
	Error string   `json:"error,omitempty"`
}
type AccountReport struct {
	Summary    Summary          `json:"summary"`
	Warnings   []string         `json:"warnings,omitempty"`
	AccountID  int64            `json:"account_id"`
	ExportedAt time.Time        `json:"exported_at"`
	Sections   []AccountSection `json:"sections"`
}

// WriteAccount keeps account metadata separate from chat exports. Failed sections
// are reported without overwriting their previously successful section files.
func WriteAccount(dir, format string, report AccountReport) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	for _, section := range report.Sections {
		if section.Error == "" && section.Data != nil && (format == "json" || format == "both") {
			if err := writeJSON(filepath.Join(dir, section.Name+".json"), section.Data); err != nil {
				return err
			}
		}
	}
	if err := writeJSON(filepath.Join(dir, "account-report.json"), report); err != nil {
		return err
	}
	if format == "json" || format == "both" {
		if err := writeAccountJSON(dir, report); err != nil {
			return err
		}
	}
	if format == "html" || format == "both" {
		return writeAccountHTML(dir, report)
	}
	return nil
}

func plainValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// DownloadItems applies the same media filters, cache and integrity checks used
// by chats to a bounded page of account photos or stories.
func DownloadItems(ctx context.Context, source Source, dir string, items []Item, opts Options) error {
	opts = summaryOptions(opts, dir)
	return downloadPage(ctx, source, dir, items, opts)
}

func WriteData(path string, value any) error { return writeJSON(path, value) }
