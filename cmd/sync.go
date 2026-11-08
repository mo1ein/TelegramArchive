package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mo1ein/TelegramArchive/internal/archive"
	"github.com/mo1ein/TelegramArchive/internal/schedule"
	transport "github.com/mo1ein/TelegramArchive/internal/telegram"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type backupComponent struct {
	Command string `json:"command"`
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
}

type backupReport struct {
	Started    time.Time         `json:"started_at"`
	Finished   time.Time         `json:"finished_at,omitempty"`
	Status     string            `json:"status"`
	Components []backupComponent `json:"components"`
}

type childRunner func(context.Context, *cobra.Command, []string) error

func NewSync() *cobra.Command { return newSync(runChild) }

func newSync(run childRunner) *cobra.Command {
	return newSyncPlan(run, nil)
}

// Cron setup uses the same validation/defaults without opening a session or
// starting the foreground scheduler.
func newSyncPlan(run childRunner, planned func(*cobra.Command, string, schedule.Options) error) *cobra.Command {
	f := &archiveFlags{}
	cmd := newArchive(f)
	cmd.Use = "sync"
	cmd.Short = "Sync selected Telegram data once or on a recurring interval"
	cmd.Long = "Back up selected Telegram content, merging new messages into existing archives.\nAccount data and Saved Messages stay in a separate directory.\nAdd --every to repeat; use --refresh to update older message edits too."
	cmd.Annotations = map[string]string{"offline": "true"}
	cmd.Flags().Lookup("chat").Usage = "chat display names, usernames, URLs or IDs; defaults to all matching chats"
	cmd.Flags().Lookup("takeout").Usage = "use bulk export sessions; account sections use takeout by default"
	cmd.Example = "  ta sync --everything --every 6h --format both\n  ta sync --all --types channels --media all --every 1h"
	var everything, account, skipBusy bool
	var chats bool
	var include []string
	var timing schedule.Options
	cmd.Flags().BoolVar(&everything, "everything", false, "all joined chats/groups/channels, account sections and media; explicit filters still apply")
	cmd.Flags().BoolVar(&account, "account", false, "also back up account data separately, including Saved Messages")
	cmd.Flags().BoolVar(&chats, "chats", true, "include chat/group/channel backups; false selects account data only")
	cmd.Flags().StringSliceVar(&include, "account-include", []string{"all"}, "account sections; supplying this also enables account backup")
	cmd.Flags().DurationVar(&timing.Every, "every", 0, "repeat interval, such as 1h or 6h; 0 runs once")
	cmd.Flags().IntVar(&timing.Runs, "runs", 0, "number of recurring cycles; 0 keeps running")
	cmd.Flags().DurationVar(&timing.Timeout, "job-timeout", 0, "maximum duration per entire backup cycle")
	cmd.Flags().BoolVar(&timing.StopOnError, "stop-on-error", false, "stop recurring backups after a failed cycle")
	cmd.Flags().BoolVar(&skipBusy, "skip-busy", false, "skip a busy output folder instead of waiting (automatic for recurring runs)")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if !cmd.Flags().Changed("all") && !cmd.Flags().Changed("chat") {
			cmd.Flags().Set("all", "true")
		}
		if everything {
			cmd.Flags().Set("all", "true")
			if !cmd.Flags().Changed("types") {
				cmd.Flags().Set("types", "personal,bots,groups,channels")
			}
			if !cmd.Flags().Changed("media") {
				cmd.Flags().Set("media", "all")
			}
		}
		account = account || everything || cmd.Flags().Changed("account-include")
		if !chats && !account {
			return fmt.Errorf("select chats or account data")
		}
		if !cmd.Flags().Changed("resume") {
			cmd.Flags().Set("resume", "true")
		}
		if timing.Every < 0 || timing.Runs < 0 || timing.Timeout < 0 || (timing.Every == 0 && timing.Runs > 0) {
			return fmt.Errorf("every/runs/job-timeout cannot be negative; --runs requires --every")
		}
		if timing.Every > 0 && !f.resume {
			return fmt.Errorf("recurring backups require --resume=true to sync existing archives")
		}
		if timing.Every > 0 && !cmd.Flags().Changed("skip-busy") {
			skipBusy = true
		}
		if len(include) != 1 || include[0] != "all" {
			if len(include) == 0 {
				return fmt.Errorf("select at least one account section")
			}
			if err := transport.ValidateAccountKinds(include); err != nil {
				return err
			}
		}
		f.selectMedia(cmd)
		if err := validateBackupFlags(f, chats); err != nil {
			return err
		}
		if chats && f.all && len(f.chats) > 0 {
			return fmt.Errorf("use either --all/--everything or --chat")
		}
		if strings.TrimSpace(f.output) == "" {
			return fmt.Errorf("output directory cannot be empty")
		}
		dir, err := filepath.Abs(f.output)
		if err != nil {
			return err
		}
		cmd.Flags().Set("output", dir)
		var commands [][]string
		for _, leaf := range []*cobra.Command{NewArchive(), NewAccount()} {
			if leaf.Name() == "archive" && !chats || leaf.Name() == "account" && !account {
				continue
			}
			args := append(inheritedArguments(cmd), leaf.Name())
			cmd.Flags().Visit(func(flag *pflag.Flag) {
				if leaf.Flags().Lookup(flag.Name) != nil {
					args = append(args, flagArgument(flag)...)
				}
			})
			if leaf.Name() == "account" {
				args = append(args, "--include="+csvValues(include))
			}
			commands = append(commands, args)
		}
		if planned != nil {
			return planned(cmd, dir, timing)
		}
		if timing.Every == 0 {
			timing.Every, timing.Runs = time.Second, 1
		}
		setScheduleLogging(cmd, &timing)
		return schedule.Run(cmd.Context(), timing, func(ctx context.Context) error {
			return runBackupCycle(ctx, cmd, dir, commands, run, skipBusy)
		})
	}
	return cmd
}

func runBackupCycle(ctx context.Context, cmd *cobra.Command, dir string, commands [][]string, run childRunner, skipBusy bool) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	lock, err := lockBackupOutput(ctx, dir, skipBusy, cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	if lock == nil {
		return nil
	}
	defer lock.Close()
	report := backupReport{Started: time.Now().UTC(), Status: "running"}
	path := filepath.Join(dir, "backup-report.json")
	if err := archive.WriteData(path, report); err != nil {
		return err
	}
	var failures []error
	for _, args := range commands {
		if ctx.Err() != nil {
			failures = append(failures, ctx.Err())
			break
		}
		name := "archive"
		for _, arg := range args {
			if arg == "account" {
				name = "account"
			}
		}
		report.Components = append(report.Components, backupComponent{Command: name, Status: "running"})
		if err := archive.WriteData(path, report); err != nil {
			return err
		}
		err := run(ctx, cmd, args)
		if ctx.Err() != nil {
			err = errors.Join(err, ctx.Err())
		}
		component := &report.Components[len(report.Components)-1]
		component.Status = "complete"
		if err != nil {
			component.Status, component.Error = "failed", err.Error()
			failures = append(failures, fmt.Errorf("%s: %w", name, err))
		}
		if err := archive.WriteData(path, report); err != nil {
			return err
		}
	}
	report.Status = "complete"
	if len(failures) > 0 {
		report.Status = "failed"
	}
	report.Finished = time.Now().UTC()
	return errors.Join(errors.Join(failures...), archive.WriteData(path, report))
}

func validateBackupFlags(f *archiveFlags, chats bool) error {
	since, err := parseDate(f.since)
	if err != nil {
		return fmt.Errorf("since: %w", err)
	}
	until, err := parseDate(f.until)
	if err != nil {
		return fmt.Errorf("until: %w", err)
	}
	media := f.media
	if len(media) == 1 && media[0] == "all" {
		media = archive.MediaKinds
	} else if len(media) == 1 && media[0] == "none" {
		media = nil
	}
	o := archive.Options{Concurrency: 1, Media: media, Format: f.format, MinFileSize: f.minFile, MaxFileSize: f.maxFile, Extensions: f.extensions, MIMETypes: f.mimes, PartSize: f.partSize, HTMLPageSize: f.pageSize, Since: since, Until: until}
	if err := o.Validate(); err != nil {
		return err
	}
	if chats {
		if f.includeLeft && (!f.all || !f.takeout) {
			return fmt.Errorf("--include-left requires --all and --takeout")
		}
		return transport.ValidateTypes(f.types)
	}
	return nil
}
