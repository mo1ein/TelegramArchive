package cmd

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/mattn/go-runewidth"
	"github.com/mo1ein/TelegramArchive/internal/archive"
	"golang.org/x/term"
)

type activeTransfer struct {
	size, received int64
	order          uint64
}

type progressMessage struct {
	text string
	done chan struct{}
}

type backupProgress struct {
	mu                       sync.Mutex
	started, transferStarted time.Time
	plan                     archive.DownloadPlan
	active                   map[string]activeTransfer
	order                    uint64
	bytes, completeBytes     int64
	completeFiles            int64
	unavailableThumbnails    int64
	finished                 bool
	colorful                 bool
	failure                  error
	stop                     chan error
	stopped, refresh         chan struct{}
	messages                 chan progressMessage
}

func newBackupProgress(output io.Writer) *backupProgress {
	p := &backupProgress{started: time.Now(), active: map[string]activeTransfer{}, stop: make(chan error), stopped: make(chan struct{}), refresh: make(chan struct{}, 1), messages: make(chan progressMessage)}
	fd, terminal := 0, false
	if file, ok := output.(interface{ Fd() uintptr }); ok {
		fd = int(file.Fd())
		terminal = term.IsTerminal(fd)
	}
	p.colorful = terminal && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
	go func() {
		defer close(p.stopped)
		interval := 10 * time.Second
		if terminal {
			interval = 100 * time.Millisecond
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		drawn := false
		render := func() {
			width := 80
			if terminal {
				if columns, _, err := term.GetSize(fd); err == nil {
					width = columns
				}
			}
			lines := p.lines(time.Now(), width)
			if terminal {
				if drawn {
					fmt.Fprint(output, "\x1b[3A")
				}
				for _, line := range lines {
					fmt.Fprintf(output, "\r\x1b[2K%s\n", line)
				}
			} else {
				fmt.Fprintln(output, strings.Join(lines, " | "))
			}
			drawn = true
		}
		render()
		for {
			select {
			case message := <-p.messages:
				if terminal && drawn {
					fmt.Fprint(output, "\x1b[3A\x1b[J")
				}
				fmt.Fprint(output, message.text)
				drawn = false
				render()
				close(message.done)
			case err := <-p.stop:
				p.mu.Lock()
				p.finished, p.failure = true, err
				p.mu.Unlock()
				render()
				return
			case <-p.refresh:
				render()
			case <-ticker.C:
				render()
			}
		}
	}()
	return p
}

func (p *backupProgress) print(message string) {
	request := progressMessage{text: message, done: make(chan struct{})}
	p.messages <- request
	<-request.done
}

func (p *backupProgress) setPlan(plan archive.DownloadPlan) {
	p.mu.Lock()
	p.plan = plan
	if plan.Ready {
		p.transferStarted = time.Now()
	}
	p.mu.Unlock()
	if plan.Ready {
		select {
		case p.refresh <- struct{}{}:
		default:
		}
	}
}

func (p *backupProgress) transfer(event archive.TransferEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := event.File.Path
	t := p.active[key]
	switch event.Kind {
	case archive.TransferStarted:
		p.order++
		t = activeTransfer{size: event.File.Size, order: p.order}
		if event.File.Data != nil {
			t.size = int64(len(event.File.Data))
		}
		p.active[key] = t
	case archive.TransferProgressed:
		p.bytes += event.Delta
		t.received += event.Delta
		p.active[key] = t
	case archive.TransferFinished:
		if event.Err == nil {
			p.completeFiles++
			p.completeBytes += t.received
		} else if event.File.Field == "thumbnail" && errors.Is(event.Err, archive.ErrThumbnailUnavailable) {
			p.unavailableThumbnails++
			p.bytes = max(0, p.bytes-t.received)
			p.plan.Bytes = max(0, p.plan.Bytes-t.size)
			p.plan.Files = max(0, p.plan.Files-1)
			if t.size <= 0 {
				p.plan.UnknownFiles = max(0, p.plan.UnknownFiles-1)
			}
		}
		delete(p.active, key)
	}
}

func (p *backupProgress) finish(err error) {
	p.stop <- err
	<-p.stopped
}

func (p *backupProgress) lines(now time.Time, width int) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	paint := func(color text.Color, value string) string {
		if p.colorful {
			return color.Sprint(value)
		}
		return value
	}
	label := "Scanning chat history"
	if p.plan.Chat != "" {
		label = "Scanning " + p.plan.Chat
	}
	percent, completed := 0.0, float64(p.completeBytes)
	var latest uint64
	for path, file := range p.active {
		completed += float64(min(file.received, max(0, file.size)))
		if file.order > latest {
			label, latest = filepath.Base(path), file.order
		}
	}
	if p.plan.Ready {
		if latest == 0 {
			label = "Preparing next file"
			if p.completeFiles >= p.plan.Files {
				label = "Finishing backup"
			}
		}
		if p.plan.UnknownFiles > 0 {
			completed = float64(p.completeFiles)
			percent = completed / float64(max(1, p.plan.Files)) * 100
		} else if p.plan.Bytes > 0 {
			percent = completed / float64(p.plan.Bytes) * 100
		}
	}
	percent = min(99.9, max(0, percent))
	if p.finished {
		label = "Backup complete"
		if p.failure != nil {
			label = "Backup failed"
		} else {
			percent = 100
			if p.unavailableThumbnails > 0 {
				label += fmt.Sprintf(" · %d thumbnail", p.unavailableThumbnails)
				if p.unavailableThumbnails != 1 {
					label += "s"
				}
				label += " unavailable"
			}
		}
	}
	elapsed := max(0, now.Sub(p.started))
	transferTime := now.Sub(p.transferStarted).Seconds()
	speed := 0.0
	if !p.transferStarted.IsZero() && transferTime > 0 {
		speed = float64(p.bytes) / transferTime
	}
	eta := "--"
	if percent > 0 && percent < 100 && !p.transferStarted.IsZero() {
		eta = (time.Duration(transferTime*(100-percent)/percent) * time.Second).String()
	}
	if p.finished && p.failure == nil {
		eta = "0s"
	}
	size := formatProgressBytes(p.bytes)
	if p.plan.Ready && p.plan.UnknownFiles == 0 {
		size += " / " + formatProgressBytes(p.plan.Bytes)
	}
	metrics := paint(text.FgGreen, fmt.Sprintf("Elapsed %s · ETA %s", elapsed.Round(time.Second), eta)) +
		" · " + paint(text.FgMagenta, formatProgressBytes(int64(speed))+"/s")
	if !p.plan.Ready {
		size = formatProgressBytes(p.plan.Bytes) + " found"
		metrics = paint(text.FgGreen, fmt.Sprintf("Elapsed %s", elapsed.Round(time.Second))) +
			" · " + paint(text.FgCyan, fmt.Sprintf("%d messages scanned · %d files found", p.plan.Messages, p.plan.Files))
	}
	barWidth := max(3, min(32, width-runewidth.StringWidth(size)-13))
	fill := int(math.Floor(float64(barWidth) * percent / 100))
	percentage := fmt.Sprintf("%5.1f%%", percent)
	if !p.plan.Ready && !p.finished {
		percentage = "    --"
	}
	bar := paint(text.FgYellow, "["+strings.Repeat("#", fill)+strings.Repeat(".", barWidth-fill)+"]") +
		" " + paint(text.FgHiRed, percentage) + "  " + paint(text.FgCyan, size)
	label = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, label)
	labelColor := text.FgBlue
	if p.finished {
		labelColor = text.FgGreen
		if p.failure != nil {
			labelColor = text.FgRed
		}
	}
	limit := max(1, width-1)
	return []string{
		paint(labelColor, runewidth.Truncate(label, limit, "…")),
		text.Snip(bar, limit, "…"),
		text.Snip(metrics, limit, "…"),
	}
}

func formatProgressBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	value := float64(bytes)
	for _, unit := range []string{"KiB", "MiB", "GiB", "TiB"} {
		value /= 1024
		if value < 1024 || unit == "TiB" {
			return fmt.Sprintf("%.1f %s", value, unit)
		}
	}
	return ""
}
