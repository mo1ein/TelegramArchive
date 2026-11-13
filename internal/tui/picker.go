package tui

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type choicePicker struct {
	field, title string
	choices      []string
	selected     map[string]bool
	multi        bool
	cursor       int
	offset       int
	query        textinput.Model
	searching    bool
	err          string
	labels       map[string]string
	custom       bool
	selectedOnly bool
	loading      bool
	loadStage    string
	loadErr      string
}

func newChoicePicker(field Field, width int) *choicePicker {
	p := &choicePicker{field: field.Name, title: field.Label, choices: slices.Clone(field.Choices), labels: maps.Clone(field.ChoiceLabels), selected: map[string]bool{}, multi: field.Multi, query: textinput.New()}
	p.custom = customChoices(field)
	p.query.Prompt, p.query.Placeholder, p.query.CharLimit = "", "Type to filter options…", 256
	if p.custom {
		p.query.Placeholder, p.query.CharLimit = "Type an item · click Add", 4096
	}
	p.query.SetWidth(max(1, width-5))
	p.query.SetVirtualCursor(true)
	styles := textinput.DefaultDarkStyles()
	styles.Focused.Text, styles.Focused.Placeholder, styles.Cursor.Blink = textStyle, dim, false
	p.query.SetStyles(styles)
	values := []string{field.Value}
	if field.Multi {
		values, _ = readCSV(field.Value)
	}
	for _, value := range values {
		if p.custom && !slices.Contains(p.choices, value) && value != "" {
			p.choices = append(p.choices, value)
		}
		p.selected[value] = true
	}
	if !p.multi {
		p.cursor = max(0, slices.Index(p.choices, field.Value))
	}
	return p
}

func (m *Model) openPicker(field Field) tea.Cmd {
	p := newChoicePicker(field, m.width-4)
	if choiceField(field) {
		for n, index := range m.visibleFields() {
			if m.form.Fields[index].Name == field.Name {
				m.focus = n
				break
			}
		}
	}
	m.picker, m.err, m.buttonsFocused = p, "", false
	p.query.SetWidth(max(1, min(60, m.formLayout(m.width-4, m.height-9).controlWidth)-5))
	if field.Name == "ns" {
		return m.loadAccounts()
	}
	if field.Name == "chat" {
		return m.loadChatChoices(false)
	}
	return nil
}

func (p *choicePicker) filtered() []string {
	var values []string
	for _, value := range p.choices {
		if p.selectedOnly && !p.selected[value] {
			continue
		}
		if matches(p.label(value)+" "+value, p.query.Value()) {
			values = append(values, value)
		}
	}
	return values
}

func (p *choicePicker) label(value string) string {
	if label, ok := p.labels[value]; ok {
		return safeText(label)
	}
	if p.custom {
		return safeText(value)
	}
	return choiceLabel(p.field, value)
}

func (p *choicePicker) special(value string) bool {
	return !p.custom && specialChoice(p.field, value)
}

func (p *choicePicker) addCustom() {
	value := p.query.Value()
	if !p.custom || strings.TrimSpace(value) == "" {
		return
	}
	if !slices.Contains(p.choices, value) {
		p.choices = append(p.choices, value)
	}
	p.selected[value], p.err, p.searching = true, "", false
	p.query.SetValue("")
	p.query.Blur()
	p.cursor = max(0, slices.Index(p.filtered(), value))
}

func (p *choicePicker) clear() {
	p.selected, p.err = map[string]bool{}, ""
	if p.custom {
		p.query.SetValue("")
		p.searching = false
		p.query.Blur()
	}
}

func (p *choicePicker) checked(value string) bool {
	if p.selected[value] {
		return true
	}
	if value == "all" && p.special(value) {
		found := false
		for _, option := range p.choices {
			if !p.special(option) {
				found = true
				if !p.selected[option] {
					return false
				}
			}
		}
		return found
	}
	if value != "none" && value != "all" && p.selected["all"] && p.special("all") {
		return true
	}
	return false
}

func (p *choicePicker) toggle(value string) {
	p.err = ""
	if !p.multi {
		p.selected = map[string]bool{value: true}
		return
	}
	if p.special(value) {
		checked := p.checked(value)
		p.selected = map[string]bool{}
		if !checked {
			p.selected[value] = true
		}
		return
	}
	if p.selected["all"] && p.special("all") {
		p.selected = map[string]bool{}
		for _, option := range p.choices {
			if option != "all" && option != "none" {
				p.selected[option] = true
			}
		}
	}
	if p.special("none") {
		delete(p.selected, "none")
	}
	p.selected[value] = !p.selected[value]
}

func (p *choicePicker) values() []string {
	var values []string
	for _, value := range p.choices {
		if p.selected[value] {
			values = append(values, value)
		}
	}
	return values
}

func (m *Model) applyPicker() {
	p := m.picker
	if p == nil {
		return
	}
	field := m.form.field(p.field)
	if field == nil {
		return
	}
	p.addCustom()
	values := p.values()
	if !p.multi {
		rows := p.filtered()
		if len(rows) > 0 {
			values = []string{rows[p.cursor]}
		} else {
			values = nil
		}
	}
	if len(values) == 0 && (!p.custom || field.Required) {
		p.err = "Choose at least one option before applying."
		return
	}
	copy := *field
	copy.Value, copy.Dirty = "", true
	if p.multi {
		copy.Value = csvString(values)
	} else {
		copy.Value = values[0]
	}
	if err := validate(copy); err != nil {
		p.err = copy.Error(err).Error()
		return
	}
	previous := field.Value
	*field = copy
	m.form.reconcileSelection(field.Name)
	if field.Name == "ns" && previous != copy.Value {
		m.resetChatSelection()
	}
	m.closePicker()
	m.err = ""
}

func (m Model) updatePicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := m.picker
	rows := p.filtered()
	inline := m.inlinePicker()
	if p.multi && msg.String() == "left" {
		m.applyPicker()
		return m, nil
	}
	if inline {
		switch msg.String() {
		case "tab", "shift+tab", "f6", "ctrl+r", "ctrl+enter":
			m.applyPicker()
			if m.picker != nil {
				return m, nil
			}
			switch msg.String() {
			case "ctrl+r", "ctrl+enter":
				m.prepareReview()
			case "f6":
				m.buttonsFocused, m.buttonFocus = true, 1
			default:
				delta := 1
				if msg.String() == "shift+tab" {
					delta = -1
				}
				next := m.focus + delta
				if next >= len(m.visibleFields()) {
					m.buttonsFocused, m.buttonFocus = true, 1
				} else {
					m.focus = max(0, next)
				}
			}
			return m, nil
		case "ctrl+l":
			if p.field == "ns" {
				return m, m.loadAccounts()
			}
			if p.field == "chat" {
				return m, m.loadChatChoices(true)
			}
		case "ctrl+f":
			p.searching = true
			p.query.Focus()
			return m, nil
		}
	}
	if m.buttonsFocused {
		switch msg.String() {
		case "tab", "right", "shift+tab", "left":
			delta := 1
			if msg.String() == "shift+tab" || msg.String() == "left" {
				delta = -1
			}
			m.buttonFocus = (m.buttonFocus + len(m.buttons()) + delta) % len(m.buttons())
			return m, nil
		case "enter":
			return m, m.activate(m.buttons()[m.buttonFocus].ID)
		case "esc", "up", "down":
			m.buttonsFocused = false
			return m, nil
		}
	}
	switch msg.String() {
	case "tab", "shift+tab":
		m.buttonsFocused, m.buttonFocus = true, 0
	case "ctrl+d":
		p.clear()
	case "ctrl+s":
		if p.multi {
			p.addCustom()
			p.selectedOnly, p.cursor = !p.selectedOnly, 0
			p.query.SetValue("")
			p.searching = false
			p.query.Blur()
		}
	case "esc":
		if inline {
			m.closePicker()
		} else if p.searching {
			p.searching = false
			p.query.Blur()
		} else {
			m.closePicker()
		}
	case "up", "down", "left", "right", "pgup", "pgdown", "home", "end":
		if p.searching {
			if msg.String() == "left" || msg.String() == "right" || msg.String() == "home" || msg.String() == "end" {
				p.query, _ = p.query.Update(msg)
				return m, nil
			}
			p.searching = false
			p.query.Blur()
			return m, nil
		}
		delta := 1
		columns, page := 1, 5
		if inline {
			bounds := m.inlineChoiceBounds()
			page = choiceListWindow(bounds.height, len(rows), p.cursor, p.offset).rows
		}
		switch msg.String() {
		case "up":
			delta = -columns
		case "down":
			delta = columns
		case "left":
			delta = -1
		case "pgup":
			delta = -page
		case "pgdown":
			delta = page
		case "home":
			p.cursor = 0
			return m, nil
		case "end":
			p.cursor = max(0, len(rows)-1)
			return m, nil
		}
		p.cursor = max(0, min(max(0, len(rows)-1), p.cursor+delta))
	case "enter":
		if p.custom && p.searching && strings.TrimSpace(p.query.Value()) != "" {
			p.addCustom()
			return m, nil
		}
		p.searching = false
		p.query.Blur()
		if len(rows) > 0 {
			p.cursor = max(0, min(len(rows)-1, p.cursor))
			p.toggle(rows[p.cursor])
			p.cursor = max(0, min(max(0, len(p.filtered())-1), p.cursor))
		}
		if !p.multi {
			m.applyPicker()
		}
	case "space", " ":
		if !p.searching && len(rows) > 0 {
			p.toggle(rows[p.cursor])
			return m, nil
		}
		fallthrough
	default:
		if msg.String() == "ctrl+r" && p.field == "ns" {
			return m, m.loadAccounts()
		}
		if msg.String() == "ctrl+a" && !p.searching {
			m.selectAllChoices()
			return m, nil
		}
		if msg.Text != "" || msg.String() == "backspace" || msg.String() == "ctrl+u" {
			p.searching = true
			p.query.Focus()
			p.query.SetWidth(max(1, min(60, m.formLayout(m.width-4, m.height-9).controlWidth)-5))
			p.query, _ = p.query.Update(msg)
			p.cursor = 0
		}
	}
	return m, nil
}

func (m Model) pickerClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft {
		return m, nil
	}
	if m.inlinePicker() {
		if handled, command := m.inlineChoiceClick(msg); handled {
			return m, command
		}
		layout := m.formLayout(m.width-4, m.height-9)
		index, fieldHit := layout.fieldAtRow(msg.Y - 3 - layout.top)
		m.applyPicker()
		if m.picker != nil {
			return m, nil
		}
		if fieldHit && index < len(m.visibleFields()) {
			m.focus = index
			return m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		}
		return m.mouseClick(msg)
	}
	if msg.Y == m.height-5 {
		return m.mouseClick(msg)
	}
	if msg.Y >= 6 && msg.Y <= 8 {
		m.picker.searching = true
		m.picker.query.Focus()
		return m, nil
	}
	page := max(1, m.height-19)
	start := max(0, m.picker.cursor-page+1)
	row := msg.Y - 10
	rows := m.picker.filtered()
	if row >= 0 && row < page && start+row < len(rows) {
		m.picker.cursor, m.picker.searching = start+row, false
		m.picker.query.Blur()
		m.picker.toggle(rows[start+row])
	}
	return m, nil
}

func (m *Model) selectAllChoices() {
	p := m.picker
	if p == nil || !p.multi {
		return
	}
	p.selected = map[string]bool{}
	if slices.Contains(p.choices, "all") && p.special("all") {
		p.selected["all"] = true
		return
	}
	for _, option := range p.choices {
		if !p.special(option) {
			p.selected[option] = true
		}
	}
}

func (m Model) pickerView(width, height int) string {
	p := m.picker
	rows := p.filtered()
	help := "Choose an option with ↑ / ↓, then press Enter."
	if p.multi {
		help = "Space checks an item · Enter applies your selection"
	}
	query := p.query
	query.SetWidth(max(1, width-5))
	lines := []string{titleStyle.Render(p.title), dim.Render(clip(help, width)), "", lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(line).Padding(0, 1).Width(width).Render(query.View()), ""}
	page := max(1, height-10)
	start := max(0, p.cursor-page+1)
	for i := start; i < min(len(rows), start+page); i++ {
		value := rows[i]
		mark, style := "[ ]", textStyle
		if p.checked(value) {
			mark = "[x]"
		}
		if !p.multi {
			mark = "○"
			if p.checked(value) {
				mark = "●"
			}
		}
		if i == p.cursor {
			style = selectedStyle.Bold(true).Width(width)
		}
		lines = append(lines, style.Render(clip(" "+mark+" "+p.label(value), width)))
	}
	if len(rows) == 0 {
		message := "No matching options. Clear the search to see everything."
		if len(p.choices) == 0 && p.field == "ns" {
			message = "No saved accounts. Connect a Telegram account first."
			if m.accountError != "" {
				message = "Could not load saved accounts: " + m.accountError
			}
		}
		lines = append(lines, dim.Render(clip(message, width)))
	}
	for lipgloss.Height(strings.Join(lines, "\n")) < height-2 {
		lines = append(lines, "")
	}
	selection := fmt.Sprintf("%d selected", len(p.values()))
	if p.selected["all"] && p.special("all") {
		selection = "All selected"
	}
	lines = append(lines, accent.Render(selection)+dim.Render(" · Type to filter · ↓ returns to the list"))
	if p.err != "" {
		lines = append(lines, warning.Render(clip(p.err, width)))
	} else if p.field == "ns" && m.accountError != "" {
		lines = append(lines, warning.Render(clip("Could not refresh saved accounts: "+m.accountError, width)))
	}
	return strings.Join(lines, "\n")
}
