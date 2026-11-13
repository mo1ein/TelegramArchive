package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const calendarWidth, calendarHeight = 30, 9

type datePicker struct {
	field string
	day   time.Time
}

func dateField(field Field) bool { return field.Name == "since" || field.Name == "until" }

func calendarDay(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func (m *Model) openDatePicker(field Field) {
	day := calendarDay(time.Now())
	value := field.Value
	if m.editing {
		value = m.editor.Value()
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			day = calendarDay(parsed)
			break
		}
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		day = calendarDay(time.Unix(seconds, 0).UTC())
	}
	m.calendar = &datePicker{field: field.Name, day: day}
	m.editing, m.buttonsFocused, m.err = false, false, ""
	m.buttonFocus = 0
	m.editor.Blur()
}

func (m *Model) closeDatePicker() {
	m.calendar, m.buttonsFocused, m.buttonFocus = nil, false, 0
}

func (m *Model) applyDatePicker(value string) {
	if m.calendar == nil {
		return
	}
	if field := m.form.field(m.calendar.field); field != nil {
		field.Value, field.Dirty = value, true
	}
	m.closeDatePicker()
	m.err = ""
}

func (m *Model) typeDate() {
	m.closeDatePicker()
	m.editField()
}

func (p *datePicker) moveMonth(delta int) {
	first := time.Date(p.day.Year(), p.day.Month()+time.Month(delta), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	p.day = first.AddDate(0, 0, min(p.day.Day(), last)-1)
}

// Every month uses six rows, so controls stay under the pointer when paging.
func (p datePicker) firstCell() time.Time {
	first := time.Date(p.day.Year(), p.day.Month(), 1, 0, 0, 0, 0, time.UTC)
	return first.AddDate(0, 0, -(int(first.Weekday())+6)%7)
}

func (m Model) inlineCalendar() bool {
	return m.screen == configure && m.calendar != nil && m.height >= 25
}

func (m Model) calendarBounds() choiceBounds {
	if m.inlineCalendar() {
		g := m.formLayout(m.width-4, m.height-9)
		return choiceBounds{2 + g.controlX, 3 + g.top + g.fieldRow(m.focus) + 1, calendarWidth, calendarHeight}
	}
	return choiceBounds{2, 5, calendarWidth, calendarHeight}
}

func (m Model) calendarView() string {
	p := m.calendar
	frame := lipgloss.NewStyle().Foreground(violet)
	heading := p.day.Format("January 2006")
	inner := calendarWidth - 2
	space := calendarWidth - len(heading) - 6
	lines := []string{
		frame.Render("╭ ") + selectedStyle.Render("‹") + strings.Repeat(" ", space/2) + titleStyle.Render(heading) + strings.Repeat(" ", space-space/2) + selectedStyle.Render("›") + frame.Render(" ╮"),
		frame.Render("│") + dim.Render(" Mo  Tu  We  Th  Fr  Sa  Su ") + frame.Render("│"),
	}
	today := calendarDay(time.Now())
	first := p.firstCell()
	for week := range 6 {
		var row strings.Builder
		row.WriteString(frame.Render("│"))
		for column := range 7 {
			day := first.AddDate(0, 0, week*7+column)
			style := textStyle
			if day.Month() != p.day.Month() {
				style = dim
			}
			if day.Equal(today) {
				style = accent.Underline(true)
			}
			if day.Equal(p.day) {
				style = lipgloss.NewStyle().Background(mint).Foreground(lipgloss.Color("#102C2C")).Bold(true)
			}
			row.WriteString(style.Render(fmt.Sprintf(" %2d ", day.Day())))
		}
		row.WriteString(frame.Render("│"))
		lines = append(lines, row.String())
	}
	lines = append(lines, frame.Render("╰"+strings.Repeat("─", inner)+"╯"))
	return strings.Join(lines, "\n")
}

func (m Model) datePickerView() string {
	label := m.calendar.field
	if field := m.form.field(label); field != nil {
		label = field.Label
	}
	return titleStyle.Render(label+" · "+m.calendar.day.Format("2006-01-02")) + "\n\n" + m.calendarView()
}

func (m *Model) calendarAction(id string) bool {
	if m.calendar == nil {
		return false
	}
	switch id {
	case "date-choose":
		m.applyDatePicker(m.calendar.day.Format("2006-01-02"))
	case "date-today":
		m.calendar.day = calendarDay(time.Now())
	case "date-clear":
		m.applyDatePicker("")
	case "date-type":
		m.typeDate()
	case "date-cancel":
		m.closeDatePicker()
	default:
		return false
	}
	return true
}

func (m Model) updateDatePicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "f6" {
		m.buttonsFocused, m.buttonFocus = !m.buttonsFocused, 0
		return m, nil
	}
	if m.buttonsFocused {
		switch key {
		case "tab", "shift+tab", "left", "right":
			delta := 1
			if key == "shift+tab" || key == "left" {
				delta = -1
			}
			m.buttonFocus = (m.buttonFocus + len(m.buttons()) + delta) % len(m.buttons())
			return m, nil
		case "enter":
			return m, m.activate(m.buttons()[m.buttonFocus].ID)
		}
	}
	p := m.calendar
	switch key {
	case "esc":
		m.closeDatePicker()
	case "left":
		p.day = p.day.AddDate(0, 0, -1)
	case "right":
		p.day = p.day.AddDate(0, 0, 1)
	case "up":
		p.day = p.day.AddDate(0, 0, -7)
	case "down":
		p.day = p.day.AddDate(0, 0, 7)
	case "pgup":
		p.moveMonth(-1)
	case "pgdown":
		p.moveMonth(1)
	case "ctrl+pgup":
		p.moveMonth(-12)
	case "ctrl+pgdown":
		p.moveMonth(12)
	case "home":
		p.day = p.day.AddDate(0, 0, 1-p.day.Day())
	case "end":
		p.day = time.Date(p.day.Year(), p.day.Month()+1, 0, 0, 0, 0, 0, time.UTC)
	case "t":
		m.calendarAction("date-today")
	case "delete", "backspace", "ctrl+d":
		m.calendarAction("date-clear")
	case "i":
		m.typeDate()
	case "enter", "tab", "shift+tab", "ctrl+r", "ctrl+enter":
		m.applyDatePicker(p.day.Format("2006-01-02"))
		if key == "ctrl+r" || key == "ctrl+enter" {
			m.prepareReview()
		} else if key == "tab" || key == "shift+tab" {
			delta := 1
			if key == "shift+tab" {
				delta = -1
			}
			if m.focus+delta >= len(m.visibleFields()) {
				m.buttonsFocused, m.buttonFocus = true, 1
			} else {
				m.focus = max(0, m.focus+delta)
			}
		}
	default:
		if msg.Text != "" {
			m.typeDate()
			return m, m.updateEditor(msg)
		}
	}
	return m, nil
}

func (m Model) datePickerClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft {
		return m, nil
	}
	b := m.calendarBounds()
	x, y := msg.X-b.x, msg.Y-b.y
	if b.contains(msg.X, msg.Y) {
		switch {
		case y == 0 && x >= 1 && x <= 3:
			m.calendar.moveMonth(-1)
		case y == 0 && x >= b.width-4 && x <= b.width-2:
			m.calendar.moveMonth(1)
		case y >= 2 && y < 8 && x >= 1 && x < b.width-1:
			day := m.calendar.firstCell().AddDate(0, 0, (y-2)*7+(x-1)/4)
			m.applyDatePicker(day.Format("2006-01-02"))
		}
		return m, nil
	}
	if m.inlineCalendar() && msg.Y == b.y-1 {
		start := b.x
		for _, action := range calendarActions {
			size := lipgloss.Width(action.label) + 2
			if msg.X >= start && msg.X < start+size {
				m.calendarAction(action.id)
				return m, nil
			}
			start += size + 1
		}
	}
	if msg.Y == m.height-5 {
		return m.mouseClick(msg)
	}
	if m.inlineCalendar() {
		g := m.formLayout(m.width-4, m.height-9)
		index, hit := g.fieldAtRow(msg.Y - 3 - g.top)
		m.closeDatePicker()
		if hit && index < len(m.visibleFields()) {
			m.focus = index
			return m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		}
	}
	return m, nil
}

var calendarActions = []choiceAction{{"date-today", "Today"}, {"date-clear", "Clear"}, {"date-type", "Type date/time"}}

func calendarToolbar() string {
	var parts []string
	for _, action := range calendarActions {
		parts = append(parts, selectedStyle.Render("["+action.label+"]"))
	}
	return strings.Join(parts, " ")
}

func (m *Model) dateControlClick(msg tea.MouseClickMsg) bool {
	fields := m.visibleFields()
	if m.calendar != nil || len(fields) == 0 || m.focus >= len(fields) {
		return false
	}
	field := m.form.Fields[fields[m.focus]]
	if !dateField(field) {
		return false
	}
	g := m.formLayout(m.width-4, m.height-9)
	x, y := msg.X-2-g.controlX, msg.Y-3-g.top-g.fieldRow(m.focus)
	if y != 1 || x < 0 || x >= 31 {
		return false
	}
	if x < 12 {
		m.openDatePicker(field)
	} else if !m.editing {
		m.editField()
	}
	return true
}
