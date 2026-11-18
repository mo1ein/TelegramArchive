package tui

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/iyear/tdl/pkg/prog"
	pw "github.com/jedib0t/go-pretty/v6/progress"
)

// Exercise the actual dependency renderer so changes to its output format
// cannot silently disconnect download measurements from the native cards.
type transferCapture struct {
	sync.Mutex
	terminal *transcript
}

func (c *transferCapture) Write(p []byte) (int, error) {
	c.Lock()
	defer c.Unlock()
	c.terminal.write(p)
	return len(p), nil
}
func (c *transferCapture) text() string { c.Lock(); defer c.Unlock(); return c.terminal.text() }

func TestDownloadProgressAdaptsUpstreamRenderer(t *testing.T) {
	capture := &transferCapture{terminal: newTranscript(240, 40)}
	writer := prog.New(pw.FormatBytes)
	writer.SetOutputWriter(capture)
	writer.SetMessageWidth(50)
	writer.SetTrackerLength(20)
	writer.SetUpdateFrequency(10 * time.Millisecond)
	writer.SetNumTrackersExpected(2)
	first := prog.AppendTracker(writer, pw.FormatBytes, "Chat(42):1 -> downloads/movie.mp4", 1000)
	second := prog.AppendTracker(writer, pw.FormatBytes, "Chat(42):2 -> downloads/music.mp3", 1000)
	first.SetValue(250)
	second.SetValue(750)
	done := make(chan struct{})
	go func() { writer.Render(); close(done) }()
	defer func() { writer.Stop(); <-done }()
	var rows []downloadTransfer
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rows, _ = parseDownloadProgress(capture.text())
		if len(rows) == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(rows) != 2 {
		t.Fatalf("expected two native trackers from actual renderer:\n%s", capture.text())
	}
	if rows[0].percent != .25 || rows[1].percent != .75 {
		t.Fatalf("incorrect measurements: %+v", rows)
	}
	for _, row := range rows {
		if row.detail == "" {
			t.Fatal("transfer stats lost")
		}
	}
}

func TestDownloadProgressKeepsPromptsErrorsAndUnknownOutput(t *testing.T) {
	output := "Continue download?\nvideo.mp4 ... 25.0% [##......] [25MiB in 1s; 25MiB/s; ~ETA: 3s]\n[##......] [25MiB/s; 1s]\nerror: network unavailable\nnew upstream format 40%\nother.mp4 ...  ???  [.<#>...] [0B in 1s]"
	rows, activity := parseDownloadProgress(output)
	if len(rows) != 2 || !rows[0].determinate || rows[1].determinate {
		t.Fatalf("incorrect parsed rows: %+v", rows)
	}
	for _, want := range []string{"Continue download?", "error: network unavailable", "new upstream format 40%"} {
		if !strings.Contains(activity, want) {
			t.Fatalf("lost activity %q: %s", want, activity)
		}
	}
	if strings.Contains(activity, "[##") {
		t.Fatalf("raw progress duplicated in activity: %s", activity)
	}
}

func TestDownloadProgressFitsWorkspaceAndAnimates(t *testing.T) {
	root, _ := fixture()
	m := New(Catalog(root), nil, "default", context.Background())
	m.screen = running
	m.plan = Plan{Path: []string{"download"}, Title: "Download files"}
	m.started = time.Now()
	m.output = "video.mp4 ... 25.0% [##......] [25MiB in 1s; 25MiB/s; ~ETA: 3s]\nmusic.mp3 ... 75.0% [######..] [75MiB in 1s]"
	commands := m.syncDownloadProgress()
	if len(commands) != 2 || commands[0] == nil {
		t.Fatal("downloads did not schedule animated bars")
	}
	for _, size := range [][2]int{{55, 20}, {80, 24}, {120, 40}} {
		width, height := size[0]-4, size[1]-9
		view := m.taskView(width, height)
		if lipgloss.Width(view) > width || lipgloss.Height(view) > height {
			t.Fatalf("progress exceeds %dx%d: %dx%d\n%s", width, height, lipgloss.Width(view), lipgloss.Height(view), ansi.Strip(view))
		}
		for _, want := range []string{"video.mp4", "25.0%", "ACTIVITY"} {
			if !strings.Contains(ansi.Strip(view), want) {
				t.Fatalf("missing %s:\n%s", want, view)
			}
		}
	}
	m.downloads = nil
	view := ansi.Strip(m.downloadProgressView(74, 15))
	if strings.Contains(view, "%") || !strings.Contains(view, "Finalizing") {
		t.Fatalf("unexpected waiting display: %s", view)
	}
	if (Plan{Path: []string{"download"}, Args: []string{"--serve=true"}}).downloadProgress() {
		t.Fatal("server mode must keep its normal activity display")
	}
}
