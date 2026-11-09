package archive

import (
	"bytes"
	"fmt"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"
)

func SummaryFilters(opts Options) []string {
	media := strings.Join(opts.Media, ",")
	all := true
	for _, kind := range MediaKinds {
		all = all && slices.Contains(opts.Media, kind)
	}
	if all {
		media = "all"
	}
	if media == "" {
		media = "none"
	}
	filters := []string{"media=" + media}
	if len(opts.Extensions) > 0 {
		filters = append(filters, "extensions="+strings.Join(opts.Extensions, ","))
	}
	if len(opts.MIMETypes) > 0 {
		filters = append(filters, "mime="+strings.Join(opts.MIMETypes, ","))
	}
	if opts.MinFileSize > 0 {
		filters = append(filters, fmt.Sprintf("min-size=%d B", opts.MinFileSize))
	}
	if opts.MaxFileSize > 0 {
		filters = append(filters, fmt.Sprintf("max-size=%d B", opts.MaxFileSize))
	}
	if !opts.Since.IsZero() {
		filters = append(filters, "since="+opts.Since.Format(time.RFC3339))
	}
	if !opts.Until.IsZero() {
		filters = append(filters, "until="+opts.Until.Format(time.RFC3339))
	}
	if opts.OnlyMedia {
		filters = append(filters, "matching-media messages only")
	}
	return filters
}

func CompletionLabel(status string) string {
	label := map[string]string{"complete": "completed", "complete_with_warnings": "completed with warnings", "failed": "failed", "canceled": "canceled", "skipped": "skipped", "unknown": "result unavailable"}[status]
	if label == "" {
		return status
	}
	return label
}

func SummaryStatus(err error, s Summary, warnings []string) string {
	if err != nil {
		return "failed"
	}
	if len(warnings) > 0 || s.StoredUnavailableThumbnails > 0 || s.Totals().Unavailable > 0 || s.Totals().Failed > 0 || s.Totals().Canceled > 0 || s.Error != "" {
		return "complete_with_warnings"
	}
	return "complete"
}

func SummaryText(title, status string, s Summary, finished time.Time, elapsed time.Duration) string {
	var out bytes.Buffer
	clean := func(value string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return -1
			}
			return r
		}, value)
	}
	label := CompletionLabel(status)
	fmt.Fprintf(&out, "\nResults: %s\n", clean(strings.TrimPrefix(title, "Finished ")))
	finish := func() string {
		fmt.Fprintf(&out, "%s | %s", clean(title), clean(label))
		if status != "skipped" && s.MediaKnown {
			total := s.Totals()
			fmt.Fprintf(&out, " | %d downloaded, %d reused, %d filtered, %d failed, %d unavailable", total.Downloaded, total.Reused, total.Filtered, total.Failed, total.Unavailable)
			if total.Canceled > 0 {
				fmt.Fprintf(&out, ", %d canceled", total.Canceled)
			}
			if s.StoredUnavailableThumbnails > 0 {
				fmt.Fprintf(&out, " | %d thumbnail(s) unavailable in archive", s.StoredUnavailableThumbnails)
			}
		}
		fmt.Fprintf(&out, " | %s | Elapsed %s\n", finished.Local().Format(time.RFC3339), max(time.Duration(0), elapsed).Round(time.Second))
		return out.String()
	}
	if status == "skipped" {
		out.WriteString("No backup work was performed.\n")
		return finish()
	}
	if len(s.Filters) > 0 {
		fmt.Fprintln(&out, "Filters:", clean(strings.Join(s.Filters, "; ")))
	}
	if s.StoredUnavailableThumbnails > 0 {
		fmt.Fprintf(&out, "Archive warning: %d unavailable thumbnail(s) retained; use --refresh to retry.\n", s.StoredUnavailableThumbnails)
	}
	if s.HasMessages {
		if s.MessagesKnown {
			fmt.Fprintf(&out, "Messages archived: %d total | %d added this run\n", s.Messages, s.MessagesAdded)
		} else {
			fmt.Fprintln(&out, "Messages archived: unavailable (archive did not finish)")
		}
	}
	if !s.MediaKnown {
		if s.Error != "" {
			fmt.Fprintln(&out, "Media summary unavailable:", clean(s.Error))
		} else {
			fmt.Fprintln(&out, "Media summary unavailable.")
		}
		return finish()
	}
	if len(s.Media) == 0 {
		fmt.Fprintln(&out, "Media this run: no files processed (0 downloaded)")
		return finish()
	}
	fmt.Fprintln(&out, "Media this run (unique files):")
	table := tabwriter.NewWriter(&out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "Type\tDownloaded\tReused\tFiltered\tFailed\tUnavailable\tCanceled")
	labels := map[string]string{"photos": "Photos", "videos": "Videos", "audios": "Audio files", "documents": "Documents", "voice_messages": "Voice messages", "video_messages": "Round videos", "stickers": "Stickers", "animations": "Animations", "contacts": "Contacts", "thumbnails": "Thumbnails", "custom_emoji": "Custom emoji", "files": "Files", "account_data": "Account data"}
	keys := make([]string, 0, len(s.Media))
	for kind := range s.Media {
		keys = append(keys, kind)
	}
	sort.Strings(keys)
	row := func(label string, c MediaCounts) {
		fmt.Fprintf(table, "%s\t%d\t%d\t%d\t%d\t%d\t%d\n", label, c.Downloaded, c.Reused, c.Filtered, c.Failed, c.Unavailable, c.Canceled)
	}
	for _, kind := range keys {
		label := labels[kind]
		if label == "" {
			label = kind
		}
		row(clean(label), s.Media[kind])
	}
	row("Total", s.Totals())
	table.Flush()
	fmt.Fprintf(&out, "Downloaded this run: %s\n", fileSize(s.DownloadedBytes))
	return finish()
}
