package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCombinedBackupContinuesAfterFailureAndKeepsAccountSeparate(t *testing.T) {
	dir := t.TempDir()
	var calls [][]string
	c := newSync(func(ctx context.Context, cmd *cobra.Command, args []string) error {
		calls = append(calls, slices.Clone(args))
		if slices.Contains(args, "archive") {
			return errors.New("one channel failed")
		}
		return nil
	})
	c.SetArgs([]string{"--everything", "--output", dir, "--format=both", "--extensions=pdf,mp4"})
	if err := c.ExecuteContext(context.Background()); err == nil {
		t.Fatal("partial failure was hidden")
	}
	if len(calls) != 2 {
		t.Fatalf("account backup did not run after chat failure: %v", calls)
	}
	for i, name := range []string{"archive", "account"} {
		args := calls[i]
		if !slices.Contains(args, name) || !slices.Contains(args, "--format=both") || !slices.Contains(args, "--resume=true") || !slices.Contains(args, "--output="+dir) {
			t.Fatalf("incorrect %s invocation: %v", name, args)
		}
	}
	if !slices.Contains(calls[0], "--all=true") || slices.Contains(calls[1], "--all=true") || !slices.Contains(calls[1], "--include=all") {
		t.Fatalf("chat and account selection mixed: %v", calls)
	}
	b, err := os.ReadFile(filepath.Join(dir, "backup-report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Status     string
		Components []struct{ Command, Status, Error string }
	}
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "failed" || len(report.Components) != 2 || report.Components[0].Error == "" || report.Components[1].Status != "complete" {
		t.Fatalf("incorrect report: %s", b)
	}
}

func TestRecurringBackupDefaultsToIncrementalAndDoesNotOverlap(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ENABLE_RESUME", "false")
	var calls int
	c := newSync(func(ctx context.Context, cmd *cobra.Command, args []string) error {
		calls++
		if !slices.Contains(args, "--resume=true") || slices.Contains(args, "--refresh=true") {
			t.Fatalf("default recurring job is not incremental: %v", args)
		}
		other := newSync(func(context.Context, *cobra.Command, []string) error {
			t.Fatal("overlapping backup was started")
			return nil
		})
		other.SetArgs([]string{"--output", dir})
		if err := other.ExecuteContext(ctx); err == nil || !strings.Contains(err.Error(), "another backup") {
			t.Fatalf("missing cross-process lock: %v", err)
		}
		return nil
	})
	c.SetArgs([]string{"--output", dir, "--every=1ms", "--runs=2"})
	if err := c.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected two incremental cycles, got %d", calls)
	}
}

func TestBackupValidatesBeforeStartingAndPreservesInheritedFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--every=-1s"}, {"--every=1s", "--resume=false"}, {"--account-include=typo"}, {"--everything", "--chat=someone"},
		{"--format=bad"}, {"--extensions=../pdf"}, {"--min-file-size=-1"}, {"--types=typo"}, {"--since=bad"},
	} {
		c := newSync(func(context.Context, *cobra.Command, []string) error {
			t.Fatal("invalid options started a backup")
			return nil
		})
		c.SetArgs(args)
		if err := c.ExecuteContext(context.Background()); err == nil {
			t.Fatalf("accepted invalid options: %v", args)
		}
	}
	root := &cobra.Command{Use: "ta"}
	root.PersistentFlags().String("ns", "default", "")
	root.PersistentFlags().StringToString("storage", nil, "")
	root.AddCommand(newSync(func(ctx context.Context, cmd *cobra.Command, args []string) error {
		if !slices.Contains(args, "--ns=personal") || !slices.Contains(args, "--storage=path=/tmp/a,type=bolt") {
			t.Fatalf("session flags lost: %v", args)
		}
		return nil
	}))
	root.SetArgs([]string{"--ns=personal", "--storage=path=/tmp/a,type=bolt", "sync", "--output", t.TempDir()})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAccountOnlyBackupIgnoresChatConfiguration(t *testing.T) {
	t.Setenv("CHAT_IDS", "invalid JSON")
	t.Setenv("CHAT_EXPORT_GROUPS", "invalid boolean")
	calls := 0
	c := newSync(func(ctx context.Context, cmd *cobra.Command, args []string) error {
		calls++
		if !slices.Contains(args, "account") || slices.Contains(args, "archive") || !slices.Contains(args, "--include=contacts,saved_messages") {
			t.Fatalf("incorrect account-only scope: %v", args)
		}
		return nil
	})
	c.SetArgs([]string{"--chats=false", "--account-include=contacts,saved_messages", "--output", t.TempDir()})
	if err := c.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal(calls)
	}
}
