package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

type formGeometry struct {
	top, rows, start, focus, extra     int
	labelWidth, controlX, controlWidth int
}

func (m Model) formLayout(width, height int) formGeometry {
	g := formGeometry{top: 3, rows: max(1, height-6), focus: m.focus, labelWidth: min(27, width/3)}
	g.controlX = g.labelWidth + 2
	g.controlWidth = width - g.controlX
	fields := m.visibleFields()
	if m.inlinePicker() {
		g.extra = min(7, max(1, g.rows-1))
	} else if m.inlineCalendar() {
		g.extra = calendarHeight
	} else if len(fields) > 0 && m.focus < len(fields) && (m.form.Fields[fields[m.focus]].PathKind != "" || dateField(m.form.Fields[fields[m.focus]])) {
		g.extra = 1
	}
	g.start = max(0, m.focus-max(1, g.rows-g.extra)+1)
	return g
}

func (g formGeometry) fieldRow(index int) int {
	row := index - g.start
	if index > g.focus {
		row += g.extra
	}
	return row
}

func (g formGeometry) fieldAtRow(row int) (int, bool) {
	if row < 0 || row >= g.rows {
		return 0, false
	}
	focusRow := g.focus - g.start
	if row > focusRow && row <= focusRow+g.extra {
		return 0, false
	}
	if row > focusRow {
		row -= g.extra
	}
	return g.start + row, true
}

func (m Model) formView(width, height int) string {
	fields := m.visibleFields()
	g := m.formLayout(width, height)
	mode := "Essentials"
	if m.advanced {
		mode = "All settings"
	}
	status := fmt.Sprintf("%s · %d settings", mode, len(fields))
	if m.searching {
		status = m.inputView(width, false)
	} else if m.fieldSearch != "" {
		status = fmt.Sprintf("Options matching %q · %d found", m.fieldSearch, len(fields))
	}
	lines := []string{titleStyle.Render(clip(m.form.Action.Title, width)), dim.Render(clip(status+" · Ctrl+F to find any option", width)), ""}
	if len(fields) == 0 {
		return strings.Join(append(lines, "No matching settings. Esc clears the search."), "\n")
	}
	var rows []string
	for n := g.start; n < len(fields) && g.fieldRow(n) < g.rows; n++ {
		field := m.form.Fields[fields[n]]
		marker, style := "  ", dim
		if n == m.focus {
			marker, style = "▸ ", accent
		}
		label := field.Label
		if field.Required {
			label += " *"
		}
		name := style.Width(g.labelWidth).Render(clip(marker+label, g.labelWidth))
		value := dim.Render(clip(field.Display(), g.controlWidth-2))
		if n == m.focus {
			value = m.fieldControl(field, g.controlWidth)
			if m.inlinePicker() {
				value = m.choiceToolbar(field, min(60, g.controlWidth))
			} else if m.inlineCalendar() {
				value = calendarToolbar()
			}
		}
		rows = append(rows, name+"  "+value)
		if n == m.focus && g.extra > 0 {
			control := ""
			if m.inlinePicker() {
				control = m.inlineChoicesView(field, min(60, g.controlWidth), g.extra)
			} else if m.inlineCalendar() {
				control = m.calendarView()
			} else if dateField(field) {
				control = selectedStyle.Render("[ Calendar ]") + " " + dim.Render("[ Type date/time ]")
			} else if field.PathKind != "" {
				control = selectedStyle.Render("[ Browse… ]") + " " + dim.Render("[ Type path ]")
			}
			for _, row := range strings.Split(control, "\n") {
				rows = append(rows, strings.Repeat(" ", g.controlX)+row)
			}
		}
	}
	lines = append(lines, fit(strings.Join(rows, "\n"), width, g.rows))
	field := m.form.Fields[fields[m.focus]]
	lines = append(lines, dim.Render(clip(m.fieldHelp(field), width)), dim.Render(clip(m.fieldHint(field), width)))
	err := m.err
	if m.inlinePicker() {
		if m.picker.err != "" {
			err = m.picker.err
		} else if m.picker.loadErr != "" {
			err = m.picker.loadErr
		} else if m.picker.field == "ns" && m.accountError != "" {
			err = "Could not refresh saved accounts: " + m.accountError
		}
	}
	if err != "" {
		lines = append(lines, warning.Render(clip(err, width)))
	} else {
		lines = append(lines, dim.Render(fmt.Sprintf("Setting %d of %d", m.focus+1, len(fields)))+"  "+accent.Render("Ctrl+R  Review"))
	}
	return strings.Join(lines, "\n")
}

func (m Model) fieldControl(field Field, width int) string {
	if choiceField(field) {
		width = min(60, width)
	}
	value := field.Display()
	switch {
	case field.Type == "bool":
		value = accent.Render("● Off") + dim.Render("    ○ On")
		if field.Value == "true" {
			value = dim.Render("○ Off") + accent.Render("    ● On")
		}
	case choiceField(field):
		value = accent.Render(field.Display()) + dim.Render("  ▾")
		if field.Multi {
			value = accent.Render(field.Display()) + dim.Render("  ▾ multiple")
		}
	case m.editing:
		value = m.inputView(max(1, width-2), field.Secret)
	case dateField(field):
		if value == "" || value == "—" {
			value = "Choose a date"
		}
		value = accent.Render(value) + dim.Render("  ▾")
	case field.Value == "":
		value = dim.Render(placeholder(field))
	}
	style := lipgloss.NewStyle().Background(panel).Foreground(ink).Padding(0, 1).Width(width)
	return style.Render(clip(value, width-2))
}

func (m Model) fieldHelp(field Field) string {
	if field.Multi && choiceField(field) {
		if m.inlinePicker() && m.picker.field == field.Name {
			field.Value = csvString(m.picker.values())
		}
		if field.Value != "" {
			return "Selected: " + field.Display()
		}
	}
	if field.Name == "format" {
		return "Choose JSON, HTML, or Both. Both exports JSON and HTML."
	}
	if field.Name == "since" {
		return "Include data from this date/time (inclusive). Use YYYY-MM-DD, RFC3339 with a time zone, or Unix seconds."
	}
	if field.Name == "until" {
		return "Include data before this date/time (exclusive). Use YYYY-MM-DD, RFC3339 with a time zone, or Unix seconds."
	}
	if field.Name == "chat" {
		if len(field.Choices) > 0 {
			return "Choose the chats to back up from this account. Your choices stay checked while filtering."
		}
		return "Use me for Saved Messages, an @username or a chat ID. You can also choose a chat from Browse your chats."
	}
	return field.Help
}

func (m Model) fieldHint(field Field) string {
	if dateField(field) {
		if m.calendar != nil {
			return "Click a day or Enter to select · PgUp/PgDn changes month · Esc cancels"
		}
		if m.editing {
			return "Type a date/time · Calendar opens the picker · Tab saves · Esc undoes"
		}
		return "Enter or Calendar opens the picker · Type date/time for an exact timestamp"
	}
	if m.inlinePicker() && field.Name == "chat" && chatChoiceAction(m.form.Action.Path) {
		if m.picker.loading {
			return selectedStyle.Render("[Stop loading]") + " " + m.picker.loadStage
		}
		return selectedStyle.Render("[Reload chats]") + " Enter toggles · ← saves · Ctrl+L reloads"
	}
	if field.PathKind != "" {
		if m.editing {
			return "Alt+O browses · Tab saves · Esc undoes this edit"
		}
		return "Enter or Browse opens the chooser · Type a path for a new destination"
	}
	if customChoices(field) {
		return "Enter adds a typed item or toggles a choice · ← saves and closes · Add/Done work with the mouse"
	}
	if field.Multi && choiceField(field) {
		if m.inlinePicker() {
			return "Enter or Space toggles an item · ← saves and closes · Done also saves · Ctrl+S shows chosen items"
		}
		return "Enter or click opens choices · Type to filter · Tab next"
	}
	if len(field.Choices) > 0 {
		if m.inlinePicker() {
			return "↑ / ↓ or wheel scrolls · Enter selects · Tab next"
		}
		return "Enter opens the dropdown · ← / → changes the selection"
	}
	if field.Type == "bool" {
		return "← / → or Space changes the selection · Tab next"
	}
	if m.editing {
		return "Tab saves and moves on · Esc undoes this edit · Ctrl+U clears"
	}
	return "Start typing, paste, or press Enter to edit · Tab next"
}
