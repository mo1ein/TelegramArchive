package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type button struct{ ID, Label string }

func (m Model) buttons() []button {
	if m.calendar != nil && !m.inlineCalendar() {
		return []button{{"date-choose", "Choose"}, {"date-today", "Today"}, {"date-clear", "Clear"}, {"date-type", "Type date/time"}, {"date-cancel", "Cancel"}}
	}
	if m.pathDialog != nil {
		return m.pathButtons()
	}
	if m.picker != nil && !m.inlinePicker() {
		if m.picker.field == "ns" && len(m.picker.choices) == 0 {
			return []button{{"picker-connect", "Connect account"}, {"picker-cancel", "Cancel"}}
		}
		if m.picker.multi {
			return []button{{"picker-apply", "Apply selection"}, {"picker-all", "Select all"}, {"picker-clear", "Clear"}, {"picker-cancel", "Cancel"}}
		}
		return []button{{"picker-apply", "Choose"}, {"picker-cancel", "Cancel"}}
	}
	switch m.screen {
	case dashboard:
		return []button{{"open", "Open"}, {"accounts", "Accounts"}, {"chats", "Chats"}, {"schedules", "Schedules"}}
	case configure:
		if m.source != nil {
			return []button{{"list", "Back to list"}, {"review", "Review"}, {"advanced", "More options"}}
		}
		return []button{{"back", "Back"}, {"review", "Review"}, {"advanced", "More options"}}
	case review:
		return []button{{"edit", "Edit"}, {"run", "Run"}, {"home", "Home"}}
	case running:
		return []button{{"cancel", "Cancel"}, {"input", "Respond"}, {"follow", "Follow latest"}}
	case result:
		if m.loading {
			if len(m.data.Items) > 0 {
				if len(m.checkedResults()) > 0 {
					return []button{{"archive-selected", "Archive selected"}, {"cancel", "Stop loading"}, {"details", "Details"}}
				}
				return []button{{"archive", "Archive"}, {"export", "Export"}, {"cancel", "Stop loading"}, {"details", "Details"}, {"activity", "Activity"}}
			}
			return []button{{"cancel", "Stop loading"}, {"details", "Details"}, {"activity", "Activity"}}
		}
		buttons := []button{}
		if m.source != nil {
			buttons = append(buttons, button{"list", "Back to list"})
		}
		switch m.data.Kind {
		case "accounts":
			if len(m.checkedResults()) > 0 {
				buttons = append(buttons, button{"backup-selected", "Back up selected"})
			}
			buttons = append(buttons, button{"use", "Use account"})
		case "chat ls":
			if len(m.data.Items) > 0 {
				if len(m.checkedResults()) > 0 {
					buttons = append(buttons, button{"archive-selected", "Archive selected"})
				} else {
					buttons = append(buttons, button{"archive", "Archive"})
				}
				buttons = append(buttons, button{"export", "Export"}, button{"members", "Members"})
			}
			buttons = append(buttons, button{"refresh", "Refresh"})
			if m.last.err != nil {
				buttons = append(buttons, button{"connection", "Connection settings"})
			}
		case "cron list":
			buttons = append(buttons, button{"jobrun", "Run backup"}, button{"joblogs", "Logs"}, button{"jobremove", "Remove"})
		}
		if len(m.data.Items) > 0 {
			buttons = append(buttons, button{"details", "Details"})
		}
		return append(buttons, button{"activity", "Activity"}, button{"retry", "Run again"}, button{"edit", "Edit"}, button{"home", "Home"})
	}
	return []button{{"home", "Home"}}
}
func (m Model) buttonBar(width int) string {
	buttons := m.buttons()
	start := 0
	for start < m.buttonFocus {
		size := 0
		for i := start; i <= m.buttonFocus; i++ {
			size += lipgloss.Width(buttons[i].Label) + 5
		}
		if size <= width {
			break
		}
		start++
	}
	parts := []string{}
	used := 0
	for i := start; i < len(buttons); i++ {
		label := "[ " + buttons[i].Label + " ]"
		size := lipgloss.Width(label)
		if used+size > width {
			break
		}
		style := selectedStyle
		if i == m.buttonFocus && m.buttonsFocused {
			style = lipgloss.NewStyle().Background(mint).Foreground(lipgloss.Color("#102C2C")).Bold(true)
		}
		parts = append(parts, style.Render(label))
		used += size + 1
	}
	return strings.Join(parts, " ")
}
func (m *Model) cancelJob() {
	m.cancelling = true
	m.responding = false
	if m.taskCancel != nil {
		m.taskCancel()
	}
	if m.job != nil {
		m.cancelling = true
		m.responding = false
		m.job.cancel()
	}
}
func (m Model) sendInput(value string) tea.Cmd {
	j := m.job
	if j == nil || value == "" {
		return nil
	}
	select {
	case j.inputQ <- value:
		return nil
	default:
		return func() tea.Msg { return inputErrorMsg{fmt.Errorf("input queue is full; wait for the task to respond")} }
	}
}
func terminalKey(msg tea.KeyPressMsg) string {
	if msg.Mod&tea.ModCtrl != 0 && msg.Code >= 'a' && msg.Code <= 'z' {
		return string(msg.Code - 'a' + 1)
	}
	switch msg.String() {
	case "enter":
		return "\r"
	case "backspace":
		return "\x7f"
	case "delete":
		return "\x1b[3~"
	case "tab":
		return "\t"
	case "shift+tab":
		return "\x1b[Z"
	case "home":
		return "\x1b[H"
	case "end":
		return "\x1b[F"
	case "up":
		return "\x1b[A"
	case "down":
		return "\x1b[B"
	case "right":
		return "\x1b[C"
	case "left":
		return "\x1b[D"
	case "space":
		return " "
	}
	return msg.Text
}
func (m *Model) browse(action Action) tea.Cmd {
	m.source = nil
	m.selectedResults = nil
	m.open(action)
	plan, err := m.form.Plan(m.base)
	if err != nil {
		m.err = err.Error()
		return nil
	}
	m.plan = plan
	if m.showChatCache() {
		return nil
	}
	return m.begin()
}
func (m *Model) activate(id string) tea.Cmd {
	if m.calendar != nil {
		if m.calendarAction(id) {
			return nil
		}
		if id == "review" {
			m.applyDatePicker(m.calendar.day.Format("2006-01-02"))
		} else {
			m.closeDatePicker()
		}
	}
	if m.pathDialog != nil {
		return m.pathAction(id)
	}
	if m.picker != nil {
		switch id {
		case "picker-add":
			if strings.TrimSpace(m.picker.query.Value()) == "" {
				m.picker.searching = true
				m.picker.query.Focus()
			} else {
				m.picker.addCustom()
			}
		case "picker-apply":
			m.applyPicker()
		case "picker-cancel":
			m.closePicker()
		case "picker-all":
			m.selectAllChoices()
		case "picker-clear":
			m.picker.clear()
		case "picker-connect":
			m.closePicker()
			m.openPath("login", map[string]string{"type": "qr"})
		default:
			if m.inlinePicker() {
				m.applyPicker()
				if m.picker == nil {
					return m.activate(id)
				}
			}
		}
		m.buttonsFocused = false
		return nil
	}
	if m.loading && (id == "archive" || id == "archive-selected" || id == "export" || id == "members") && len(m.resultItems()) > 0 {
		m.pendingAction = id
		m.pendingSelection = m.resultItems()[m.resultSelected].ID
		m.cancelJob()
		return nil
	}
	if m.loading && id != "cancel" && id != "details" && id != "activity" {
		m.err = "Stop loading before starting another task."
		return nil
	}
	m.buttonsFocused = false
	switch id {
	case "backup-selected", "archive-selected":
		rows := m.checkedResults()
		if len(rows) == 0 {
			m.err = "Use Space to check the items to back up."
			return nil
		}
		var selected []string
		for _, row := range rows {
			selected = append(selected, row.ID)
		}
		m.saveList()
		if id == "backup-selected" {
			m.accountSelection = selected
			for _, name := range selected {
				if !slices.Contains(m.accounts, name) {
					m.accounts = append(m.accounts, name)
				}
			}
			m.openPath("sync", map[string]string{"everything": "true", "format": "both"})
		} else {
			m.selectBrowserAccount()
			m.openPath("archive", map[string]string{"chat": csvString(selected), "media": "all", "format": "both"})
		}
	case "list":
		m.restoreList()
	case "home", "back":
		m.source = nil
		m.screen = dashboard
		m.err = ""
	case "edit":
		m.screen = configure
	case "connection":
		m.screen = configure
		m.advanced = true
		m.fieldSearch = "proxy"
		m.focus = 0
	case "retry":
		m.screen = review
		m.scroll = 0
	case "refresh":
		return m.begin()
	case "run":
		return m.begin()
	case "cancel":
		m.cancelJob()
	case "input":
		m.responding = !m.responding
	case "follow":
		m.follow = true
	case "advanced":
		m.advanced = !m.advanced
		m.focus = 0
	case "review":
		m.prepareReview()
	case "open":
		actions := m.filteredActions()
		if len(actions) > 0 {
			action := actions[m.selected]
			path := strings.Join(action.Path, " ")
			if path == "accounts" || path == "chat ls" || path == "cron list" {
				return m.browse(action)
			}
			m.open(action)
		}
	case "accounts", "chats", "schedules":
		path := map[string]string{"accounts": "accounts", "chats": "chat ls", "schedules": "cron list"}[id]
		for _, action := range m.actions {
			if strings.Join(action.Path, " ") == path && action.Group != "Quick start" {
				return m.browse(action)
			}
		}
	case "activity":
		m.resultTab = 1
		m.scroll = 0
	case "details":
		m.resultTab = 2
		m.scroll = 0
	default:
		rows := m.resultItems()
		if len(rows) == 0 || m.resultSelected >= len(rows) {
			m.err = "Select a result first."
			return nil
		}
		row := rows[m.resultSelected]
		switch id {
		case "use":
			if row.Status != "ready" {
				m.err = "That account is " + row.Status + "; choose an available account."
				return nil
			}
			m.globals["ns"] = row.ID
			m.accountSelection = []string{row.ID}
			if !slices.Contains(m.accounts, row.ID) {
				m.accounts = append(m.accounts, row.ID)
			}
			m.screen = dashboard
			m.err = ""
		case "archive":
			m.saveList()
			m.selectBrowserAccount()
			m.openPath("archive", map[string]string{"chat": row.ID, "media": "all", "format": "both"})
		case "export":
			m.saveList()
			m.openPath("chat export", map[string]string{"chat": row.ID})
		case "members":
			m.saveList()
			m.openPath("chat users", map[string]string{"chat": row.ID})
		case "jobrun", "joblogs", "jobremove":
			m.saveList()
			path := map[string]string{"jobrun": "cron run", "joblogs": "cron logs", "jobremove": "cron remove"}[id]
			m.openPath(path, map[string]string{"arguments": fmt.Sprintf("%q", row.ID)})
			plan, err := m.form.Plan(m.base)
			if err != nil {
				m.err = err.Error()
				return nil
			}
			m.plan = plan
			if id == "joblogs" {
				return m.begin()
			}
			m.screen = review
		}
	}
	return nil
}
func (m Model) mouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft || m.help || m.searching {
		return m, nil
	}
	if m.pathDialog == nil && m.screen == configure {
		if m.dateControlClick(msg) {
			return m, nil
		}
		if handled, command := m.pathControlClick(msg); handled {
			return m, command
		}
	}
	if m.pathDialog == nil && m.editing && !m.saveInput() {
		return m, nil
	}
	width, height := m.width-4, m.height-7
	if msg.Y == 3+height-1 {
		buttons := m.buttons()
		start := 0
		for start < m.buttonFocus {
			size := 0
			for i := start; i <= m.buttonFocus; i++ {
				size += lipgloss.Width(buttons[i].Label) + 5
			}
			if size <= width {
				break
			}
			start++
		}
		x := 2
		for i := start; i < len(buttons); i++ {
			size := lipgloss.Width(buttons[i].Label) + 4
			if x+size > m.width-2 {
				break
			}
			if msg.X >= x && msg.X < x+size {
				m.buttonFocus = i
				cmd := m.activate(buttons[i].ID)
				return m, cmd
			}
			x += size + 1
		}
	}
	bodyHeight := height - 2
	if m.pathDialog != nil {
		return m, nil
	}
	switch m.screen {
	case dashboard:
		searchTop := 5
		if width < 86 {
			searchTop += 2
		}
		if msg.Y >= searchTop && msg.Y < searchTop+3 && (width < 86 || msg.X >= 25) {
			m.searching = true
			m.startInput(m.search, "Search workflows…", false)
			return m, nil
		}
		if width >= 86 && msg.X < 22 && msg.Y >= 5 {
			index := (msg.Y - 5) / 2
			if index < len(Groups) {
				m.group = index
				m.selected = 0
				m.search = ""
			}
			return m, nil
		}
		listHeight := bodyHeight
		if width < 86 {
			listHeight -= 2
		}
		rowHeight, _, start := m.actionListGeometry(listHeight)
		listTop := 9
		if width < 86 {
			listTop += 2
		}
		index := start + (msg.Y-listTop)/rowHeight
		actions := m.filteredActions()
		if msg.Y >= listTop && index >= 0 && index < len(actions) {
			m.selected = index
			return m, m.activate("open")
		}
	case configure:
		if len(m.visibleFields()) == 0 {
			return m, nil
		}
		if handled, command := m.inlineChoiceClick(msg); handled {
			return m, command
		}
		layout := m.formLayout(width, bodyHeight)
		row := msg.Y - 3 - layout.top
		index, fieldHit := layout.fieldAtRow(row)
		if fieldHit && index < len(m.visibleFields()) {
			m.focus = index
			next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			return next, cmd
		}
	case result:
		if msg.Y == 7 {
			if msg.X >= 2 && msg.X < 10 {
				m.resultTab = 0
			} else if msg.X >= 13 && msg.X < 22 {
				m.resultTab = 1
			} else if msg.X >= 25 && msg.X < 33 {
				m.resultTab = 2
			}
			m.scroll = 0
			return m, nil
		}
		rows := m.resultItems()
		rowStart, page := m.resultListGeometry(bodyHeight)
		start := max(0, m.resultSelected-page+1)
		index := start + (msg.Y-3-rowStart)/2
		if m.resultTab == 0 && msg.Y >= 3+rowStart && index >= 0 && index < len(rows) {
			m.resultSelected = index
			m.buttonsFocused = false
		}
	}
	return m, nil
}

func (m *Model) prepareReview() {
	if m.calendar != nil {
		m.applyDatePicker(m.calendar.day.Format("2006-01-02"))
	}
	if m.inlinePicker() {
		m.applyPicker()
		if m.picker != nil {
			return
		}
	}
	if !m.saveInput() {
		return
	}
	plan, err := m.form.Plan(m.base)
	if err != nil {
		m.err = err.Error()
		return
	}
	m.plan = plan
	for _, field := range m.form.Fields {
		if field.Global {
			if field.Name == "ns" && field.Multi {
				m.accountSelection, _ = readCSV(field.Value)
				if len(m.accountSelection) == 1 {
					m.globals["ns"] = m.accountSelection[0]
				}
				continue
			}
			if field.Dirty {
				m.globals[field.Name] = field.Value
			} else {
				delete(m.globals, field.Name)
			}
		}
	}
	m.screen = review
	m.scroll = 0
	m.err = ""
	m.buttonsFocused = false
}

func (m *Model) scrollActivity(delta int) {
	available := max(1, m.height-16)
	if m.screen == running {
		available = max(1, m.height-18)
		if len(m.progress) == 3 {
			available = max(1, available-4)
		}
	}
	last := max(0, len(strings.Split(m.output, "\n"))-available)
	if m.follow && m.screen == running {
		m.scroll = last
	}
	m.follow = false
	m.scroll = max(0, min(last, m.scroll+delta))
}
