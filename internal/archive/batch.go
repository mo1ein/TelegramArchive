package archive

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type ChatExport struct {
	Chat  Chat     `json:"chat"`
	Paths []string `json:"files,omitempty"`
	Stats Stats    `json:"statistics"`
	Error string   `json:"error,omitempty"`
	Link  string   `json:"-"`
}

type RunReport struct {
	Started  time.Time    `json:"started_at"`
	Finished time.Time    `json:"finished_at"`
	Chats    []ChatExport `json:"chats"`
	Warnings []string     `json:"warnings,omitempty"`
}

// RunBatch finishes independent chats after a failure, then returns all errors.
func RunBatch(ctx context.Context, source Source, root string, chats []Chat, opts Options, warnings []string) (RunReport, error) {
	report := RunReport{Started: time.Now().UTC(), Warnings: warnings}
	var failures []error
	if err := opts.Validate(); err != nil {
		return report, err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return report, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return report, err
	}
	// Read the root once; binary search keeps lookup cheap for large chat lists.
	var directories []string
	for _, entry := range entries {
		if entry.IsDir() {
			directories = append(directories, entry.Name())
		}
	}
	var plans map[string]plannedChat
	if opts.OnPlan != nil {
		plans, err = planBatch(ctx, source, root, directories, chats, opts)
		if err != nil {
			return report, err
		}
	}
	seen := map[string]bool{}
	for _, chat := range chats {
		if err := ctx.Err(); err != nil {
			failures = append(failures, err)
			break
		}
		if chat.Key == "" {
			chat.Key = fmt.Sprintf("chat_%d", chat.ID)
		}
		if seen[chat.Key] {
			continue
		}
		seen[chat.Key] = true
		if !filepath.IsLocal(chat.Key) {
			failures = append(failures, fmt.Errorf("unsafe chat key"))
			continue
		}
		record := ChatExport{Chat: chat}
		name, err := exportDirectory(chat, opts.ExportName, directories)
		if err != nil {
			record.Error = err.Error()
			report.Chats = append(report.Chats, record)
			failures = append(failures, fmt.Errorf("%s: %w", chat.Name, err))
			continue
		}
		dir := filepath.Join(root, name)
		chatOpts := opts
		var result Result
		if plan, exists := plans[chat.Key]; exists {
			chatOpts.snapshotNewest = &plan.newest
			err = plan.err
		}
		if err == nil {
			result, err = Run(ctx, source, dir, chat, chatOpts)
		}
		if err != nil {
			record.Error = err.Error()
			failures = append(failures, fmt.Errorf("%s: %w", chat.Name, err))
		} else {
			record.Stats = result.Stats
			for _, p := range result.Paths {
				rel, e := filepath.Rel(root, p)
				if e != nil {
					return report, e
				}
				record.Paths = append(record.Paths, filepath.ToSlash(rel))
				if filepath.Ext(p) == ".html" && record.Link == "" {
					record.Link = localMedia(filepath.ToSlash(rel))
				}
			}
		}
		report.Chats = append(report.Chats, record)
		// Recreate a stalled connection before trying the remaining chats.
		// Permanent per-chat failures still leave other chats independent.
		var timeout net.Error
		if opts.Resume && errors.As(err, &timeout) && timeout.Timeout() {
			break
		}
	}
	report.Finished = time.Now().UTC()
	if err := writeJSON(filepath.Join(root, "run-report.json"), report); err != nil {
		failures = append(failures, err)
	}
	if opts.Format == "html" || opts.Format == "both" {
		if err := writeHTMLIndex(root, report); err != nil {
			failures = append(failures, err)
		}
	}
	return report, errors.Join(failures...)
}

func exportDirectory(chat Chat, custom string, directories []string) (string, error) {
	base := "ChatExport_" + chat.Key
	label := strings.TrimPrefix(strings.TrimSpace(chat.Username), "@")
	if custom != "" {
		label = custom
	}
	label = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`/\\<>:"|?*`, r) {
			return '_'
		}
		return r
	}, label)
	for len(label) > 120 {
		_, size := utf8.DecodeLastRuneInString(label)
		label = label[:len(label)-size]
	}
	label = strings.Trim(label, ". ")
	name := base
	if label != "" {
		name += "_" + label
	}
	if _, exists := slices.BinarySearch(directories, name); exists {
		return name, nil
	}
	if _, exists := slices.BinarySearch(directories, base); exists {
		return base, nil // Resume older exports without a username suffix.
	}
	prefix := base + "_"
	i, _ := slices.BinarySearch(directories, prefix)
	if i < len(directories) && strings.HasPrefix(directories[i], prefix) {
		if i+1 < len(directories) && strings.HasPrefix(directories[i+1], prefix) {
			return "", fmt.Errorf("multiple existing export folders for %s; use a separate --output directory", chat.Key)
		}
		return directories[i], nil // A changed username must not restart the backup.
	}
	return name, nil
}
