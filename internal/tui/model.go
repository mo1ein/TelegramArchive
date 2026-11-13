package tui

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/mo1ein/TelegramArchive/internal/taskui"
)

type screen int

const (
	dashboard screen = iota
	configure
	review
	result
	running
)

type Model struct {
	actions                            []Action
	base                               []string
	globals                            map[string]string
	ctx                                context.Context
	run                                func(context.Context, Plan) tea.Cmd
	width, height                      int
	screen                             screen
	group, selected, focus, scroll     int
	search, fieldSearch                string
	searching, advanced, editing, help bool
	input                              []rune
	cursor                             int
	editor                             textinput.Model
	meter                              progress.Model
	loader                             spinner.Model
	status                             *taskui.Progress
	outputRevision                     uint64
	syncedOutput                       bool
	form                               Form
	plan                               Plan
	last                               finishedMsg
	loading                            bool
	pendingAction                      string
	pendingSelection                   string
	stage                              string
	taskCancel                         context.CancelFunc
	source                             *resultSnapshot
	showCommand                        bool
	chatCache                          map[[32]byte]browseCache
	cachedAt                           time.Time

	job                                            *job
	progress                                       []string
	output                                         string
	data                                           resultData
	resultSelected, resultTab, buttonFocus         int
	resultSearch                                   string
	buttonsFocused, responding, follow, cancelling bool
	started                                        time.Time
	err                                            string
	accounts, accountSelection                     []string
	accountLoader                                  AccountLoader
	accountsLoaded                                 bool
	accountError                                   string
	picker                                         *choicePicker
	batchIndex                                     int
	batchResults                                   []accountOutcome
	batchStarted                                   time.Time
	selectedResults                                map[string]bool
	pathDialog                                     *pathDialog
	choosePath                                     func(context.Context, pathRequest) ([]string, error)
	chatLookup                                     *chatLookup
	chatChoiceKey                                  [32]byte
	calendar                                       *datePicker
}

func New(actions []Action, base []string, namespace string, ctx context.Context, options ...Option) Model {
	m := Model{actions: actions, base: slices.Clone(base), globals: map[string]string{"ns": namespace}, ctx: ctx, run: RunCommand, width: 100, height: 32,
		editor: textinput.New(), meter: newProgress(),
		loader: spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(accent)), choosePath: chooseNativePath}
	if namespace != "" {
		m.accounts, m.accountSelection = []string{namespace}, []string{namespace}
	}
	for _, option := range options {
		option(&m)
	}
	return m
}

func (m Model) Init() tea.Cmd { return m.loadAccounts() }

func (m Model) filteredActions() []Action {
	var out []Action
	for _, action := range m.actions {
		if m.search == "" {
			if Groups[m.group] == "All commands" && action.Group == "Quick start" {
				continue
			}
			if Groups[m.group] != "All commands" && action.Group != Groups[m.group] {
				continue
			}
		} else if action.Group == "Quick start" {
			continue
		}
		if matches(action.Title+" "+action.Description+" "+action.Group, m.search) {
			out = append(out, action)
		}
	}
	return out
}

func matches(value, query string) bool {
	value = strings.ToLower(value)
	for _, word := range strings.Fields(strings.ToLower(query)) {
		if !strings.Contains(value, word) {
			return false
		}
	}
	return true
}

func (m Model) visibleFields() []int {
	var out []int
	for i, field := range m.form.Fields {
		if !m.advanced && field.Advanced && m.fieldSearch == "" {
			continue
		}
		if !m.advanced && m.fieldSearch == "" && strings.Join(m.form.Action.Path, " ") == "login" && (field.Name == "desktop" || field.Name == "passcode") {
			if method := m.form.field("type"); method != nil && method.Value != "desktop" {
				continue
			}
		}
		if matches(field.Label+" "+field.Name+" "+field.Help, m.fieldSearch) {
			out = append(out, i)
		}
	}
	return out
}

func (m *Model) open(action Action) {
	m.calendar = nil
	m.closePathDialog()
	m.closePicker()
	m.chatChoiceKey = [32]byte{}
	m.form = NewForm(action, m.globals)
	m.configureAccountField()
	m.configureChatField()
	m.picker = nil
	m.screen, m.focus, m.scroll, m.err = configure, 0, 0, ""
	m.advanced, m.editing, m.searching = false, false, false
	m.fieldSearch = ""
	m.buttonsFocused = false
	m.showCommand = false
	m.buttonFocus = 0
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Remember the viewport just displayed before moving the cursor. Clicking
	// a visible checkbox must leave the list in the same place under the mouse.
	if m.inlinePicker() {
		p := m.picker
		p.offset = choiceListWindow(m.inlineChoiceBounds().height, len(p.filtered()), p.cursor, p.offset).start
	}
	switch msg := msg.(type) {
	case chatLookupStartedMsg:
		return m, m.chatLookupStarted(msg)
	case chatLookupPollMsg:
		return m, m.pollChatChoices(msg.lookup)
	case pathSelectedMsg:
		return m, m.pathDialogResult(msg)
	case pathDirectoryMsg:
		if m.pathDialog != msg.dialog || msg.dialog.browser == nil || msg.dialog.browser.revision != msg.revision {
			return m, nil
		}
		b := m.pathDialog.browser
		b.loading, b.entries = false, msg.entries
		if msg.err != nil {
			b.err = safeText(msg.err.Error())
		}
		return m, nil
	case accountsLoadedMsg:
		previousAccount := ""
		if field := m.form.field("ns"); field != nil {
			previousAccount = field.Value
		}
		m.accountsLoaded = true
		m.accountError = ""
		if msg.err != nil {
			m.accountError = msg.err.Error()
		} else {
			m.accounts = slices.Clone(msg.names)
		}
		if m.screen == configure {
			m.configureAccountField()
			if field := m.form.field("ns"); field != nil && field.Value != previousAccount {
				m.resetChatSelection()
			}
		}
		if m.picker != nil && m.picker.field == "ns" {
			m.picker.choices = slices.Clone(m.accounts)
			m.picker.cursor = 0
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.pathDialog != nil && m.pathDialog.browser != nil {
			b := m.pathDialog.browser
			b.query.SetWidth(max(1, m.width-7))
			b.name.SetWidth(max(1, m.width-16))
			b.query, _ = b.query.Update(nil)
			b.name, _ = b.name.Update(nil)
		}
		if m.picker != nil {
			m.picker.query.SetWidth(max(1, min(60, m.formLayout(m.width-4, m.height-9).controlWidth)-5))
			m.picker.query, _ = m.picker.query.Update(nil)
		}
		if m.editing || m.searching {
			m.editor.SetWidth(m.editorWidth())
			m.editor, _ = m.editor.Update(nil)
		}
		if m.job != nil {
			m.job.mu.Lock()
			if m.job.resize != nil {
				m.job.resize(max(20, m.width-6), max(5, m.height-17))
			}
			m.job.terminal.width = max(20, min(m.width-6, 500))
			m.job.terminal.height = max(5, min(m.height-17, 200))
			m.job.mu.Unlock()
		}
		return m, nil
	case progress.FrameMsg:
		if m.screen != running {
			return m, nil
		}
		var command tea.Cmd
		m.meter, command = m.meter.Update(msg)
		return m, command
	case startedMsg:
		m.job = msg.job
		if m.cancelling {
			m.job.cancel()
		}
		m.started = msg.job.started
		msg.job.mu.Lock()
		if msg.job.resize != nil {
			msg.job.resize(max(20, m.width-6), max(5, m.height-17))
		}
		msg.job.mu.Unlock()
		return m, poll()
	case pollMsg:
		if m.job == nil {
			return m, nil
		}
		m.loader, _ = m.loader.Update(m.loader.Tick())
		m.job.mu.Lock()
		if !m.syncedOutput || m.outputRevision != m.job.revision {
			m.output = redactOutput(m.job.terminal.text())
			m.progress = append([]string(nil), m.job.terminal.progress...)
			if m.job.terminal.status != nil {
				status := *m.job.terminal.status
				m.status = &status
			}
			m.outputRevision, m.syncedOutput = m.job.revision, true
		}
		m.stage = m.job.stage
		if m.job.streamSeen {
			if m.data.Kind != "chat ls" || len(m.data.Items) != len(m.job.items) {
				m.data = resultData{Kind: "chat ls", Items: append([]item(nil), m.job.items...)}
			}
			if len(m.data.Items) > 0 {
				m.screen = result
				m.loading = !m.job.done
			}
		}
		done := m.job.done
		if done {
			m.last = finishedMsg{taskFailure(m.job.err, m.output), m.job.ended.Sub(m.job.started)}
			if m.job.structured && !m.job.overflow && !m.job.streamSeen {
				data, err := decodeResults(m.plan.Path, m.job.stdout.Bytes())
				if err == nil {
					m.data = data
				} else {
					if m.job.err == nil {
						m.err = "Could not build the result list; activity output is available."
					}
				}
			}
			if m.job.overflow {
				m.err = "Result exceeds the 4 MiB list limit; refine the query."
			}
		}
		m.job.mu.Unlock()
		if done {
			if m.last.err == nil && m.job.streamSeen && !m.job.overflow {
				m.saveChatCache()
			}
			m.loading = false
			m.screen = result
			if m.taskCancel != nil {
				m.taskCancel()
			}
			if command, handled := m.finishAccount(m.last); handled {
				return m, command
			}
			m.responding = false
			m.buttonFocus = 0
			if m.data.Kind == "" || (m.last.err != nil && len(m.data.Items) == 0) {
				m.resultTab = 1
			}
			if m.pendingAction != "" {
				action := m.pendingAction
				rows := m.resultItems()
				selected := slices.IndexFunc(rows, func(row item) bool { return row.ID == m.pendingSelection })
				if selected < 0 {
					m.resultSearch = ""
					selected = slices.IndexFunc(m.resultItems(), func(row item) bool { return row.ID == m.pendingSelection })
				}
				m.resultSelected = max(0, selected)
				m.pendingAction = ""
				m.pendingSelection = ""
				m.cancelling = false
				return m, m.activate(action)
			}
			return m, nil
		}
		if m.status != nil && m.status.Determinate && m.meter.Percent() != m.status.Percent {
			return m, tea.Batch(poll(), m.meter.SetPercent(m.status.Percent))
		}
		return m, poll()
	case inputErrorMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
		}
		return m, nil
	case tea.MouseClickMsg:
		if m.pathDialog != nil {
			return m.pathDialogClick(msg)
		}
		if m.picker != nil {
			return m.pickerClick(msg)
		}
		if m.calendar != nil {
			return m.datePickerClick(msg)
		}
		return m.mouseClick(msg)
	case tea.MouseWheelMsg:
		delta := 3
		if msg.Button == tea.MouseWheelUp {
			delta = -3
		}
		if m.calendar != nil {
			if m.calendarBounds().contains(msg.X, msg.Y) {
				m.calendar.moveMonth(delta / 3)
			}
			return m, nil
		}
		if m.pathDialog != nil {
			if b := m.pathDialog.browser; b != nil {
				b.cursor = max(0, min(max(0, len(b.filtered())-1), b.cursor+delta))
			}
			return m, nil
		}
		if m.picker != nil {
			if !m.inlinePicker() || m.inlineChoiceBounds().contains(msg.X, msg.Y) {
				step := delta
				if m.inlinePicker() {
					p := m.picker
					rows := choiceListWindow(m.inlineChoiceBounds().height, len(p.filtered()), p.cursor, p.offset).rows
					step = min(3, rows)
					if delta < 0 {
						step = -step
					}
				}
				m.picker.cursor = max(0, min(max(0, len(m.picker.filtered())-1), m.picker.cursor+step))
				return m, nil
			}
			m.applyPicker()
			if m.picker != nil {
				return m, nil
			}
		}
		if m.screen == configure && !m.searching {
			if m.saveInput() {
				m.focus = max(0, min(max(0, len(m.visibleFields())-1), m.focus+delta))
			}
		} else if m.screen == result && m.resultTab == 0 {
			m.resultSelected = max(0, min(max(0, len(m.resultItems())-1), m.resultSelected+delta))
		} else if m.screen == running || m.screen == result {
			m.scrollActivity(delta)
		}
		return m, nil

	case finishedMsg:
		m.loading = false
		if m.taskCancel != nil {
			m.taskCancel()
		}
		if command, handled := m.finishAccount(msg); handled {
			return m, command
		}
		m.resultTab = 1
		m.last, m.screen, m.scroll = msg, result, 0
		return m, nil
	case tea.PasteMsg:
		if m.calendar != nil {
			m.typeDate()
			return m, m.updateEditor(msg)
		}
		if m.pathDialog != nil {
			if b := m.pathDialog.browser; b != nil {
				if b.naming {
					b.name, _ = b.name.Update(msg)
				} else {
					b.searching = true
					b.query.Focus()
					b.query, _ = b.query.Update(msg)
					b.cursor = 0
				}
			}
			return m, nil
		}
		if m.picker != nil {
			m.picker.searching = true
			m.picker.query.Focus()
			m.picker.query, _ = m.picker.query.Update(msg)
			m.picker.cursor = 0
			return m, nil
		}
		if m.screen == running && m.responding && m.job != nil {
			return m, m.sendInput(msg.Content)
		}
		if !m.editing && !m.searching && m.screen == configure {
			fields := m.visibleFields()
			if len(fields) > 0 && choiceField(m.form.Fields[fields[m.focus]]) {
				command := m.openPicker(m.form.Fields[fields[m.focus]])
				m.picker.searching = true
				m.picker.query.Focus()
				m.picker.query, _ = m.picker.query.Update(msg)
				return m, command
			}
			if len(fields) > 0 && m.form.Fields[fields[m.focus]].Type != "bool" && len(m.form.Fields[fields[m.focus]].Choices) == 0 {
				m.editField()
			}
		}
		if m.editing || m.searching {
			return m, m.updateEditor(msg)
		}
		return m, nil
	case tea.KeyPressMsg:
		key := msg.String()
		if m.pathDialog != nil {
			return m.updatePathDialog(msg)
		}
		if key == "ctrl+c" {
			if m.chatLookup != nil {
				m.stopChatLookup()
				return m, nil
			}
			if m.screen == running || m.loading {
				m.cancelJob()
				return m, nil
			}
			return m, tea.Quit
		}
		if m.picker != nil {
			return m.updatePicker(msg)
		}
		if m.calendar != nil {
			return m.updateDatePicker(msg)
		}
		if m.screen == running && m.responding {
			if key == "esc" {
				m.responding = false
				return m, nil
			}
			return m, m.sendInput(terminalKey(msg))
		}
		if m.help {
			switch key {
			case "esc", "?", "enter":
				m.help, m.scroll = false, 0
			case "up", "k", "pgup":
				m.scroll = max(0, m.scroll-1)
			case "down", "j", "pgdown":
				m.scroll++
			}
			return m, nil
		}
		if m.editing || m.searching {
			return m.updateInput(msg)
		}
		if key == "?" {
			m.help, m.scroll = true, 0
			return m, nil
		}
		if key == "q" && m.screen == dashboard {
			return m, tea.Quit
		}
		if key == "f6" {
			m.buttonsFocused = !m.buttonsFocused
			m.buttonFocus = 0
			return m, nil
		}
		if m.screen == configure && m.buttonsFocused && key == "shift+tab" {
			m.buttonsFocused = false
			return m, nil
		}
		if m.buttonsFocused && (key == "tab" || key == "shift+tab") {
			delta := 1
			if key == "shift+tab" {
				delta = -1
			}
			m.buttonFocus = (m.buttonFocus + len(m.buttons()) + delta) % len(m.buttons())
			return m, nil
		}
		if key == "tab" && (m.screen == review || m.screen == running || m.screen == result) {
			m.buttonsFocused = true
			m.buttonFocus = (m.buttonFocus + 1) % len(m.buttons())
			return m, nil
		}
		if key == "shift+tab" && (m.screen == review || m.screen == running || m.screen == result) {
			m.buttonsFocused = true
			m.buttonFocus = (m.buttonFocus + len(m.buttons()) - 1) % len(m.buttons())
			return m, nil
		}
		if m.buttonsFocused && (key == "left" || key == "right") {
			delta := 1
			if key == "left" {
				delta = -1
			}
			m.buttonFocus = (m.buttonFocus + len(m.buttons()) + delta) % len(m.buttons())
			return m, nil
		}
		if m.buttonsFocused && key == "enter" {
			return m, m.activate(m.buttons()[m.buttonFocus].ID)
		}
		switch m.screen {
		case running:
			switch key {
			case "esc":
				m.cancelJob()
			case "i":
				m.responding = true
			case "up", "k", "pgup":
				m.scrollActivity(-3)
			case "down", "j", "pgdown":
				m.scrollActivity(3)
			case "end", "f":
				m.follow = true
			}
		case dashboard:
			actions := m.filteredActions()
			switch key {
			case "up", "k":
				m.selected = max(0, m.selected-1)
			case "down", "j":
				m.selected = min(max(0, len(actions)-1), m.selected+1)
			case "pgdown":
				m.selected = min(max(0, len(actions)-1), m.selected+5)
			case "pgup":
				m.selected = max(0, m.selected-5)
			case "home":
				m.selected = 0
			case "end":
				m.selected = max(0, len(actions)-1)
			case "left", "shift+tab", "h":
				m.group = (m.group + len(Groups) - 1) % len(Groups)
				m.selected, m.search = 0, ""
			case "right", "tab", "l":
				m.group = (m.group + 1) % len(Groups)
				m.selected, m.search = 0, ""
			case "/":
				m.searching = true
				m.startInput(m.search, "Search workflows…", false)
			case "esc":
				m.search, m.selected = "", 0
			case "enter":
				if len(actions) > 0 {
					action := actions[m.selected]
					if path := strings.Join(action.Path, " "); path == "accounts" || path == "chat ls" || path == "cron list" {
						return m, m.browse(action)
					}
					m.open(action)
				}
			}
		case configure:
			fields := m.visibleFields()
			m.focus = max(0, min(max(0, len(fields)-1), m.focus))
			switch key {
			case "esc":
				if m.source != nil && m.fieldSearch == "" {
					m.restoreList()
					break
				}
				if m.fieldSearch != "" {
					m.fieldSearch, m.focus = "", 0
				} else {
					m.screen, m.err = dashboard, ""
				}
			case "up", "shift+tab":
				m.focus = max(0, m.focus-1)
			case "down", "tab":
				if key == "tab" && m.focus == len(fields)-1 {
					m.buttonsFocused, m.buttonFocus = true, 1
				} else {
					m.focus = min(max(0, len(fields)-1), m.focus+1)
				}
			case "pgdown":
				m.focus = min(max(0, len(fields)-1), m.focus+8)
			case "pgup":
				m.focus = max(0, m.focus-8)
			case "ctrl+a":
				m.advanced = !m.advanced
				m.focus = 0
			case "ctrl+f":
				m.searching = true
				m.startInput(m.fieldSearch, "Find an option…", false)
			case "ctrl+r", "ctrl+enter":
				m.prepareReview()
			case "alt+o":
				if len(fields) > 0 {
					return m, m.openPathDialog(m.form.Fields[fields[m.focus]])
				}

			case "enter", "space", " ", "right", "left":
				if len(fields) == 0 {
					break
				}
				field := &m.form.Fields[fields[m.focus]]
				if dateField(*field) && key != "left" {
					m.openDatePicker(*field)
					return m, nil
				}
				if field.PathKind != "" && key == "enter" {
					return m, m.openPathDialog(*field)
				}
				if field.Type == "bool" {
					if field.Value == "true" {
						field.Value = "false"
					} else {
						field.Value = "true"
					}
					field.Dirty, m.err = true, ""
					m.form.reconcileSelection(field.Name)
				} else if choiceField(*field) {
					if field.Multi && key == "left" {
						return m, nil
					}
					if field.Multi || key == "enter" || field.Type == "accounts" {
						return m, m.openPicker(*field)
					}
					next := slices.Index(field.Choices, field.Value) + 1
					if key == "left" {
						next -= 2
					}
					field.Value, field.Dirty = field.Choices[(next+len(field.Choices))%len(field.Choices)], true
				} else {
					m.editField()
				}
			default:
				if msg.Text != "" && len(fields) > 0 {
					field := m.form.Fields[fields[m.focus]]
					if choiceField(field) {
						command := m.openPicker(field)
						next, _ := m.updatePicker(msg)
						return next, command
					}
					if field.Type != "bool" && len(field.Choices) == 0 {
						m.editField()
						return m, m.updateEditor(msg)
					}
				}
			case "ctrl+d":
				if len(fields) > 0 {
					field := &m.form.Fields[fields[m.focus]]
					field.Value, field.Dirty, m.err = field.Default, false, ""
				}
			}
		case review:
			switch key {
			case "esc", "e":
				m.screen, m.scroll = configure, 0
			case "c":
				m.showCommand = !m.showCommand
			case "enter":
				return m, m.begin()
			case "up", "k", "pgup":
				m.scroll = max(0, m.scroll-1)
			case "down", "j", "pgdown":
				m.scroll++
			}
		case result:
			if m.loading && (key == "esc" || key == "e" || key == "r" || key == "s") {
				m.cancelJob()
				return m, nil
			}
			rows := m.resultItems()
			switch key {
			case "space", " ":
				if (m.data.Kind == "accounts" || m.data.Kind == "chat ls") && len(rows) > 0 {
					row := rows[m.resultSelected]
					if m.data.Kind == "accounts" && row.Status != "ready" {
						m.err = "Choose an available account."
						break
					}
					if m.selectedResults == nil {
						m.selectedResults = map[string]bool{}
					}
					m.selectedResults[row.ID] = !m.selectedResults[row.ID]
				}
			case "b":
				if m.data.Kind == "accounts" {
					return m, m.activate("backup-selected")
				}
			case "esc":
				if m.resultSearch != "" {
					m.resultSearch = ""
					m.resultSelected = 0
				} else {
					m.screen = dashboard
					m.source = nil
					m.err = ""
					m.buttonsFocused = false
				}
			case "enter":
				if len(rows) > 0 {
					m.resultTab = 2
				} else {
					m.screen = dashboard
				}
			case "up", "k":
				m.buttonsFocused = false
				if m.resultTab == 0 {
					m.resultSelected = max(0, m.resultSelected-1)
				} else {
					m.scroll = max(0, m.scroll-1)
				}
			case "down", "j":
				m.buttonsFocused = false
				if m.resultTab == 0 {
					m.resultSelected = min(max(0, len(rows)-1), m.resultSelected+1)
				} else {
					m.scroll++
				}
			case "pgup":
				m.scroll = max(0, m.scroll-8)
				m.resultSelected = max(0, m.resultSelected-8)
			case "pgdown":
				m.scroll += 8
				m.resultSelected = min(max(0, len(rows)-1), m.resultSelected+8)
			case "/":
				m.searching = true
				m.startInput(m.resultSearch, "Search this list…", false)
			case "l":
				m.resultTab = 1
				m.scroll = 0
			case "t":
				m.resultTab = 0
				m.scroll = 0
			case "d":
				m.resultTab = 2
				m.scroll = 0
			case "e":
				m.screen = configure
				m.buttonsFocused = false
			case "r":
				m.screen = review
				m.scroll = 0
				m.buttonsFocused = false
			case "a":
				if m.data.Kind == "chat ls" {
					return m, m.activate("archive")
				}
			case "s":
				if m.data.Kind == "accounts" {
					return m, m.activate("use")
				}
			}

		}
	}
	return m, nil
}

func (m *Model) syncSearch() {
	if !m.searching {
		return
	}
	if m.screen == dashboard {
		m.search, m.selected = string(m.input), 0
	} else if m.screen == result {
		m.resultSearch, m.resultSelected = string(m.input), 0
	} else {
		m.fieldSearch, m.focus = string(m.input), 0
	}
}

func (m *Model) completePath() {
	value := string(m.input)
	if value == "" {
		return
	}
	if strings.HasPrefix(value, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			value = filepath.Join(home, value[2:])
		}
	}
	// Read a single directory instead of expanding arbitrary glob patterns.
	dir, prefix := filepath.Split(value)
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var candidates []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) {
			candidate := filepath.Join(dir, entry.Name())
			if entry.IsDir() {
				candidate += string(filepath.Separator)
			}
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) == 1 {
		m.input = []rune(candidates[0])
		m.cursor = len(m.input)
	}
}
