package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/mo1ein/TelegramArchive/internal/schedule"
	"github.com/spf13/cobra"
)

func NewSchedule() *cobra.Command {
	var opts schedule.Options
	var target string
	cmd := &cobra.Command{Use: "schedule --every 6h -- [archive flags]", Short: "Run recurring backups, refreshing discovery each run", Annotations: map[string]string{"offline": "true"}, Args: cobra.MinimumNArgs(1)}
	cmd.Flags().DurationVar(&opts.Every, "every", 0, "backup interval, such as 30m, 6h or 24h (required)")
	cmd.Flags().StringVar(&target, "target", "archive", "backup command: archive, account or sync")
	cmd.Flags().IntVar(&opts.Runs, "runs", 0, "number of runs (0 keeps running until stopped)")
	cmd.Flags().DurationVar(&opts.Timeout, "job-timeout", 0, "maximum duration per backup job (0 = unlimited)")
	cmd.Flags().BoolVar(&opts.StopOnError, "stop-on-error", false, "stop scheduling when a backup fails")
	cmd.MarkFlagRequired("every")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if target != "archive" && target != "account" && target != "sync" {
			return fmt.Errorf("target must be archive, account or sync")
		}
		if opts.Every <= 0 || opts.Runs < 0 || opts.Timeout < 0 {
			return fmt.Errorf("every must be positive; runs and job-timeout cannot be negative")
		}
		childArgs := append(inheritedArguments(cmd), target)
		childArgs = append(childArgs, args...)
		setScheduleLogging(cmd, &opts)
		return schedule.Run(cmd.Context(), opts, func(ctx context.Context) error {
			return runChild(ctx, cmd, childArgs)
		})
	}
	return cmd
}

func setScheduleLogging(cmd *cobra.Command, opts *schedule.Options) {
	opts.OnError = func(err error) { fmt.Fprintf(cmd.ErrOrStderr(), "Backup failed; checkpoints retained: %v\n", err) }
	opts.OnStart = func(n int, t time.Time) {
		fmt.Fprintf(cmd.ErrOrStderr(), "Backup run %d starting at %s\n", n, t.UTC().Format(time.RFC3339))
	}
	opts.OnNext = func(t time.Time) { fmt.Fprintf(cmd.ErrOrStderr(), "Next backup: %s\n", t.UTC().Format(time.RFC3339)) }
}
