package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/mattn/go-runewidth"
	"github.com/mo1ein/TelegramArchive/internal/archive"
)

func TestBackupProgressColorPreservesContentAndTerminalWidth(t *testing.T) {
	started := time.Now().Add(-time.Second)
	p := &backupProgress{started: started, transferStarted: started, active: map[string]activeTransfer{}, colorful: true}
	p.setPlan(archive.DownloadPlan{Ready: true, Bytes: 100, Files: 1})
	file := archive.File{Path: "files/video.mp4", Size: 100}
	p.transfer(archive.TransferEvent{Kind: archive.TransferStarted, File: file})
	p.transfer(archive.TransferEvent{Kind: archive.TransferProgressed, File: file, Delta: 40})
	colored := p.lines(started.Add(time.Second), 80)
	p.colorful = false
	plain := p.lines(started.Add(time.Second), 80)
	for i := range colored {
		if text.StripEscape(colored[i]) != plain[i] {
			t.Fatalf("color changed progress information: %q vs %q", colored[i], plain[i])
		}
		if strings.Contains(plain[i], "\x1b") {
			t.Fatalf("plain progress contains color escapes: %q", plain[i])
		}
		if text.FgCyan.Sprint("probe") != "probe" && !strings.Contains(colored[i], "\x1b[") {
			t.Fatalf("colored progress has no color: %q", colored[i])
		}
	}
	if !strings.Contains(strings.Join(plain, "\n"), "40.0%") {
		t.Fatal("progress lost its percentage")
	}
	p.colorful = true
	for _, line := range p.lines(started.Add(time.Second), 24) {
		if runewidth.StringWidth(text.StripEscape(line)) >= 24 {
			t.Fatalf("colored progress overflows a narrow terminal: %q", line)
		}
	}
}
