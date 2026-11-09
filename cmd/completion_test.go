package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mo1ein/TelegramArchive/internal/archive"
	"github.com/spf13/cobra"
	bolt "go.etcd.io/bbolt"
)

func TestBackupCompletionCarriesChildSummaryToParent(t *testing.T) {
	dir := t.TempDir()
	result := filepath.Join(dir, "parent-result")
	root := &cobra.Command{Use: "ta"}
	root.PersistentFlags().String(flagResultFile, "", "")
	child := newSync(func(_ context.Context, _ *cobra.Command, args []string) error {
		var path string
		for _, arg := range args {
			if value, ok := strings.CutPrefix(arg, "--"+flagResultFile+"="); ok {
				path = value
			}
		}
		info, err := os.Stat(path)
		if err != nil || info.Size() != 0 {
			t.Fatalf("child did not receive a fresh result file: %v", err)
		}
		return archive.WriteData(path, archive.CompletionReport{Kind: "archive", Status: "complete_with_warnings", Started: time.Now(), Finished: time.Now(), Warnings: []string{"preview unavailable"}, Summary: archive.Summary{HasMessages: true, MessagesKnown: true, Messages: 3, MediaKnown: true, Media: map[string]archive.MediaCounts{"photos": {Downloaded: 2}, "thumbnails": {Unavailable: 1}}}})
	})
	root.AddCommand(child)
	root.SetArgs([]string{"sync", "--chat=demo", "--output=" + dir, "--" + flagResultFile + "=" + result})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	report, err := readCompletion(result)
	if err != nil || report.Kind != "backup" || report.Status != "complete_with_warnings" || report.Summary.Messages != 3 || report.Summary.Totals().Downloaded != 2 || report.Summary.Totals().Unavailable != 1 {
		t.Fatalf("job lost child outcomes: %+v err=%v", report, err)
	}
}

func TestSkippedBackupReportsSkipWithoutReusingOldSummary(t *testing.T) {
	dir := t.TempDir()
	result := filepath.Join(dir, "parent-result")
	lock, err := bolt.Open(filepath.Join(dir, ".backup.lock"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	old := []byte("previous active backup report")
	if err = os.WriteFile(filepath.Join(dir, "backup-report.json"), old, 0600); err != nil {
		t.Fatal(err)
	}
	root := &cobra.Command{Use: "ta"}
	root.PersistentFlags().String(flagResultFile, "", "")
	root.AddCommand(newSync(func(context.Context, *cobra.Command, []string) error { t.Fatal("busy backup executed"); return nil }))
	root.SetArgs([]string{"sync", "--chat=demo", "--output=" + dir, "--skip-busy", "--" + flagResultFile + "=" + result})
	if err = root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	report, err := readCompletion(result)
	current, _ := os.ReadFile(filepath.Join(dir, "backup-report.json"))
	if err != nil || report.Status != "skipped" || report.Summary.Totals().Downloaded != 0 || !bytes.Equal(old, current) {
		t.Fatalf("skip reused or overwrote active results: %+v err=%v", report, err)
	}
}
