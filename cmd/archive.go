package cmd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/iyear/tdl/core/storage"
	"github.com/iyear/tdl/pkg/consts"
	transferprog "github.com/iyear/tdl/pkg/prog"
	"github.com/iyear/tdl/pkg/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"ta/internal/archive"
	transport "ta/internal/telegram"
)

type archiveFlags struct {
	refresh                      bool
	includeLeft, includeMigrated bool
	chats, types, media          []string
	all, takeout, resume         bool
	output, since, until         string
	exportName                   string
	format                       string
	extensions, mimes            []string
	minFile                      int64
	onlyMedia                    bool
	pageSize                     int
	maxFile, partSize            int64
}

func NewArchive() *cobra.Command {
	return newArchive(&archiveFlags{})
}

func (f *archiveFlags) selectMedia(cmd *cobra.Command) {
	if !cmd.Flags().Changed("media") && (cmd.Flags().Changed("extensions") || cmd.Flags().Changed("mime")) {
		f.media = archive.MediaKinds
	}
}

func newArchive(f *archiveFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "archive", Short: "Back up chats, groups and channels as JSON or offline HTML", Args: cobra.NoArgs}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		f.selectMedia(cmd)
		since, err := parseDate(f.since)
		if err != nil {
			return fmt.Errorf("since: %w", err)
		}
		until, err := parseDate(f.until)
		if err != nil {
			return fmt.Errorf("until: %w", err)
		}
		if len(f.media) == 1 && f.media[0] == "all" {
			f.media = archive.MediaKinds
		}
		if len(f.media) == 1 && f.media[0] == "none" {
			f.media = nil
		}
		opts := archive.Options{Media: f.media, Concurrency: viper.GetInt(consts.FlagLimit), MaxFileSize: f.maxFile, PartSize: f.partSize, Since: since, Until: until, Resume: f.resume, Refresh: f.refresh, Takeout: f.takeout, Format: f.format, ExportName: f.exportName, Extensions: f.extensions, MIMETypes: f.mimes, MinFileSize: f.minFile, OnlyMedia: f.onlyMedia, HTMLPageSize: f.pageSize}
		if err := opts.Validate(); err != nil {
			return err
		}
		if err := transport.ValidateTypes(f.types); err != nil {
			return err
		}
		if strings.TrimSpace(f.output) == "" {
			return fmt.Errorf("output directory cannot be empty")
		}
		threads, pool := viper.GetInt(consts.FlagThreads), viper.GetInt(consts.FlagPoolSize)
		if threads < 1 || threads > 64 || pool < 1 || pool > 64 {
			return fmt.Errorf("threads and pool must be between 1 and 64")
		}
		if f.all && len(f.chats) > 0 {
			return fmt.Errorf("use either --all or --chat")
		}
		if f.includeLeft && (!f.all || !f.takeout) {
			return fmt.Errorf("--include-left requires --all and --takeout")
		}
		if len(f.chats) == 0 && !f.all {
			f.chats = []string{"me"}
		}
		return tRunArchive(cmd.Context(), f.resume, cmd.ErrOrStderr(), func(ctx context.Context, c *telegram.Client, kv storage.Storage) (rerr error) {
			var err error
			source := transport.New(ctx, c, kv, pool, threads, viper.GetDuration(consts.FlagReconnectTimeout), since, until)
			defer func() { rerr = errors.Join(rerr, source.Close(rerr == nil)) }()
			source.DownloadCustomEmoji = slices.Contains(f.media, "stickers")
			if err := source.Identity(ctx); err != nil {
				return err
			}
			var chats []archive.Chat
			if f.all {
				if f.takeout {
					if err := source.EnableTakeout(ctx, f.types, f.media, f.maxFile); err != nil {
						return err
					}
				}
				chats, err = source.Dialogs(ctx, f.types)
				if err != nil {
					source.Warnings = append(source.Warnings, err.Error())
				}
				if len(chats) == 0 && err != nil {
					return err
				}
			} else {
				for _, ref := range f.chats {
					chat, err := source.Resolve(ctx, ref)
					if err != nil {
						return err
					}
					chats = append(chats, chat)
				}
			}
			if f.exportName != "" {
				if f.all || len(chats) != 1 {
					return fmt.Errorf("--export-name requires exactly one selected chat")
				}
				chats[0].Name = f.exportName
			}
			if f.includeLeft {
				left, e := source.LeftDialogs(ctx, f.types)
				chats = append(chats, left...)
				if e != nil {
					source.Warnings = append(source.Warnings, e.Error())
					err = errors.Join(err, e)
				}
			}
			if f.includeMigrated {
				expanded, e := source.MigratedGroups(ctx, chats)
				chats = expanded
				if e != nil {
					source.Warnings = append(source.Warnings, e.Error())
					err = errors.Join(err, e)
				}
			}
			if len(chats) == 0 {
				return fmt.Errorf("no chats match the selection")
			}
			if f.takeout && !f.all {
				var types []string
				for _, chat := range chats {
					switch chat.Type {
					case "personal_chat", "bot_chat", "saved_messages":
						types = append(types, "personal")
					case "private_group", "private_supergroup", "public_supergroup":
						types = append(types, "groups")
					default:
						types = append(types, "channels")
					}
				}
				if err := source.EnableTakeout(ctx, types, f.media, f.maxFile); err != nil {
					return err
				}
			}
			for _, warning := range source.Warnings {
				fmt.Fprintln(cmd.ErrOrStderr(), "Discovery:", warning)
			}
			progress := transferprog.New(utils.Byte.FormatBinaryBytes)
			progress.SetOutputWriter(cmd.ErrOrStderr())
			progress.SetNumTrackersExpected(1)
			tracker := transferprog.AppendTracker(progress, utils.Byte.FormatBinaryBytes, "Telegram archive", 0)
			tracker.Start()
			var messages, files, active atomic.Int64
			var expectedMu sync.Mutex
			var expectedBytes int64
			updateMessage := func() {
				tracker.UpdateMessage(fmt.Sprintf("Archive · %d messages · %d files · %d active", messages.Load(), files.Load(), active.Load()))
			}
			opts.Progress = func(n int) {
				messages.Add(int64(n))
				updateMessage()
			}
			opts.OnTransfer = func(event archive.TransferEvent) {
				switch event.Kind {
				case archive.TransferStarted:
					files.Add(1)
					active.Add(1)
					if event.File.Size > 0 {
						expectedMu.Lock()
						expectedBytes += event.File.Size
						tracker.UpdateTotal(expectedBytes + 1)
						expectedMu.Unlock()
					}
					updateMessage()
				case archive.TransferProgressed:
					tracker.Increment(event.Delta)
				case archive.TransferFinished:
					active.Add(-1)
					updateMessage()
				}
			}
			go progress.Render()
			for !progress.IsRenderInProgress() {
				time.Sleep(time.Millisecond)
			}
			report, batchErr := archive.RunBatch(ctx, source, f.output, chats, opts, source.Warnings)
			if len(report.Warnings) != len(source.Warnings) {
				report.Warnings = source.Warnings
				batchErr = errors.Join(batchErr, archive.WriteData(filepath.Join(f.output, "run-report.json"), report))
				for _, warning := range source.Warnings {
					fmt.Fprintln(cmd.ErrOrStderr(), "Export:", warning)
				}
			}
			resultErr := errors.Join(batchErr, err)
			if resultErr != nil {
				tracker.MarkAsErrored()
			} else {
				tracker.UpdateTotal(tracker.Value())
				tracker.MarkAsDone()
			}
			transferprog.Wait(context.Background(), progress)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			for _, entry := range report.Chats {
				if entry.Error != "" {
					fmt.Fprintf(cmd.ErrOrStderr(), "%s: failed: %s\n", entry.Chat.Name, entry.Error)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "%s: %d messages, %d output file(s)\n", entry.Chat.Name, entry.Stats.Messages, len(entry.Paths))
				}
			}
			return resultErr
		})
	}
	flags := cmd.Flags()
	flags.BoolVar(&f.refresh, "refresh", false, "refresh existing messages and reactions while retaining archived deletions")
	flags.BoolVar(&f.includeLeft, "include-left", false, "also export accessible history of left channels (requires --all --takeout)")
	flags.BoolVar(&f.includeMigrated, "include-migrated", true, "also export original basic-group history for migrated supergroups")
	flags.StringVar(&f.format, "format", "json", "output format: json, html or both")
	flags.IntVar(&f.pageSize, "html-page-size", 1000, "maximum messages per HTML page")
	flags.StringSliceVar(&f.extensions, "extensions", nil, "file extensions to download, such as pdf,zip,mp4")
	flags.StringSliceVar(&f.mimes, "mime", nil, "MIME filters, such as application/pdf or video/*")
	flags.Int64Var(&f.minFile, "min-file-size", 0, "minimum media file size in bytes (inclusive)")
	flags.BoolVar(&f.onlyMedia, "only-media", false, "include only messages with a file matching all download filters")
	flags.StringSliceVarP(&f.chats, "chat", "c", nil, "chat usernames, joined-dialog names, URLs or IDs (repeat or comma-separate; defaults to me)")
	flags.StringVar(&f.exportName, "export-name", "", "custom display name and folder label for one selected chat (defaults to its username for new folders)")
	flags.BoolVar(&f.all, "all", false, "archive account dialogs selected by --types")
	flags.StringSliceVar(&f.types, "types", []string{"personal", "bots", "groups", "channels"}, "chat types: "+strings.Join(transport.ChatTypes, ", "))
	flags.StringSliceVar(&f.media, "media", []string{"photos"}, "media types: all, none, or "+strings.Join(archive.MediaKinds, ", "))
	flags.StringVarP(&f.output, "output", "o", "data", "archive root directory")
	flags.StringVar(&f.since, "since", "", "inclusive date (YYYY-MM-DD or RFC3339)")
	flags.StringVar(&f.until, "until", "", "exclusive date (YYYY-MM-DD or RFC3339)")
	flags.Int64Var(&f.maxFile, "max-file-size", 0, "maximum media file size in bytes (0 = unlimited)")
	flags.Int64Var(&f.partSize, "part-size", 0, "maximum JSON part size in bytes (0 = one result.json)")
	flags.BoolVar(&f.resume, "resume", true, "resume checkpoints and append new messages; false rebuilds history")
	flags.BoolVar(&f.takeout, "takeout", false, "use Telegram's bulk export session (may require approval/delay)")
	return cmd
}

func parseDate(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t, nil
		}
	}
	if ts, err := strconv.ParseInt(value, 10, 64); err == nil {
		return time.Unix(ts, 0).UTC(), nil
	}
	return time.Time{}, fmt.Errorf("invalid date %q; use YYYY-MM-DD, RFC3339 or Unix seconds", value)
}
