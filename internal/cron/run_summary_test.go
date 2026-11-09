package cron

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mo1ein/TelegramArchive/internal/archive"
)

func requireSummaryCount(t *testing.T, text, label string, count int64) {
	t.Helper()
	// Permit either "2 downloaded" or "Downloaded: 2" without fixing layout,
	// spacing, row order or surrounding explanatory text.
	pattern := fmt.Sprintf(`(?i)(?:\b%d\s+%s\b|\b%s\b[^\d\n]{0,24}\b%d\b)`, count, regexp.QuoteMeta(label), regexp.QuoteMeta(label), count)
	if regexp.MustCompile(pattern).MatchString(text) {
		return
	}
	// Match numeric cells under their header, allowing multiword media labels
	// and any amount of padding from the table writer.
	column, columns := -1, 0
	number := regexp.MustCompile(`^\d+$`)
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 1 && strings.EqualFold(fields[0], "Type") {
			columns = len(fields) - 1
			for i, header := range fields[1:] {
				if strings.EqualFold(header, label) {
					column = i
				}
			}
			continue
		}
		if column < 0 || len(fields) <= columns {
			continue
		}
		cells := fields[len(fields)-columns:]
		valid := true
		for _, cell := range cells {
			valid = valid && number.MatchString(cell)
		}
		if valid && cells[column] == fmt.Sprint(count) {
			return
		}
	}
	t.Errorf("missing %s count %d in:\n%s", label, count, text)
}

func TestRunSummaryReflectsChildOutcome(t *testing.T) {
	runnerFailure := errors.New("media connection failed")
	for _, test := range []struct {
		name, status, phrase string
		summary              archive.Summary
		runnerError          error
	}{
		{
			name: "complete", status: "complete", phrase: "completed",
			summary: archive.Summary{HasMessages: true, MessagesKnown: true, Messages: 120, MessagesAdded: 4, MediaKnown: true, DownloadedBytes: 2048,
				Media: map[string]archive.MediaCounts{"videos": {Downloaded: 2, Reused: 5, Filtered: 1}}},
		},
		{
			name: "warnings", status: "complete_with_warnings", phrase: "completed with warnings",
			summary: archive.Summary{HasMessages: true, MessagesKnown: true, Messages: 120, MediaKnown: true, DownloadedBytes: 2048,
				Media: map[string]archive.MediaCounts{"videos": {Downloaded: 2, Reused: 5}, "thumbnails": {Unavailable: 1}}},
		},
		{
			name: "skipped", status: "skipped", phrase: "skipped",
			summary: archive.Summary{HasMessages: true},
		},
		{
			name: "failed", status: "failed", phrase: "failed", runnerError: runnerFailure,
			summary: archive.Summary{HasMessages: true, MessagesKnown: true, Messages: 90, MessagesAdded: 2, MediaKnown: true, DownloadedBytes: 1024,
				Media: map[string]archive.MediaCounts{"videos": {Downloaded: 1, Reused: 4, Failed: 2, Canceled: 3}}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			manager := Manager{Directory: filepath.Join(dir, "jobs"), Table: &memoryTable{}}
			job := sampleJob(dir)
			if err := manager.Install(context.Background(), job); err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(manager.File(job.Name))
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			var resultFile string
			err = Run(context.Background(), manager.File(job.Name), &output, func(ctx context.Context, run Job, log io.Writer) error {
				resultFile = run.ResultFile
				if resultFile == "" {
					t.Error("runner was not given a private completion result file")
					return test.runnerError
				}
				info, err := os.Stat(resultFile)
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode().Perm() != 0600 || info.Size() != 0 {
					t.Errorf("completion file must be private and fresh: mode=%o size=%d", info.Mode().Perm(), info.Size())
				}
				report := archive.CompletionReport{Kind: "backup", Status: test.status, Started: time.Now(), Summary: test.summary}
				report.Finished = time.Now()
				if test.status == "complete_with_warnings" {
					report.Warnings = []string{"one thumbnail is unavailable"}
				}
				if test.runnerError != nil {
					report.Error = test.runnerError.Error()
				}
				data, err := json.Marshal(report)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(resultFile, data, 0600); err != nil {
					t.Fatal(err)
				}
				return test.runnerError
			})
			if test.runnerError == nil && err != nil {
				t.Fatal(err)
			}
			if test.runnerError != nil && !errors.Is(err, test.runnerError) {
				t.Errorf("cron discarded the actual runner failure: %v", err)
			}
			text := output.String()
			if !strings.Contains(text, fmt.Sprintf("Finished job %q", job.Name)) || !strings.Contains(strings.ToLower(text), test.phrase) {
				t.Errorf("missing finished job identity or true status %q:\n%s", test.phrase, text)
			}
			if test.status == "skipped" {
				if strings.Contains(strings.ToLower(text), "backup complete") || strings.Contains(strings.ToLower(text), " completed") {
					t.Errorf("skipped cron job falsely claimed completion:\n%s", text)
				}
			} else {
				if !strings.Contains(strings.ToLower(text), "videos") || !strings.Contains(strings.ToLower(text), "messages") || !strings.Contains(text, fmt.Sprint(test.summary.Messages)) {
					t.Errorf("completion summary omitted the media type or archived message inventory:\n%s", text)
				}
				for _, counts := range test.summary.Media {
					for label, count := range map[string]int64{
						"downloaded": counts.Downloaded, "reused": counts.Reused, "filtered": counts.Filtered,
						"failed": counts.Failed, "unavailable": counts.Unavailable, "canceled": counts.Canceled,
					} {
						if count > 0 {
							requireSummaryCount(t, text, label, count)
						}
					}
				}
			}
			if resultFile != "" {
				if _, err := os.Stat(resultFile); !os.IsNotExist(err) {
					t.Errorf("private completion report was not removed: %s, %v", resultFile, err)
				}
			}
			saved, err := os.ReadFile(manager.File(job.Name))
			if err != nil || !bytes.Equal(saved, original) {
				t.Errorf("transient completion path changed saved job configuration: %v", err)
			}
			log, err := os.ReadFile(job.Log)
			if err != nil || !bytes.Equal(log, output.Bytes()) {
				t.Errorf("private job log and user-visible completion summary differ: %v", err)
			}
		})
	}
}

func TestRunSummaryUsesFreshReportsAndCannotReusePreviousCompletion(t *testing.T) {
	dir := t.TempDir()
	manager := Manager{Directory: filepath.Join(dir, "jobs"), Table: &memoryTable{}}
	job := sampleJob(dir)
	if err := manager.Install(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	var previousPath string
	for _, status := range []string{"complete", "skipped"} {
		var output bytes.Buffer
		err := Run(context.Background(), manager.File(job.Name), &output, func(ctx context.Context, run Job, log io.Writer) error {
			if run.ResultFile == "" {
				t.Error("runner did not receive a completion report path")
				return nil
			}
			if previousPath == run.ResultFile {
				t.Error("cron reused a completion report path across invocations")
			}
			previousPath = run.ResultFile
			info, err := os.Stat(run.ResultFile)
			if err != nil || info.Size() != 0 {
				t.Errorf("new invocation inherited an old result: %v", err)
			}
			report := archive.CompletionReport{Kind: "backup", Status: status, Started: time.Now(), Finished: time.Now()}
			if status == "complete" {
				report.Summary = archive.Summary{HasMessages: true, MessagesKnown: true, Messages: 987654321, MediaKnown: true,
					Media: map[string]archive.MediaCounts{"videos": {Downloaded: 12345}}}
			}
			data, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			return os.WriteFile(run.ResultFile, data, 0600)
		})
		if err != nil {
			t.Fatal(err)
		}
		if status == "skipped" && (strings.Contains(output.String(), "987654321") || strings.Contains(output.String(), "12345") || strings.Contains(strings.ToLower(output.String()), "completed")) {
			t.Errorf("skipped invocation reported the previous completed backup's results:\n%s", output.String())
		}
		if previousPath != "" {
			if _, err := os.Stat(previousPath); !os.IsNotExist(err) {
				t.Errorf("completion report survived invocation: %v", err)
			}
		}
	}
}
