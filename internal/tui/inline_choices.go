package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func multiChoices(field Field) bool { return field.Multi && choiceField(field) }
func customChoices(field Field) bool {
	return field.Multi && (field.AllowCustom || len(field.Choices) == 0) && field.PathKind == "" && field.Type != "accounts" && !field.Secret
}
func choiceField(field Field) bool {
	return len(field.Choices) > 0 || field.Type == "accounts" || customChoices(field)
}
func (m Model) inlinePicker() bool { return m.screen == configure && m.picker != nil }

type choiceBounds struct{ x, y, width, height int }

func (b choiceBounds) contains(x, y int) bool {
	return x >= b.x && x < b.x+b.width && y >= b.y && y < b.y+b.height
}

// The dropdown is attached to the argument's value column and has its own
// bounded viewport. Rendering, mouse hit testing and wheel routing share it.
func (m Model) inlineChoiceBounds() choiceBounds {
	g := m.formLayout(m.width-4, m.height-9)
	return choiceBounds{2 + g.controlX, 3 + g.top + g.fieldRow(m.focus) + 1, min(60, g.controlWidth), g.extra}
}

type choiceWindow struct{ start, end, rows, thumbStart, thumbSize int }

func choiceListWindow(height, count, cursor, offset int) choiceWindow {
	w := choiceWindow{rows: max(1, height-3)}
	cursor = max(0, min(max(0, count-1), cursor))
	w.start = max(0, min(max(0, count-w.rows), offset))
	if cursor < w.start {
		w.start = cursor
	} else if cursor >= w.start+w.rows {
		w.start = cursor - w.rows + 1
	}
	w.end = min(count, w.start+w.rows)
	w.thumbSize = max(1, w.rows*w.rows/max(1, count))
	w.thumbStart = w.start * (w.rows - w.thumbSize) / max(1, count-w.rows)
	return w
}

type choiceAction struct{ id, label string }

func inlineChoiceActions(p *choicePicker) []choiceAction {
	if len(p.choices) == 0 && p.field == "ns" {
		return []choiceAction{{"picker-connect", "Connect account"}, {"picker-refresh", "Refresh"}}
	}
	if !p.multi {
		return []choiceAction{{"picker-apply", "Done"}}
	}
	if p.custom {
		return []choiceAction{{"picker-add", "Add"}, {"picker-all", "All"}, {"picker-clear", "Clear"}, {"picker-apply", "Done"}}
	}
	return []choiceAction{{"picker-all", "All"}, {"picker-clear", "Clear"}, {"picker-apply", "Done"}}
}

func (m Model) choiceToolbar(field Field, width int) string {
	p := m.picker
	var actions []string
	for _, action := range inlineChoiceActions(p) {
		actions = append(actions, selectedStyle.Render("["+action.label+"]"))
	}
	toolbar := strings.Join(actions, " ")
	values := p.values()
	if p.multi {
		field.Value = csvString(values)
	}
	selection := field.Display()
	if p.multi && len(values) > 1 {
		selection = fmt.Sprintf("%d · %s", len(values), selection)
	}
	if lipgloss.Width(toolbar)+lipgloss.Width(selection)+2 > width && p.multi {
		selection = fmt.Sprintf("%d ✓", len(p.values()))
	}
	selection = clip(selection, max(1, width-lipgloss.Width(toolbar)-1))
	space := max(1, width-lipgloss.Width(toolbar)-lipgloss.Width(selection))
	return clip(accent.Render(selection)+strings.Repeat(" ", space)+toolbar, width)
}

func (m Model) inlineChoicesView(_ Field, width, height int) string {
	p := m.picker
	rows := p.filtered()
	w := choiceListWindow(height, len(rows), p.cursor, p.offset)
	inner := max(1, width-2)
	border := line
	if !p.searching {
		border = violet
	}
	frame := lipgloss.NewStyle().Foreground(border)
	query := p.query
	query.SetWidth(max(1, inner-3))
	lines := []string{frame.Render("╭" + strings.Repeat("─", inner) + "╮"), frame.Render("│") + fit(dim.Render("⌕ ")+query.View(), inner, 1) + frame.Render("│")}
	for row := range w.rows {
		index := w.start + row
		value := ""
		style := textStyle
		if index < len(rows) {
			mark, pointer := "○", " "
			if p.multi {
				mark = "[ ]"
			}
			if p.checked(rows[index]) {
				mark, style = "●", accent
				if p.multi {
					mark = "[✓]"
				}
			}
			if index == p.cursor && !p.searching {
				pointer, style = "▸", selectedStyle.Bold(true)
			}
			value = pointer + " " + mark + " " + p.label(rows[index])
		} else if row == 0 && len(rows) == 0 {
			value, style = "No matching options", dim
			if p.custom {
				value = "Type an item; Enter adds it"
				if p.query.Value() != "" {
					value = "+ Add " + safeText(p.query.Value()) + " ↵"
				}
			} else if p.selectedOnly {
				value = "No selected items"
			}
			if len(p.choices) == 0 && p.field == "ns" {
				value = "No saved accounts"
			}
		}
		track := " "
		if len(rows) > w.rows {
			track = dim.Render("│")
			if row >= w.thumbStart && row < w.thumbStart+w.thumbSize {
				track = accent.Render("┃")
			}
		}
		lines = append(lines, frame.Render("│")+style.Width(inner-1).Render(clip(value, inner-1))+track+frame.Render("│"))
	}
	page := ""
	if len(rows) > 0 {
		page = fmt.Sprintf(" %d–%d / %d ", w.start+1, w.end, len(rows))
	}
	page = clip(page, inner)
	lines = append(lines, frame.Render("╰"+page+strings.Repeat("─", max(0, inner-lipgloss.Width(page)))+"╯"))
	return fit(strings.Join(lines, "\n"), width, height)
}

func (m *Model) inlineChoiceClick(msg tea.MouseClickMsg) (bool, tea.Cmd) {
	if !m.inlinePicker() {
		return false, nil
	}
	if m.picker.field == "chat" && chatChoiceAction(m.form.Action.Path) && msg.Y == m.height-8 && msg.X >= 2 && msg.X < 16 {
		if m.picker.loading {
			m.stopChatLookup()
			return true, nil
		}
		return true, m.loadChatChoices(true)
	}
	b := m.inlineChoiceBounds()
	x, y := msg.X-b.x, msg.Y-b.y
	if msg.Y == b.y-1 && x >= 0 && x < b.width {
		start := b.width
		actions := inlineChoiceActions(m.picker)
		for index := len(actions) - 1; index >= 0; index-- {
			action := actions[index]
			size := lipgloss.Width(action.label) + 2
			start -= size
			if x >= start && x < start+size {
				if action.id == "picker-refresh" {
					return true, m.loadAccounts()
				}
				return true, m.activate(action.id)
			}
			start--
		}
		return true, nil
	}
	if !b.contains(msg.X, msg.Y) {
		return false, nil
	}
	p := m.picker
	if y == 1 {
		p.searching = true
		p.query.Focus()
		return true, nil
	}
	rows := p.filtered()
	if p.custom && len(rows) == 0 && y == 2 && p.query.Value() != "" {
		p.addCustom()
		return true, nil
	}
	w := choiceListWindow(b.height, len(rows), p.cursor, p.offset)
	index := w.start + y - 2
	if y >= 2 && y < b.height-1 && index < len(rows) {
		if x == b.width-2 && len(rows) > w.rows {
			p.cursor = (y - 2) * (len(rows) - 1) / max(1, w.rows-1)
			return true, nil
		}
		p.cursor, p.searching = index, false
		p.query.Blur()
		p.toggle(rows[index])
		if !p.multi {
			m.applyPicker()
		}
	}
	return true, nil
}
