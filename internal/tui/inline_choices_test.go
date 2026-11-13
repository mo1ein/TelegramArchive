package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func inlineModel(t *testing.T, accounts ...string) Model {
	t.Helper()
	root, action := fixture()
	m := New(Catalog(root), nil, accounts[0], context.Background(), WithSavedAccounts(accounts))
	m.open(action)
	focusField(&m, "ns")
	return m
}

func TestMultiChoicesAreVisibleWithinTheFormAndTabSaves(t *testing.T) {
	m := inlineModel(t, "personal", "work", "family")
	assertForm := func() {
		t.Helper()
		view := ansi.Strip(m.View().Content)
		for _, want := range []string{m.form.Action.Title, "Essentials", "Accounts to back up", "personal", "work", "family", "[ Review ]"} {
			if !strings.Contains(view, want) {
				t.Fatalf("the inline choices hid %q:\n%s", want, view)
			}
		}
	}
	if strings.Contains(ansi.Strip(m.View().Content), "[ ] family") {
		t.Fatal("a closed selector should stay compact")
	}
	m = update(m, key(tea.KeyEnter, ""))
	assertForm()
	m = update(m, key(tea.KeyRight, ""))
	m = update(m, key(' ', " "))
	before := m.focus
	m = update(m, key(tea.KeyTab, ""))
	if m.picker != nil || m.focus != before+1 || m.form.field("ns").Value != "personal,work" {
		t.Fatal("Tab must save the checked cards and advance to the next setting")
	}
}

func TestInlineReviewSavesSelectionAndEscapeUndoesIt(t *testing.T) {
	m := inlineModel(t, "personal", "work")
	set(&m.form, "chat", "me")
	m = update(m, key(tea.KeyEnter, ""))
	m = update(m, key(tea.KeyRight, ""))
	m = update(m, key(' ', " "))
	m = update(m, key(tea.KeyEscape, ""))
	if m.form.field("ns").Value != "personal" || m.screen != configure {
		t.Fatal("Esc should undo the selection without leaving the form")
	}
	m = update(m, key(tea.KeyEnter, ""))
	m = update(m, key(tea.KeyRight, ""))
	m = update(m, key(' ', " "))
	m = update(m, key('r', "", tea.ModCtrl))
	if m.screen != review || m.picker != nil || len(m.plan.Tasks) != 2 {
		t.Fatal("review did not save the currently checked accounts")
	}
}

func TestInlineEmptySelectionBlocksLeavingTheControl(t *testing.T) {
	for _, leave := range []tea.KeyPressMsg{key(tea.KeyTab, ""), key('r', "", tea.ModCtrl)} {
		m := inlineModel(t, "personal", "work")
		m = update(m, key(tea.KeyEnter, ""))
		m = update(m, key('d', "", tea.ModCtrl))
		focus := m.focus
		m = update(m, leave)
		if m.screen != configure || m.focus != focus || m.picker == nil || m.picker.err == "" {
			t.Fatal("an empty selection must retain focus and explain the problem")
		}
	}
}

func clickChoiceText(t *testing.T, m Model, text string) Model {
	t.Helper()
	for y, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
		if index := strings.Index(line, text); index >= 0 {
			return update(m, tea.MouseClickMsg{X: ansi.StringWidth(line[:index]), Y: y, Button: tea.MouseLeft})
		}
	}
	t.Fatalf("could not find clickable %q in:\n%s", text, ansi.Strip(m.View().Content))
	return m
}

func TestInlineChoicesCanBeClickedAndKeepChecksWhileFiltering(t *testing.T) {
	for _, size := range [][2]int{{55, 20}, {80, 24}, {110, 32}, {120, 20}} {
		m := inlineModel(t, "personal", "work")
		m.width, m.height = size[0], size[1]
		m = update(m, key(tea.KeyEnter, ""))
		m = update(m, key(tea.KeyDown, ""))
		m = clickChoiceText(t, m, "[ ] work")
		if m.picker == nil || !m.picker.checked("work") {
			t.Fatalf("clicking an inline item did not check it at %v", size)
		}
		m = update(m, tea.PasteMsg{Content: "personal"})
		if !m.picker.checked("work") || !slices.Equal(m.picker.filtered(), []string{"personal"}) {
			t.Fatal("filtering discarded a checked item outside the search")
		}
		m = clickChoiceText(t, m, "[Done]")
		if m.picker != nil || m.form.field("ns").Value != "personal,work" {
			t.Fatal("Done did not save the selection")
		}
	}
}

func TestInlineDropdownFitsAndScrollsAfterResizing(t *testing.T) {
	accounts := []string{"personal", "名字", "آرشیو"}
	for n := range 24 {
		accounts = append(accounts, fmt.Sprintf("account-%02d", n))
	}
	for _, size := range [][2]int{{55, 20}, {80, 24}, {110, 32}, {120, 20}, {120, 36}} {
		m := inlineModel(t, accounts...)
		m = update(m, key(tea.KeyEnter, ""))
		m = update(m, key(tea.KeyEnd, ""))
		m = update(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := ansi.Strip(m.View().Content)
		if !strings.Contains(view, "account-23") || !strings.Contains(view, "[ Review ]") || len(strings.Split(view, "\n")) > size[1] {
			t.Fatalf("the final item or page actions were hidden at %v:\n%s", size, view)
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("choice grid overflow at %v: %q", size, line)
			}
		}
		m = clickChoiceText(t, m, "[ ] account-23")
		if !m.picker.checked("account-23") {
			t.Fatal("mouse selection disagreed with the scrolled grid")
		}
	}
}

func TestInlineMouseReviewSavesAndEmptyAccountsOfferConnection(t *testing.T) {
	m := inlineModel(t, "personal", "work")
	set(&m.form, "chat", "me")
	m = update(m, key(tea.KeyEnter, ""))
	m = clickChoiceText(t, m, "[ ] work")
	m = clickChoiceText(t, m, "[ Review ]")
	if m.screen != review || len(m.plan.Tasks) != 2 {
		t.Fatal("the form Review button did not apply the active selection")
	}
	m = inlineModel(t, "personal")
	m = update(m, accountsLoadedMsg{})
	m = update(m, key(tea.KeyEnter, ""))
	if !strings.Contains(ansi.Strip(m.View().Content), "No saved accounts") {
		t.Fatal("an empty list should explain how to get started")
	}
}

func TestTypingFiltersInlineAndEscapeDiscardsUnsavedChecks(t *testing.T) {
	m := inlineModel(t, "personal", "work")
	m = update(m, key('w', "w"))
	if m.picker == nil || !slices.Equal(m.picker.filtered(), []string{"work"}) {
		t.Fatal("typing should filter the cards directly in the form")
	}
	m = update(m, key(tea.KeyDown, ""))
	m = update(m, key(' ', " "))
	m = update(m, tea.PasteMsg{Content: "unmatched"})
	m = update(m, key(tea.KeyEscape, ""))
	if m.picker != nil || m.form.field("ns").Value != "personal" {
		t.Fatal("Escape during filtering should undo the unsaved card selection")
	}
}

func TestInlineAccountRefreshHasItsOwnShortcut(t *testing.T) {
	m := inlineModel(t, "personal", "work")
	m.accountLoader = func(context.Context, string) ([]string, error) {
		return []string{"personal", "work", "family"}, nil
	}
	m = update(m, key(tea.KeyEnter, ""))
	next, command := m.Update(key('l', "", tea.ModCtrl))
	if command == nil {
		t.Fatal("Ctrl+L should refresh saved accounts")
	}
	m = update(next.(Model), command())
	if !slices.Contains(m.picker.choices, "family") || m.screen != configure || !m.picker.checked("personal") {
		t.Fatal("refresh should update the inline choices and preserve checks")
	}
}

func TestDropdownWheelScrollsIndependentlyAndPreservesChecks(t *testing.T) {
	accounts := []string{"personal"}
	for index := range 24 {
		accounts = append(accounts, fmt.Sprintf("account-%02d", index))
	}
	m := inlineModel(t, accounts...)
	m.width, m.height = 110, 32
	m = update(m, key(tea.KeyEnter, ""))
	bounds := m.inlineChoiceBounds()
	if bounds.height > 7 || bounds.width >= m.width-4 {
		t.Fatal("the dropdown should be bounded inside the argument's value column")
	}
	focus := m.focus
	for range 3 {
		m = update(m, tea.MouseWheelMsg{X: bounds.x + 3, Y: bounds.y + 2, Button: tea.MouseWheelDown})
	}
	if m.focus != focus || m.picker == nil || m.picker.cursor != 9 || !m.picker.checked("personal") {
		t.Fatal("scrolling inside the dropdown moved the form or discarded checks")
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "┃") || !strings.Contains(view, "account-08") || strings.Contains(view, "▸ [✓] personal") {
		t.Fatalf("the dropdown did not scroll its visible options:\n%s", view)
	}
	m = clickChoiceText(t, m, "[ ] account-08")
	if !m.picker.checked("account-08") || !m.picker.checked("personal") {
		t.Fatal("clicking after scrolling toggled the wrong item")
	}
	m = update(m, tea.MouseWheelMsg{X: 3, Y: bounds.y + 2, Button: tea.MouseWheelDown})
	if m.picker != nil || m.focus != focus+3 || m.form.field("ns").Value != "personal,account-08" {
		t.Fatal("scrolling outside the dropdown should save it and move through the arguments")
	}
}

func TestClickingAnArgumentBelowTheDropdownTargetsItsOriginalRow(t *testing.T) {
	m := inlineModel(t, "personal", "work")
	m = update(m, key(tea.KeyEnter, ""))
	m = update(m, key(tea.KeyDown, ""))
	m = update(m, key(' ', " "))
	m = clickChoiceText(t, m, "Chats to back up")
	fields := m.visibleFields()
	if m.picker == nil || m.picker.field != "chat" || !m.picker.custom || m.editing || m.form.Fields[fields[m.focus]].Name != "chat" || m.form.field("ns").Value != "personal,work" {
		t.Fatal("closing the dropdown changed the field under the mouse")
	}
}

func TestSingleChoicesUseTheSameCompactDropdown(t *testing.T) {
	m := inlineModel(t, "personal")
	focusField(&m, "format")
	m = update(m, key(tea.KeyEnter, ""))
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Chats to back up") || !strings.Contains(view, "○ HTML") || !strings.Contains(view, "○ Both") {
		t.Fatal("the single selector should stay anchored in the argument form")
	}
	m = clickChoiceText(t, m, "○ HTML")
	if m.picker != nil || m.form.field("format").Value != "html" || m.screen != configure {
		t.Fatal("clicking a single choice should apply it in place")
	}
}

func TestClickingAScrolledCheckboxKeepsItUnderTheMouse(t *testing.T) {
	accounts := []string{"personal"}
	for index := range 20 {
		accounts = append(accounts, fmt.Sprintf("account-%02d", index))
	}
	m := inlineModel(t, accounts...)
	m = clickChoiceText(t, m, "Accounts to back up")
	b := m.inlineChoiceBounds()
	for range 3 {
		m = update(m, tea.MouseWheelMsg{X: b.x + 3, Y: b.y + 2, Button: tea.MouseWheelDown})
	}
	for y, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
		if index := strings.Index(line, "[ ] account-05"); index >= 0 {
			x := ansi.StringWidth(line[:index])
			click := tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
			m = update(m, click)
			m = update(m, tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
			after := strings.Split(ansi.Strip(m.View().Content), "\n")
			if !strings.Contains(after[y], "[✓] account-05") {
				t.Fatalf("clicking moved the item away from the mouse:\n%s", strings.Join(after, "\n"))
			}
			m = update(m, click)
			if m.picker.checked("account-05") || !m.picker.checked("personal") {
				t.Fatal("a second click should uncheck the same item and retain the other choices")
			}
			return
		}
	}
	t.Fatal("the scrolled checkbox was not visible")
}
