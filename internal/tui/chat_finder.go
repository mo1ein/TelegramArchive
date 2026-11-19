package tui

import (
	"fmt"
	"strings"
)

func (m Model) chatFinder() bool {
	return m.screen == configure && m.picker != nil && m.picker.field == "chat" && chatChoiceAction(m.form.Action.Path)
}

func (m Model) choiceQueryWidth() int {
	if m.chatFinder() {
		return max(1, m.width-9)
	}
	return max(1, min(60, m.formLayout(m.width-4, m.height-9).controlWidth)-5)
}

func (m Model) chatFinderView(width, height int) string {
	p := m.picker
	heading := titleStyle.Render("Choose chats to back up") + dim.Render(fmt.Sprintf("  ·  %d selected", len(p.values())))
	field := *m.form.field("chat")
	status := m.fieldHint(field)
	if p.err != "" {
		status = warning.Render(p.err)
	} else if p.loadErr != "" {
		status = warning.Render(p.loadErr)
	}
	return strings.Join([]string{
		clip(heading, width),
		m.choiceToolbar(field, width),
		m.inlineChoicesView(field, width, height-3),
		clip(status, width),
	}, "\n")
}

// Match an abbreviated sequence, like "bkcl" for "Book Club", without
// changing the stable order of a streaming list or its checked items.
func fuzzyChatMatch(value, query string) bool {
	wanted := []rune(strings.ToLower(strings.Join(strings.Fields(query), "")))
	if len(wanted) == 0 {
		return true
	}
	index := 0
	for _, char := range strings.ToLower(value) {
		if char == wanted[index] {
			index++
			if index == len(wanted) {
				return true
			}
		}
	}
	return false
}
