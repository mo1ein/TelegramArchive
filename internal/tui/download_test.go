package tui

import (
	"context"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
)

func downloadFixture() Action {
	command := &cobra.Command{Use: "download", RunE: func(*cobra.Command, []string) error { panic("must not run") }}
	command.Flags().StringSlice("url", nil, "message links")
	command.Flags().StringSlice("file", nil, "export files")
	command.Flags().StringSlice("include", nil, "extensions")
	command.Flags().StringSlice("exclude", nil, "extensions")
	command.Flags().String("dir", "downloads", "folder")
	command.Flags().String("ns", "default", "account")
	command.Flags().Bool("group", false, "album")
	command.Flags().Bool("skip-same", false, "skip duplicates")
	command.Flags().Bool("continue", false, "continue")
	command.Flags().Bool("restart", false, "restart")
	return Action{Title: "download", Path: []string{"download"}, Command: command}
}

func TestDownloadPasteAndEnterRunsWithoutCommandSyntax(t *testing.T) {
	for _, width := range []int{55, 80, 120} {
		m := New(nil, nil, "default", context.Background())
		m.width, m.height = width, 24
		m.open(downloadFixture())
		if !m.editing || m.form.Fields[m.visibleFields()[m.focus]].Name != "url" || m.form.field("arguments") != nil {
			t.Fatal("download must start in its URL input without an arguments field")
		}
		runs := 0
		m.run = func(_ context.Context, plan Plan) tea.Cmd {
			runs++
			if !slices.Contains(plan.Args, "--url=https://t.me/channel/123") {
				t.Fatal(plan.Args)
			}
			return func() tea.Msg { return finishedMsg{} }
		}
		m = update(m, tea.PasteMsg{Content: "t.me/channel/123"})
		for _, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("URL bar overflows %d: %s", width, line)
			}
		}
		m = update(m, key(tea.KeyEnter, ""))
		if runs != 1 || m.screen != running {
			t.Fatal("Enter did not start the download")
		}
	}
}

func TestDownloadMultiplePastedLinksPreserveCommasAndParameters(t *testing.T) {
	m := New(nil, nil, "default", context.Background())
	m.open(downloadFixture())
	m = update(m, tea.PasteMsg{Content: "https://t.me/channel/123?x=a,b\nhttps://t.me/c/123/456?thread=12"})
	m = update(m, key(tea.KeyTab, ""))
	set(&m.form, "dir", "My downloads")
	set(&m.form, "group", "true")
	plan, err := m.form.Plan(nil)
	if err != nil {
		t.Fatal(err)
	}
	command := downloadFixture().Command
	if err := command.ParseFlags(plan.Args[1:]); err != nil {
		t.Fatal(err)
	}
	urls, _ := command.Flags().GetStringSlice("url")
	if !slices.Equal(urls, []string{"https://t.me/channel/123?x=a,b", "https://t.me/c/123/456?thread=12"}) {
		t.Fatal(urls)
	}
	dir, _ := command.Flags().GetString("dir")
	group, _ := command.Flags().GetBool("group")
	if dir != "My downloads" || !group {
		t.Fatal("download options were lost")
	}
}

func TestDownloadValidationAndExportAlternative(t *testing.T) {
	for _, value := range []string{"", "not-a-url", "https://youtube.com/watch?v=123", "https://t.me/channel"} {
		m := New(nil, nil, "default", context.Background())
		m.open(downloadFixture())
		m = update(m, tea.PasteMsg{Content: value})
		m = update(m, key(tea.KeyEnter, ""))
		if m.screen != configure || m.err == "" {
			t.Fatalf("invalid link %q escaped validation", value)
		}
	}
	form := NewForm(downloadFixture(), nil)
	set(&form, "file", "export.json")
	if _, err := form.Plan(nil); err != nil {
		t.Fatal(err)
	}
	set(&form, "include", "mp4")
	set(&form, "exclude", "jpg")
	if _, err := form.Plan(nil); err == nil {
		t.Fatal("conflicting filters accepted")
	}
}

func TestDownloadButtonSavesPastedURLAndStarts(t *testing.T) {
	m := New(nil, nil, "default", context.Background())
	m.open(downloadFixture())
	runs := 0
	m.run = func(context.Context, Plan) tea.Cmd { runs++; return func() tea.Msg { return finishedMsg{} } }
	m = update(m, tea.PasteMsg{Content: "https://t.me/channel/123"})
	m = clickChoiceText(t, m, "[ Download ]")
	if runs != 1 || m.screen != running {
		t.Fatal("Download did not save the active URL and run")
	}
}

func TestDownloadURLBoxKeepsOptionsAndMouseAligned(t *testing.T) {
	for _, size := range [][2]int{{55, 20}, {80, 24}, {120, 40}} {
		m := New(nil, nil, "default", context.Background())
		m.width, m.height = size[0], size[1]
		m.open(downloadFixture())
		view := ansi.Strip(m.View().Content)
		if !strings.Contains(view, "╭─ URL ") || !strings.Contains(view, "╰──") || !strings.Contains(view, "│") {
			t.Fatal("missing URL box")
		}
		// All three lines belong to the URL field, even after focus moves away.
		m = update(m, key(tea.KeyTab, ""))
		g := m.formLayout(m.width-4, m.height-9)
		for row := range 3 {
			index, hit := g.fieldAtRow(row)
			if !hit || m.form.Fields[m.visibleFields()[index]].Name != "url" {
				t.Fatalf("URL box row %d not clickable", row)
			}
		}
		m = update(m, tea.MouseClickMsg{X: 6, Y: 3 + g.top + 1, Button: tea.MouseLeft})
		if !m.editing || !m.editingDownloadURL() {
			t.Fatal("click inside box did not focus URL")
		}
		m = update(m, key(tea.KeyTab, ""))
		for focus := range m.visibleFields() {
			m.focus = focus
			view = ansi.Strip(m.View().Content)
			if len(strings.Split(view, "\n")) > m.height {
				t.Fatal("box pushes actions below terminal")
			}
			for _, line := range strings.Split(view, "\n") {
				if ansi.StringWidth(line) > m.width {
					t.Fatalf("overflow: %s", line)
				}
			}
			g = m.formLayout(m.width-4, m.height-9)
			row := g.fieldRow(focus)
			if row < 0 || row >= g.rows {
				t.Fatal("focused setting is offscreen")
			}
			index, hit := g.fieldAtRow(row)
			if !hit || index != focus {
				t.Fatal("setting hit area disagrees with its visible row")
			}
		}
	}
}
