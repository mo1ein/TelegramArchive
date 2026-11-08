package cmd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/gotd/td/telegram"
	"github.com/iyear/tdl/core/storage"
	"github.com/iyear/tdl/pkg/consts"
	"github.com/mo1ein/TelegramArchive/internal/archive"
	transport "github.com/mo1ein/TelegramArchive/internal/telegram"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewAccount() *cobra.Command {
	f := archiveFlags{}
	var include []string
	cmd := &cobra.Command{Use: "account", Short: "Back up account data and Saved Messages separately from chats", Args: cobra.NoArgs}
	flags := cmd.Flags()
	flags.BoolVar(&f.refresh, "refresh", false, "refresh existing messages and reactions while retaining archived deletions")
	flags.StringSliceVar(&include, "include", []string{"all"}, "account sections: all, or "+strings.Join(transport.AccountKinds, ", "))
	flags.StringVarP(&f.output, "output", "o", "data", "backup root; writes AccountExport_<account ID>")
	flags.StringVar(&f.format, "format", "json", "json, html or both")
	flags.StringSliceVar(&f.media, "media", []string{"all"}, "media categories for photos, stories and Saved Messages")
	flags.StringSliceVar(&f.extensions, "extensions", nil, "file extensions to download, such as pdf,zip,mp4")
	flags.StringSliceVar(&f.mimes, "mime", nil, "MIME filters, such as video/*")
	flags.Int64Var(&f.minFile, "min-file-size", 0, "minimum media size in bytes")
	flags.Int64Var(&f.maxFile, "max-file-size", 0, "maximum media size in bytes (0 = unlimited locally)")
	flags.StringVar(&f.since, "since", "", "inclusive media/message date")
	flags.StringVar(&f.until, "until", "", "exclusive media/message date")
	flags.Int64Var(&f.partSize, "part-size", 0, "maximum JSON part size in bytes")
	flags.IntVar(&f.pageSize, "html-page-size", 1000, "maximum messages/assets per HTML page")
	flags.BoolVar(&f.resume, "resume", true, "resume Saved Messages and reuse completed media")
	flags.BoolVar(&f.takeout, "takeout", true, "use Telegram's official export session; may require approval")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		f.selectMedia(cmd)
		if len(include) == 1 && include[0] == "all" {
			include = transport.AccountKinds
		}
		if err := transport.ValidateAccountKinds(include); err != nil {
			return err
		}
		if len(include) == 0 {
			return fmt.Errorf("select at least one account section")
		}
		since, err := parseDate(f.since)
		if err != nil {
			return err
		}
		until, err := parseDate(f.until)
		if err != nil {
			return err
		}
		if len(f.media) == 1 && f.media[0] == "all" {
			f.media = archive.MediaKinds
		}
		if len(f.media) == 1 && f.media[0] == "none" {
			f.media = nil
		}
		opts := archive.Options{Media: f.media, Concurrency: viper.GetInt(consts.FlagLimit), MaxFileSize: f.maxFile, MinFileSize: f.minFile, Extensions: f.extensions, MIMETypes: f.mimes, Format: f.format, PartSize: f.partSize, HTMLPageSize: f.pageSize, Since: since, Until: until, Resume: f.resume, Refresh: f.refresh, Takeout: f.takeout}
		if err := opts.Validate(); err != nil {
			return err
		}
		if strings.TrimSpace(f.output) == "" {
			return fmt.Errorf("output cannot be empty")
		}
		threads, pool := viper.GetInt(consts.FlagThreads), viper.GetInt(consts.FlagPoolSize)
		if threads < 1 || threads > 64 || pool < 1 || pool > 64 {
			return fmt.Errorf("threads and pool must be between 1 and 64")
		}
		return tRun(cmd.Context(), func(ctx context.Context, c *telegram.Client, kv storage.Storage) (rerr error) {
			source := transport.New(ctx, c, kv, pool, threads, viper.GetDuration(consts.FlagReconnectTimeout), since, until)
			defer func() { rerr = errors.Join(rerr, source.Close(rerr == nil)) }()
			source.DownloadCustomEmoji = slices.Contains(f.media, "stickers")
			if err := source.Identity(ctx); err != nil {
				return err
			}
			if f.takeout {
				if err := source.EnableAccountTakeout(ctx, include, opts); err != nil {
					return err
				}
			}
			dir := filepath.Join(f.output, "AccountExport_"+strconv.FormatInt(source.AccountID(), 10))
			report, err := source.ExportAccount(ctx, dir, include, opts)
			for _, section := range report.Sections {
				if section.Error != "" {
					fmt.Fprintf(cmd.ErrOrStderr(), "%s: failed: %s\n", section.Name, section.Error)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "%s: backed up\n", section.Name)
				}
			}
			fmt.Fprintln(cmd.OutOrStdout(), dir)
			return err
		})
	}
	return cmd
}
