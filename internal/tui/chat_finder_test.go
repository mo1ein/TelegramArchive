package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func loadedChatFinder(t *testing.T) Model {
	t.Helper()
	m := chatChoiceModel(t)
	rows := []item{{ID: "42", Title: "Book Club"}, {ID: "84", Title: "Weekend plans"}}
	for i := range 40 {
		rows = append(rows, item{ID: fmt.Sprint(100 + i), Title: fmt.Sprintf("Chat %02d", i)})
	}
	m.run = func(context.Context, Plan) tea.Cmd {
		return func() tea.Msg {
			return startedMsg{&job{terminal: newTranscript(80, 24), done: true, streamSeen: true, items: rows, cancel: func() {}}}
		}
	}
	return startChatChoiceTest(t, m)
}

func TestChatFinderUsesLargeViewportAndKeepsMouseRowsAligned(t *testing.T) {
	for _, size := range [][2]int{{55, 20}, {80, 24}, {120, 40}} {
		m := loadedChatFinder(t)
		m = update(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		bounds := m.inlineChoiceBounds()
		if bounds.width != size[0]-4 || bounds.height != size[1]-12 {
			t.Fatal("chat picker did not expand with the terminal")
		}
		view := ansi.Strip(m.View().Content)
		if !strings.Contains(view, "Choose chats to back up") || strings.Contains(view, "Save to folder") {
			t.Fatal("chat finder did not replace the small settings dropdown")
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("finder overflow")
			}
		}
		if len(strings.Split(view, "\n")) > size[1] {
			t.Fatal("actions fell below terminal")
		}
		m = update(m, key(tea.KeyEnd, ""))
		m = clickChoiceText(t, m, "[ ] Chat 39")
		if !m.picker.selected["139"] {
			t.Fatal("scrolled mouse selection selected the wrong chat")
		}
		m = clickChoiceText(t, m, "[ Use selected chats ]")
		if m.picker != nil || m.form.field("chat").Value != "139" {
			t.Fatal("apply did not save selected chat")
		}
	}
}

func TestChatFinderFuzzySearchDoesNotSaveSearchAsAChat(t *testing.T) {
	m := loadedChatFinder(t)
	m = update(m, tea.PasteMsg{Content: "bkcl"})
	if rows := m.picker.filtered(); len(rows) != 1 || rows[0] != "42" {
		t.Fatal("abbreviated search did not match Book Club")
	}
	m = update(m, key(tea.KeyLeft, ""))
	if m.picker == nil {
		t.Fatal("moving the search cursor unexpectedly applied the selection")
	}
	m = update(m, key(tea.KeyEnter, ""))
	if !m.picker.selected["42"] || m.picker.selected["bkcl"] {
		t.Fatal("search text was added as a chat instead of selecting its result")
	}
	m = update(m, key(tea.KeyEnter, "", tea.ModCtrl))
	if m.picker != nil || m.form.field("chat").Value != "42" || m.screen != configure {
		t.Fatal("Ctrl+Enter did not return selected chats to the form")
	}
}

func TestChatFinderTabSelectionAndEscapeDiscard(t *testing.T) {
	m := loadedChatFinder(t)
	m = update(m, key(tea.KeyTab, ""))
	if !m.picker.selected["me"] || m.picker.cursor != 1 || m.picker == nil {
		t.Fatal("Tab should check a chat and move down")
	}
	m = update(m, key('n', "", tea.ModCtrl))
	m = update(m, key('p', "", tea.ModCtrl))
	if m.picker.cursor != 1 {
		t.Fatal("Ctrl+N/P navigation disagreed")
	}
	m = update(m, key(tea.KeyF6, ""))
	if !m.buttonsFocused {
		t.Fatal("buttons cannot receive focus")
	}
	m = update(m, key(tea.KeyEscape, ""))
	if m.picker != nil || m.form.field("chat").Value != "" {
		t.Fatal("Escape must discard the unconfirmed selection")
	}
}
