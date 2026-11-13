package tui

import (
	"context"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type Option func(*Model)
type AccountLoader func(context.Context, string) ([]string, error)

func WithAccountLoader(loader AccountLoader) Option {
	return func(m *Model) { m.accountLoader = loader }
}

func WithSavedAccounts(names []string) Option {
	return func(m *Model) {
		m.accounts = slices.Clone(names)
		m.accountsLoaded = true
	}
}

type accountsLoadedMsg struct {
	names []string
	err   error
}

func (m Model) loadAccounts() tea.Cmd {
	if m.accountLoader == nil {
		return nil
	}
	loader, ctx, storage := m.accountLoader, m.ctx, m.globals["storage"]
	if field := m.form.field("storage"); field != nil && field.Dirty {
		storage = field.Value
	}
	return func() tea.Msg {
		names, err := loader(ctx, storage)
		return accountsLoadedMsg{names, err}
	}
}

func backupAction(path []string) bool {
	return slices.Contains([]string{"archive", "account", "sync"}, strings.Join(path, " "))
}

func (m *Model) configureAccountField() {
	field := m.form.field("ns")
	if field == nil || strings.Join(m.form.Action.Path, " ") == "login" {
		return
	}
	field.Choices = slices.Clone(m.accounts)
	if !backupAction(m.form.Action.Path) {
		if field.Value != "" && !slices.Contains(field.Choices, field.Value) {
			field.Choices = append(field.Choices, field.Value)
		}
		return
	}
	if field.Type != "accounts" {
		selected := slices.Clone(m.accountSelection)
		if len(selected) == 0 {
			selected = []string{field.Value}
		}
		field.Value = csvString(selected)
	}
	field.Type, field.Label, field.Multi, field.Advanced, field.Dirty = "accounts", "Accounts to back up", true, false, true
	field.Help = "Choose one or more saved accounts. Each account in a multi-account backup gets its own folder."
	if m.accountsLoaded && m.accountError == "" {
		selected, _ := readCSV(field.Value)
		selected = slices.DeleteFunc(selected, func(name string) bool { return !slices.Contains(m.accounts, name) })
		if len(selected) == 0 && len(m.accounts) > 0 {
			selected = []string{m.accounts[0]}
		}
		field.Value = csvString(selected)
	}
}

func (m *Model) configureChatField() {
	field := m.form.field("chat")
	if field == nil || !chatChoiceAction(m.form.Action.Path) {
		return
	}
	field.AllowCustom = true
	field.Choices = []string{"me"}
	field.ChoiceLabels = map[string]string{"me": "Saved Messages"}
	selected, _ := readCSV(field.Value)
	for _, value := range selected {
		if !slices.Contains(field.Choices, value) {
			field.Choices = append(field.Choices, value)
		}
	}
	if m.source == nil || m.source.data.Kind != "chat ls" {
		return
	}
	account, err := m.chatChoiceAccount()
	if err != nil || planAccount(m.source.plan, m.globals["ns"]) != account || !m.sourceChatConnectionMatches() {
		return
	}
	for _, row := range m.source.data.Items {
		if !slices.Contains(field.Choices, row.ID) {
			field.Choices = append(field.Choices, row.ID)
		}
		field.ChoiceLabels[row.ID] = row.Title
	}
}
