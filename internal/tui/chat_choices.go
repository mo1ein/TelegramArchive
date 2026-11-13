package tui

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type chatLookup struct {
	picker *choicePicker
	plan   Plan
	ctx    context.Context
	cancel context.CancelFunc
	job    *job
	count  int
}

type chatLookupStartedMsg struct {
	lookup *chatLookup
	job    *job
	err    error
}

type chatLookupPollMsg struct{ lookup *chatLookup }

func chatChoiceAction(path []string) bool {
	return slices.Contains([]string{"archive", "sync", "cron add"}, strings.Join(path, " "))
}

func planAccount(plan Plan, fallback string) string {
	return planSetting(plan, "ns", fallback)
}

func planSetting(plan Plan, name, fallback string) string {
	for _, arg := range plan.Args {
		if value, ok := strings.CutPrefix(arg, "--"+name+"="); ok {
			return value
		}
	}
	return fallback
}

func (m Model) sourceChatConnectionMatches() bool {
	if m.source == nil {
		return false
	}
	for _, name := range []string{"storage", "proxy"} {
		initial := planSetting(Plan{Args: m.base}, name, "")
		current := initial
		if field := m.form.field(name); field != nil && field.Dirty {
			current = field.Value
		}
		if planSetting(m.source.plan, name, initial) != current {
			return false
		}
	}
	return true
}

func (m Model) chatChoiceAccount() (string, error) {
	field := m.form.field("ns")
	if field == nil {
		return m.globals["ns"], nil
	}
	if !field.Multi {
		return field.Value, nil
	}
	names, err := readCSV(field.Value)
	if err != nil || len(names) == 0 {
		return "", fmt.Errorf("choose a saved account to load its chats")
	}
	if len(names) != 1 {
		return "", fmt.Errorf("select one account to browse its chats; for several accounts, use all chats or add chat names manually")
	}
	return names[0], nil
}

// Listing chats only needs the current account and connection settings. An
// unfinished local backup setting must not prevent browsing available chats.
func (m Model) chatChoicePlan() (Plan, error) {
	account, err := m.chatChoiceAccount()
	if err != nil {
		return Plan{}, err
	}
	for _, action := range m.actions {
		if strings.Join(action.Path, " ") != "chat ls" {
			continue
		}
		form := NewForm(action, m.globals)
		for _, source := range m.form.Fields {
			if !source.Global {
				continue
			}
			if target := form.field(source.Name); target != nil {
				target.Value, target.Dirty = source.Value, source.Dirty
			}
		}
		if namespace := form.field("ns"); namespace != nil {
			namespace.Value, namespace.Dirty = account, true
		}
		plan, err := form.Plan(m.base)
		plan.width, plan.height = max(20, m.width-6), max(5, m.height-17)
		return plan, err
	}
	return Plan{}, errNoChatBrowser
}

var errNoChatBrowser = errors.New("chat browser is unavailable in this build")

func (m *Model) stopChatLookup() {
	if lookup := m.chatLookup; lookup != nil {
		lookup.cancel()
		if lookup.job != nil && lookup.job.cancel != nil {
			lookup.job.cancel()
		}
		lookup.picker.loading = false
		m.chatLookup = nil
	}
}

func (m *Model) closePicker() {
	m.stopChatLookup()
	m.picker = nil
}

func (m *Model) resetChatSelection() {
	m.stopChatLookup()
	m.chatChoiceKey = [32]byte{}
	if field := m.form.field("chat"); field != nil && chatChoiceAction(m.form.Action.Path) {
		selected, _ := readCSV(field.Value)
		selected = slices.DeleteFunc(selected, func(value string) bool { return value != "me" })
		field.Value, field.Dirty = csvString(selected), true
		m.configureChatField()
	}
	if m.picker != nil && m.picker.field == "chat" {
		m.closePicker()
	}
}

func (m *Model) loadChatChoices(refresh bool) tea.Cmd {
	p := m.picker
	if p == nil || p.field != "chat" || !chatChoiceAction(m.form.Action.Path) {
		return nil
	}
	m.stopChatLookup()
	p.loadErr = ""
	plan, err := m.chatChoicePlan()
	if err != nil {
		if !errors.Is(err, errNoChatBrowser) {
			p.loadErr = safeText(err.Error())
		}
		return nil
	}
	key := chatCacheKey(plan)
	if key != m.chatChoiceKey {
		m.chatChoiceKey = key
		m.configureChatField()
		field := m.form.field("chat")
		p.choices, p.labels = slices.Clone(field.Choices), maps.Clone(field.ChoiceLabels)
		p.cursor, p.offset = 0, 0
	}
	if cached, ok := m.chatCache[key]; ok && !refresh {
		m.mergeChatChoices(cached.data.Items)
		return nil
	}
	ctx, cancel := context.WithTimeout(m.ctx, 95*time.Second)
	lookup := &chatLookup{picker: p, plan: plan, ctx: ctx, cancel: cancel}
	m.chatLookup = lookup
	p.loading, p.loadStage = true, "Loading chats for "+safeText(planAccount(plan, "the selected account"))+"…"
	command := m.run(ctx, plan)
	if command == nil {
		m.stopChatLookup()
		p.loadErr = "Chat loading is unavailable. You can add names or IDs manually."
		return nil
	}
	return func() tea.Msg {
		switch msg := command().(type) {
		case startedMsg:
			return chatLookupStartedMsg{lookup: lookup, job: msg.job}
		case finishedMsg:
			return chatLookupStartedMsg{lookup: lookup, err: msg.err}
		default:
			return chatLookupStartedMsg{lookup: lookup, err: fmt.Errorf("could not start chat loading")}
		}
	}
}

func chatLookupPoll(lookup *chatLookup) tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return chatLookupPollMsg{lookup} })
}

func (m *Model) chatLookupStarted(msg chatLookupStartedMsg) tea.Cmd {
	if m.chatLookup != msg.lookup || m.picker != msg.lookup.picker {
		if msg.job != nil && msg.job.cancel != nil {
			msg.job.cancel()
		}
		msg.lookup.cancel()
		return nil
	}
	if msg.err != nil || msg.job == nil {
		msg.lookup.picker.loadErr = "Could not load chats."
		if msg.err != nil {
			msg.lookup.picker.loadErr = safeText(msg.err.Error())
		}
		m.stopChatLookup()
		return nil
	}
	msg.lookup.job = msg.job
	return chatLookupPoll(msg.lookup)
}

func (m *Model) pollChatChoices(lookup *chatLookup) tea.Cmd {
	if m.chatLookup != lookup || m.picker != lookup.picker || lookup.job == nil {
		return nil
	}
	j := lookup.job
	j.mu.Lock()
	count := len(j.items)
	var rows []item
	if count != lookup.count || j.done {
		rows = slices.Clone(j.items)
	}
	stage, done, err, overflow := j.stage, j.done, j.err, j.overflow
	output := ""
	if done && err != nil {
		output = redactOutput(j.terminal.text())
	}
	j.mu.Unlock()
	if count != lookup.count {
		m.mergeChatChoices(rows)
		lookup.count = count
	}
	if stage != "" {
		lookup.picker.loadStage = stage
	}
	if !done {
		return chatLookupPoll(lookup)
	}
	if err != nil || overflow {
		message := "Chat loading failed."
		if err != nil {
			message = err.Error()
		}
		if output != "" {
			lines := strings.Split(strings.TrimSpace(output), "\n")
			message = lines[len(lines)-1]
		}
		if overflow {
			message = "Only part of the chat list could be loaded. Reload or add a chat manually."
		} else if errors.Is(lookup.ctx.Err(), context.DeadlineExceeded) {
			message = "Chat loading timed out. Check your connection or proxy and reload."
		}
		lookup.picker.loadErr = safeText(message)
	} else {
		m.cacheChats(lookup.plan, resultData{Kind: "chat ls", Items: rows})
	}
	m.stopChatLookup()
	return nil
}

func (m *Model) mergeChatChoices(rows []item) {
	field := m.form.field("chat")
	if field == nil {
		return
	}
	known := map[string]bool{}
	for _, id := range field.Choices {
		known[id] = true
	}
	if field.ChoiceLabels == nil {
		field.ChoiceLabels = map[string]string{}
	}
	for _, row := range rows {
		if !known[row.ID] {
			field.Choices = append(field.Choices, row.ID)
			known[row.ID] = true
		}
		field.ChoiceLabels[row.ID] = safeText(row.Title)
	}
	if p := m.picker; p != nil && p.field == "chat" {
		listed := make(map[string]bool, len(p.choices))
		for _, id := range p.choices {
			listed[id] = true
		}
		for _, id := range field.Choices {
			if !listed[id] {
				p.choices = append(p.choices, id)
				listed[id] = true
			}
		}
		p.labels = maps.Clone(field.ChoiceLabels)
	}
}
