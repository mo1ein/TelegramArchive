package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/mo1ein/TelegramArchive/internal/archive"
)

func TestBackupProgressShowsOneOverallBarAndCurrentFile(t *testing.T) {
	start := time.Now().Add(-10 * time.Second)
	p := &backupProgress{started: start, transferStarted: start, active: map[string]activeTransfer{}}
	p.setPlan(archive.DownloadPlan{Ready: true, Bytes: 100, Files: 2})
	p.transferStarted = start
	first := archive.File{Path: "files/first.zip", Size: 40}
	second := archive.File{Path: "files/second.mp4", Size: 60}
	p.transfer(archive.TransferEvent{Kind: archive.TransferStarted, File: first})
	p.transfer(archive.TransferEvent{Kind: archive.TransferProgressed, File: first, Delta: 20})
	p.transfer(archive.TransferEvent{Kind: archive.TransferStarted, File: second})
	p.transfer(archive.TransferEvent{Kind: archive.TransferProgressed, File: second, Delta: 10})
	text := strings.Join(p.lines(start.Add(10*time.Second), 80), "\n")
	for _, want := range []string{"second.mp4", "30.0%", "30 B / 100 B", "Elapsed", "ETA", "/s"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Count(text, "[") != 1 || strings.Contains(text, "messages") || strings.Contains(text, "active") {
		t.Fatalf("noisy or duplicate progress:\n%s", text)
	}
	p.transfer(archive.TransferEvent{Kind: archive.TransferFinished, File: second})
	if got := p.lines(start.Add(10*time.Second), 80)[0]; got != "first.zip" {
		t.Fatalf("completed filename remained visible: %s", got)
	}
}

func TestBackupProgressShowsHistoryScanWork(t *testing.T) {
	start := time.Now()
	p := &backupProgress{started: start, active: map[string]activeTransfer{}}
	p.setPlan(archive.DownloadPlan{Chat: "Fadi", Messages: 23500, Files: 900, Bytes: 1024 * 1024 * 1024})
	output := strings.Join(p.lines(start.Add(3*time.Minute), 80), "\n")
	for _, want := range []string{"Scanning Fadi", "23500 messages scanned", "900 files found", "1.0 GiB found", "Elapsed 3m0s"} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing %q in preparation:\n%s", want, output)
		}
	}
	if strings.Count(output, "[") != 1 || strings.Contains(output, "%") || strings.Contains(output, "ETA") || strings.Contains(output, "/s") {
		t.Fatalf("history scan invented download progress:\n%s", output)
	}
	// Chat titles are untrusted, just like filenames.
	p.setPlan(archive.DownloadPlan{Chat: "Fadi\n\x1b[31m" + strings.Repeat("名字", 30)})
	for _, line := range p.lines(start, 40) {
		if runewidth.StringWidth(line) >= 40 || strings.ContainsAny(line, "\n\r\x1b") {
			t.Fatalf("unsafe or overflowing preparation: %q", line)
		}
	}
}

func TestBackupProgressExcludesUnavailableThumbnails(t *testing.T) {
	start := time.Now()
	p := &backupProgress{started: start, active: map[string]activeTransfer{}}
	p.setPlan(archive.DownloadPlan{Ready: true, Files: 2, Bytes: 100})
	thumb := archive.File{Field: "thumbnail", Path: "video_files/thumb.jpg", Size: 20}
	p.transfer(archive.TransferEvent{Kind: archive.TransferStarted, File: thumb})
	p.transfer(archive.TransferEvent{Kind: archive.TransferProgressed, File: thumb, Delta: 4})
	p.transfer(archive.TransferEvent{Kind: archive.TransferFinished, File: thumb, Err: archive.ErrThumbnailUnavailable})
	video := archive.File{Field: "file", Path: "video_files/movie.mp4", Size: 80}
	p.transfer(archive.TransferEvent{Kind: archive.TransferStarted, File: video})
	p.transfer(archive.TransferEvent{Kind: archive.TransferProgressed, File: video, Delta: 40})
	output := strings.Join(p.lines(start.Add(time.Second), 100), "\n")
	if !strings.Contains(output, "50.0%") || !strings.Contains(output, "40 B / 80 B") {
		t.Fatalf("unavailable thumbnail distorted remaining progress:\n%s", output)
	}
	p.finished = true
	if output = strings.Join(p.lines(start.Add(time.Second), 100), "\n"); !strings.Contains(output, "1 thumbnail unavailable") {
		t.Fatalf("completion hid missing thumbnail:\n%s", output)
	}
}

func TestBackupProgressFinishesOnlyAfterArchiveSucceeds(t *testing.T) {
	start := time.Now()
	p := &backupProgress{started: start, active: map[string]activeTransfer{}}
	p.setPlan(archive.DownloadPlan{Ready: true, Bytes: 4, Files: 1})
	file := archive.File{Path: "files/one.zip", Size: 4}
	p.transfer(archive.TransferEvent{Kind: archive.TransferStarted, File: file})
	p.transfer(archive.TransferEvent{Kind: archive.TransferProgressed, File: file, Delta: 4})
	p.transfer(archive.TransferEvent{Kind: archive.TransferFinished, File: file})
	if text := strings.Join(p.lines(start.Add(time.Second), 80), "\n"); strings.Contains(text, "100.0%") {
		t.Fatalf("reported completion before rendering finished: %s", text)
	}
	p.finished, p.failure = true, errors.New("render failed")
	if text := strings.Join(p.lines(start.Add(time.Second), 80), "\n"); !strings.Contains(text, "failed") || strings.Contains(text, "100.0%") {
		t.Fatalf("failure reported success: %s", text)
	}
	p.failure = nil
	if text := strings.Join(p.lines(start.Add(time.Second), 80), "\n"); !strings.Contains(text, "100.0%") {
		t.Fatalf("missing completion: %s", text)
	}
}

func TestBackupProgressFitsNarrowTerminalsAndSanitizesFilenames(t *testing.T) {
	p := &backupProgress{started: time.Now(), active: map[string]activeTransfer{}}
	if text := strings.Join(p.lines(time.Now(), 40), "\n"); strings.Contains(text, "%") || !strings.Contains(text, "Scanning") {
		t.Fatalf("preparation invented a percentage: %s", text)
	}
	p.setPlan(archive.DownloadPlan{Ready: true, Files: 1, UnknownFiles: 1})
	p.transfer(archive.TransferEvent{Kind: archive.TransferStarted, File: archive.File{Path: "files/خیلی_طولانی_" + strings.Repeat("名字", 30) + "\n\x1b[31m.zip"}})
	for _, line := range p.lines(time.Now(), 40) {
		if runewidth.StringWidth(line) >= 40 || strings.ContainsAny(line, "\n\r\x1b") {
			t.Fatalf("unsafe or overflowing progress line: %q", line)
		}
	}
}

func TestBackupProgressRunsWithArchiveAndWritesPlainLogs(t *testing.T) {
	var output bytes.Buffer
	progress := newBackupProgress(&output)
	source := &reconnectArchiveSource{downloads: map[string]int{}}
	_, err := archive.RunBatch(context.Background(), source, t.TempDir(), []archive.Chat{{ID: 1, Key: "user_1"}}, archive.Options{
		Concurrency: 1, Media: []string{"videos"}, OnPlan: progress.setPlan, OnTransfer: progress.transfer,
	}, nil)
	progress.finish(err)
	if err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if strings.Contains(text, "\x1b") || strings.Count(text, "100.0%") != 1 || !strings.Contains(text, "16 B / 16 B") {
		t.Fatalf("invalid progress log:\n%s", text)
	}
}

func TestBackupProgressKeepsArchiveSummaryVisible(t *testing.T) {
	var output bytes.Buffer
	progress := newBackupProgress(&output)
	progress.print("Finished archive test | completed\nMessages archived: 4 total\n")
	progress.finish(nil)
	if text := output.String(); strings.Count(text, "Finished archive test") != 1 || !strings.Contains(text, "Messages archived: 4 total") || strings.Contains(text, "\x1b") {
		t.Fatalf("summary was lost or corrupted in progress output: %q", text)
	}
}
