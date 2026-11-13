package tui

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
)

func chatChoiceModel(t *testing.T) Model {
	t.Helper()
	root, action := fixture()
	chat := &cobra.Command{Use: "chat"}
	list := &cobra.Command{Use: "ls", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error { panic("must not execute Telegram calls in tests") }}
	list.Flags().String("filter", "true", "chat filter")
	list.Flags().String("output", "table", "output")
	chat.AddCommand(list)
	root.AddCommand(chat)
	m := New(Catalog(root), nil, "personal", t.Context(), WithSavedAccounts([]string{"personal", "work"}))
	m.open(action)
	return m
}

func startChatChoiceTest(t *testing.T, m Model) Model {
	t.Helper()
	focusField(&m, "chat")
	next, command := m.Update(key(tea.KeyEnter, ""))
	if command == nil {
		t.Fatal("a fresh chat selector did not start its lookup")
	}
	next, poll := next.(Model).Update(command())
	if poll == nil {
		t.Fatal("chat lookup did not begin polling")
	}
	return update(next.(Model), poll())
}

func TestChatChoicesCacheIsScopedToTheSelectedAccount(t *testing.T) {
	m := chatChoiceModel(t)
	calls := 0
	var latest Plan
	m.run = func(_ context.Context, plan Plan) tea.Cmd {
		calls++
		latest = plan
		j := &job{terminal: newTranscript(80, 24), done: true, streamSeen: true, items: []item{{ID: "42", Title: "Personal chat"}}, cancel: func() {}}
		return func() tea.Msg { return startedMsg{j} }
	}
	m = startChatChoiceTest(t, m)
	m = update(m, key(tea.KeyLeft, ""))
	next, command := m.Update(key(tea.KeyEnter, ""))
	m = next.(Model)
	if command != nil || calls != 1 || m.picker.label("42") != "Personal chat" {
		t.Fatal("reopening the same account's chats should reuse the completed list")
	}
	m = update(m, key(tea.KeyEscape, ""))
	focusField(&m, "ns")
	m.openPicker(*m.form.field("ns"))
	m.picker.selected = map[string]bool{"work": true}
	m.applyPicker()
	focusField(&m, "chat")
	next, command = m.Update(key(tea.KeyEnter, ""))
	m = next.(Model)
	if command == nil || calls != 2 || !slices.Contains(latest.Args, "--ns=work") || slices.Contains(m.picker.choices, "42") {
		t.Fatal("switching accounts reused the previous account's chat names")
	}
	m.closePicker()
}

func TestClosingChatChoicesCancelsAndIgnoresLateWorkers(t *testing.T) {
	m := chatChoiceModel(t)
	stopped := false
	var ctx context.Context
	j := &job{terminal: newTranscript(80, 24), cancel: func() { stopped = true }}
	m.run = func(child context.Context, _ Plan) tea.Cmd {
		ctx = child
		return func() tea.Msg { return startedMsg{j} }
	}
	focusField(&m, "chat")
	next, command := m.Update(key(tea.KeyEnter, ""))
	m = update(next.(Model), key(tea.KeyEscape, ""))
	m = update(m, command())
	if m.picker != nil || m.chatLookup != nil || !stopped || ctx.Err() == nil || m.screen != configure {
		t.Fatal("a dismissed chat lookup changed the form or left a late worker running")
	}
}

func TestChatLookupFailureKeepsPartialChoicesAndCanBeReloaded(t *testing.T) {
	m := chatChoiceModel(t)
	j := &job{terminal: newTranscript(80, 24), done: true, streamSeen: true, err: errors.New("failed"), items: []item{{ID: "42", Title: "Received chat"}}, cancel: func() {}}
	j.terminal.write([]byte("connection refused\n"))
	m.run = func(context.Context, Plan) tea.Cmd { return func() tea.Msg { return startedMsg{j} } }
	m = startChatChoiceTest(t, m)
	if m.chatLookup != nil || m.picker.loading || m.picker.label("42") != "Received chat" || !strings.Contains(ansi.Strip(m.View().Content), "connection refused") {
		t.Fatal("a failed lookup should retain received chats and show its error")
	}
	next, command := m.Update(key('l', "", tea.ModCtrl))
	m = next.(Model)
	if command == nil || !m.picker.loading || m.picker.loadErr != "" {
		t.Fatal("Reload did not start a fresh lookup and clear the previous error")
	}
	m.closePicker()
}

func TestChatChoicesAllowManualUsernamesAndClearIDsOnAccountChange(t *testing.T) {
	m := chatChoiceModel(t)
	set(&m.form, "chat", "me,42")
	m.form.field("chat").ChoiceLabels["42"] = "Old account chat"
	focusField(&m, "ns")
	m.openPicker(*m.form.field("ns"))
	m.picker.selected = map[string]bool{"work": true}
	m.applyPicker()
	if m.form.field("chat").Value != "me" || slices.Contains(m.form.field("chat").Choices, "42") {
		t.Fatal("changing accounts must clear account-specific chat IDs")
	}
	m.run = func(context.Context, Plan) tea.Cmd { return nil }
	focusField(&m, "chat")
	m = update(m, key(tea.KeyEnter, ""))
	m = update(m, tea.PasteMsg{Content: "@manual_chat"})
	m = update(m, key(tea.KeyEnter, ""))
	m = update(m, key(tea.KeyLeft, ""))
	if m.picker != nil || m.form.field("chat").Value != "me,@manual_chat" {
		t.Fatal("chat names should remain addable when a lookup is unavailable")
	}
}

func TestSinceUntilAreVisibleAndAcceptDateTimes(t *testing.T) {
	root, action := fixture()
	action.Command.Flags().String("until", "", "exclusive date/time")
	m := New(Catalog(root), nil, "personal", t.Context())
	m.open(action)
	for _, name := range []string{"since", "until"} {
		focusField(&m, name)
		if m.focus < 0 {
			t.Fatalf("%s must be visible without More options", name)
		}
	}
	focusField(&m, "since")
	m = update(m, key(tea.KeyEnter, ""))
	m = update(m, tea.PasteMsg{Content: "2026-11-01T18:30:00+03:30"})
	m = update(m, key(tea.KeyTab, ""))
	m = update(m, key(tea.KeyEnter, ""))
	m = update(m, tea.PasteMsg{Content: "2026-11-14T00:00:00Z"})
	m = update(m, key('r', "", tea.ModCtrl))
	if m.screen != review || !slices.Contains(m.plan.Args, "--since=2026-11-01T18:30:00+03:30") || !slices.Contains(m.plan.Args, "--until=2026-11-14T00:00:00Z") {
		t.Fatal("Since and Until date/time inputs were not saved into the backup plan")
	}
}

func TestOldBrowserChatNamesAreNotReusedForDifferentStorage(t *testing.T) {
	m := chatChoiceModel(t)
	m.source = &resultSnapshot{plan: Plan{Args: []string{"--ns=personal", "--storage=type=bolt,path=old"}}, data: resultData{Kind: "chat ls", Items: []item{{ID: "42", Title: "Old session chat"}}}}
	set(&m.form, "storage", "type=bolt,path=new")
	m.configureChatField()
	if slices.Contains(m.form.field("chat").Choices, "42") {
		t.Fatal("a browser snapshot from different session storage leaked into the chat field")
	}
}

func TestStopLoadingButtonKeepsTheChatSelectorUsable(t *testing.T) {
	m := chatChoiceModel(t)
	stopped := false
	j := &job{terminal: newTranscript(80, 24), streamSeen: true, items: []item{{ID: "42", Title: "Received chat"}}, cancel: func() { stopped = true }}
	m.run = func(context.Context, Plan) tea.Cmd { return func() tea.Msg { return startedMsg{j} } }
	m = startChatChoiceTest(t, m)
	m = clickChoiceText(t, m, "[Stop loading]")
	if !stopped || m.chatLookup != nil || m.picker == nil || m.picker.loading || m.picker.label("42") != "Received chat" {
		t.Fatal("Stop loading must retain received chats without leaving the form")
	}
	m = clickChoiceText(t, m, "[ ] Received chat")
	m = clickChoiceText(t, m, "[Done]")
	if m.form.field("chat").Value != "42" || m.picker != nil {
		t.Fatal("a received chat could not be selected after stopping the lookup")
	}
}

func TestChatFieldLoadsNamesWithoutOpeningTheChatBrowserFirst(t *testing.T) {
	m := chatChoiceModel(t)
	set(&m.form, "proxy", "socks5://example.invalid:1080")
	set(&m.form, "since", "unfinished-date")
	j := &job{terminal: newTranscript(80, 24), structured: true, streamSeen: true, started: time.Now()}
	stopped := false
	j.cancel = func() { stopped = true }
	var lookupPlan Plan
	var lookupContext context.Context
	m.run = func(ctx context.Context, plan Plan) tea.Cmd {
		lookupPlan, lookupContext = plan, ctx
		return func() tea.Msg { return startedMsg{j} }
	}
	focusField(&m, "chat")
	next, command := m.Update(key(tea.KeyEnter, ""))
	if command == nil {
		t.Fatal("opening Chats to back up directly must start loading chats")
	}
	next, poll := next.(Model).Update(command())
	m = next.(Model)
	if poll == nil || !slices.Equal(lookupPlan.Path, []string{"chat", "ls"}) || !slices.Contains(lookupPlan.Args, "--ns=personal") || !slices.Contains(lookupPlan.Args, "--proxy=socks5://example.invalid:1080") {
		t.Fatal("the chat selector did not look up chats for the form's account and connection")
	}
	j.mu.Lock()
	j.items = []item{{ID: "42", Title: "Friends"}, {ID: "84", Title: "Work"}}
	j.stage = "2 chats received"
	j.mu.Unlock()
	m = update(m, poll())
	if m.screen != configure || m.picker == nil || m.picker.label("42") != "Friends" || m.picker.label("84") != "Work" {
		t.Fatal("streamed chat names were not shown in the original form")
	}
	m = update(m, key(tea.KeyEnd, ""))
	m = update(m, key(tea.KeyEnter, ""))
	m = update(m, key(tea.KeyUp, ""))
	m = update(m, key(tea.KeyEnter, ""))
	m = update(m, key(tea.KeyLeft, ""))
	if m.picker != nil || m.form.field("chat").Value != "42,84" || !stopped || lookupContext.Err() == nil {
		t.Fatal("accepting chat choices must save IDs and stop the unfinished lookup")
	}
	set(&m.form, "since", "")
	plan, err := m.form.Plan(nil)
	if err != nil || !slices.Contains(plan.Args, "--chat=42,84") {
		t.Fatalf("selected chat IDs did not reach the backup plan: %v", err)
	}
}
