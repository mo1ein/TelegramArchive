package tui

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mo1ein/TelegramArchive/internal/taskui"
)

func focusField(m *Model, name string) {
	index := slices.IndexFunc(m.form.Fields, func(field Field) bool { return field.Name == name })
	m.focus = slices.Index(m.visibleFields(), index)
}

func TestFormAcceptsTypingAndPasteWithoutAnEditMode(t *testing.T) {
	root, action := fixture()
	m := New(Catalog(root), nil, "default", context.Background())
	m.open(action)
	focusField(&m, "output")
	m = update(m, key('/', "/"))
	if !m.editing || m.searching {
		t.Fatal("typing a path should edit the selected field")
	}
	m = update(m, key('u', "", tea.ModCtrl))
	m = update(m, tea.PasteMsg{Content: "آرشیو/Book Club"})
	previous := m.focus
	m = update(m, key(tea.KeyTab, ""))
	if got := m.form.field("output").Value; got != "آرشیو/Book Club" {
		t.Fatalf("Tab lost the input: %q", got)
	}
	if m.editing || (m.focus == previous && !m.buttonsFocused) {
		t.Fatal("Tab should save and move to the next setting")
	}
	m = update(m, key(tea.KeyTab, "", tea.ModShift))
	if m.focus != previous {
		t.Fatal("Shift+Tab should return to the previous setting")
	}
}

func TestReviewSavesTheActiveEditorAndRejectsInvalidValues(t *testing.T) {
	root, action := fixture()
	m := New(Catalog(root), nil, "default", context.Background())
	m.open(action)
	focusField(&m, "output")
	m = clickChoiceText(t, m, "[ Type path ]")
	m = update(m, key('u', "", tea.ModCtrl))
	m = update(m, tea.PasteMsg{Content: "exports/Saved Messages"})
	m = update(m, key('r', "", tea.ModCtrl))
	if m.screen != review || !slices.Contains(m.plan.Args, "--output=exports/Saved Messages") {
		t.Fatal("Review must include the value still being edited")
	}
	m = update(m, key(tea.KeyEscape, ""))
	m.advanced = true
	focusField(&m, "every")
	m = update(m, key(tea.KeyEnter, ""))
	m = update(m, key('u', "", tea.ModCtrl))
	m = update(m, tea.PasteMsg{Content: "tomorrow"})
	focus := m.focus
	m = update(m, key(tea.KeyTab, ""))
	if !m.editing || m.focus != focus || m.err == "" {
		t.Fatal("an invalid duration should keep focus and explain the error")
	}
	if m.form.field("every").Value == "tomorrow" {
		t.Fatal("invalid input overwrote the last valid setting")
	}
}

func TestChoosingAllChatsClearsAnEarlierChatSelection(t *testing.T) {
	root, action := fixture()
	m := New(Catalog(root), nil, "default", context.Background())
	m.open(action)
	set(&m.form, "chat", "me")
	focusField(&m, "all")
	m = update(m, key(' ', " "))
	if m.form.field("chat").Value != "" || m.form.field("all").Value != "true" {
		t.Fatal("the scope controls left mutually exclusive settings selected")
	}
	if _, err := m.form.Plan(nil); err != nil {
		t.Fatalf("switching the backup scope should remain runnable: %v", err)
	}
}

func TestNativeProgressUsesMeasurementsAndDoesNotInventScanPercentages(t *testing.T) {
	var buffer bytes.Buffer
	taskui.WriteStatus(&buffer, taskui.Progress{Label: "video.mp4", Detail: "30 MiB / 100 MiB", Summary: "ETA 10s · 3 MiB/s", Percent: .3, Determinate: true, CompletedFiles: 2, TotalFiles: 8})
	tr := newTranscript(80, 24)
	tr.write(buffer.Bytes())
	if tr.status == nil || tr.status.Percent != .3 || strings.Contains(tr.text(), "ta-progress") {
		t.Fatal("structured progress was not kept separate from activity")
	}
	root, _ := fixture()
	m := New(Catalog(root), nil, "default", context.Background())
	m.status = tr.status
	view := ansi.Strip(m.progressView(74))
	for _, want := range []string{"video.mp4", "30.0%", "2 / 8 files", "3 MiB/s"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing transfer measurement %q in %s", want, view)
		}
	}
	m.status = &taskui.Progress{Label: "Scanning history", Detail: "800 files found"}
	view = ansi.Strip(m.progressView(74))
	if strings.Contains(view, "%") || !strings.Contains(view, "Discovering") {
		t.Fatal("a history scan must show activity without a made-up percentage")
	}
}

func TestAReceivedChatCanBeArchivedBeforeTheBrowserFinishes(t *testing.T) {
	root, _ := fixture()
	m := New(Catalog(root), nil, "default", context.Background())
	m.screen, m.loading = result, true
	m.plan = Plan{Path: []string{"chat", "ls"}, Title: "Browse your chats"}
	rows := []item{{ID: "42", Title: "Synthetic chat"}, {ID: "84", Title: "Another synthetic chat"}}
	m.data = resultData{Kind: "chat ls", Items: rows}
	cancelled := false
	m.job = &job{terminal: newTranscript(80, 24), streamSeen: true, items: rows, cancel: func() { cancelled = true }}
	m.activate("archive")
	if !cancelled || m.pendingAction != "archive" || m.screen != result {
		t.Fatal("the lookup must stop before opening the selected chat")
	}
	m.resultSelected = 1 // Moving during shutdown must not change the chosen chat.
	m.job.done, m.job.ended, m.job.err = true, time.Now(), context.Canceled
	m = update(m, pollMsg{})
	if m.screen != configure || m.form.field("chat").Value != "42" || m.source == nil {
		t.Fatal("selected chat was lost while stopping the lookup")
	}
	m.restoreList()
	if len(m.data.Items) != 2 || m.data.Items[m.resultSelected].ID != "42" {
		t.Fatal("the received list must remain available without another request")
	}
}

func TestActiveSecretInputIsMaskedAndEscapeDiscardsTheEdit(t *testing.T) {
	root, action := fixture()
	action.Command.Flags().String("passcode", "", "local app passcode")
	m := New(Catalog(root), nil, "default", context.Background())
	m.open(action)
	m.advanced = true
	focusField(&m, "passcode")
	m = update(m, key(tea.KeyEnter, ""))
	m = update(m, tea.PasteMsg{Content: "synthetic private phrase"})
	if strings.Contains(ansi.Strip(m.View().Content), "synthetic private phrase") {
		t.Fatal("the active input exposed a passcode")
	}
	m = update(m, key(tea.KeyEscape, ""))
	if m.editing || m.form.field("passcode").Value != "" {
		t.Fatal("Escape should discard the current secret edit")
	}
}

func TestMouseOpensTheDisplayedWorkflowAfterScrollingANarrowDashboard(t *testing.T) {
	_, action := fixture()
	var actions []Action
	for i := range 12 {
		copy := action
		copy.Group, copy.Title = "Quick start", fmt.Sprintf("Workflow %d", i)
		actions = append(actions, copy)
	}
	m := New(actions, nil, "default", context.Background())
	m.width, m.height, m.selected = 80, 24, 9
	for y, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
		if strings.Contains(line, "Workflow 7") {
			m = update(m, tea.MouseClickMsg{X: 4, Y: y, Button: tea.MouseLeft})
			if m.screen != configure || m.form.Action.Title != "Workflow 7" {
				t.Fatal("mouse selection disagreed with the workflow shown under the cursor")
			}
			return
		}
	}
	t.Fatal("expected workflow was not visible")
}

func TestTabReachesReviewWithoutEditingAndReturnsToTheLastField(t *testing.T) {
	root, action := fixture()
	m := New(Catalog(root), nil, "default", context.Background())
	m.open(action)
	m.focus = len(m.visibleFields()) - 1
	m = update(m, key(tea.KeyTab, ""))
	if !m.buttonsFocused || m.buttons()[m.buttonFocus].ID != "review" {
		t.Fatal("Tab from the final field should focus the review action")
	}
	m = update(m, key(tea.KeyTab, "", tea.ModShift))
	if m.buttonsFocused || m.focus != len(m.visibleFields())-1 {
		t.Fatal("Shift+Tab should return to the final field")
	}
}

func TestLongUnicodeInputKeepsTheCursorEndVisibleAndFitsTheTerminal(t *testing.T) {
	root, action := fixture()
	for _, width := range []int{55, 80, 120} {
		m := New(Catalog(root), nil, "default", context.Background())
		m.width, m.height = width, 24
		m.open(action)
		focusField(&m, "output")
		m = clickChoiceText(t, m, "[ Type path ]")
		m = update(m, key('u', "", tea.ModCtrl))
		m = update(m, tea.PasteMsg{Content: strings.Repeat("آرشیو/名字/", 20) + "visible-end"})
		view := ansi.Strip(m.View().Content)
		if !strings.Contains(view, "visible-end") {
			t.Fatalf("input cursor scrolled out of view at width %d:\n%s", width, view)
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("input overflow at width %d: %q", width, line)
			}
		}
	}
}

func BenchmarkTaskPolling(b *testing.B) {
	for _, changed := range []bool{false, true} {
		name := "idle"
		if changed {
			name = "new-output"
		}
		b.Run(name, func(b *testing.B) {
			m := New(nil, nil, "default", context.Background())
			m.screen = running
			m.job = &job{terminal: newTranscript(80, 24)}
			m.job.terminal.write([]byte(strings.Repeat("A completed synthetic file\n", 1800)))
			m = update(m, pollMsg{})
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if changed {
					m.job.revision++
				}
				m = update(m, pollMsg{})
			}
		})
	}
}
