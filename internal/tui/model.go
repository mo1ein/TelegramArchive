package tui

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
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
	form                               Form
	plan                               Plan
	last                               finishedMsg
	loading                            bool
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
}

func New(actions []Action, base []string, namespace string, ctx context.Context) Model {
	return Model{actions: actions, base: slices.Clone(base), globals: map[string]string{"ns": namespace}, ctx: ctx, run: RunCommand, width: 100, height: 32}
}

func (m Model) Init() tea.Cmd { return nil }

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
		if matches(field.Label+" "+field.Name+" "+field.Help, m.fieldSearch) {
			out = append(out, i)
		}
	}
	return out
}

func (m *Model) open(action Action) {
	m.form = NewForm(action, m.globals)
	m.screen, m.focus, m.scroll, m.err = configure, 0, 0, ""
	m.advanced, m.editing, m.searching = false, false, false
	m.fieldSearch = ""
	m.buttonsFocused = false
	m.showCommand = false
	m.buttonFocus = 0
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
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
		m.job.mu.Lock()
		m.output = redactOutput(m.job.terminal.text())
		m.progress = append([]string(nil), m.job.terminal.progress...)
		m.stage = m.job.stage
		if m.job.streamSeen {
			m.data = resultData{Kind: "chat ls", Items: append([]item(nil), m.job.items...)}
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
			m.responding = false
			m.buttonFocus = 0
			if m.data.Kind == "" || (m.last.err != nil && len(m.data.Items) == 0) {
				m.resultTab = 1
			}
			return m, nil
		}
		return m, poll()
	case inputErrorMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
		}
		return m, nil
	case tea.MouseClickMsg:
		return m.mouseClick(msg)
	case tea.MouseWheelMsg:
		delta := 3
		if msg.Button == tea.MouseWheelUp {
			delta = -3
		}
		if m.screen == result && m.resultTab == 0 {
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
		m.resultTab = 1
		m.last, m.screen, m.scroll = msg, result, 0
		return m, nil
	case tea.PasteMsg:
		if m.screen == running && m.responding && m.job != nil {
			return m, m.sendInput(msg.Content)
		}
		if m.editing || m.searching {
			m.insert(strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(msg.Content))
			m.syncSearch()
		}
		return m, nil
	case tea.KeyPressMsg:
		key := msg.String()
		if key == "ctrl+c" {
			if m.screen == running || m.loading {
				m.cancelJob()
				return m, nil
			}
			return m, tea.Quit
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
				m.input, m.cursor = []rune(m.search), len([]rune(m.search))
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
			case "up", "k", "shift+tab":
				m.focus = max(0, m.focus-1)
			case "down", "j", "tab":
				m.focus = min(max(0, len(fields)-1), m.focus+1)
			case "pgdown":
				m.focus = min(max(0, len(fields)-1), m.focus+8)
			case "pgup":
				m.focus = max(0, m.focus-8)
			case "ctrl+a":
				m.advanced = !m.advanced
				m.focus = 0
			case "/":
				m.searching = true
				m.input, m.cursor = []rune(m.fieldSearch), len([]rune(m.fieldSearch))
			case "ctrl+r":
				m.prepareReview()

			case "enter", "space", " ", "right", "left":
				if len(fields) == 0 {
					break
				}
				field := &m.form.Fields[fields[m.focus]]
				if field.Type == "bool" {
					if field.Value == "true" {
						field.Value = "false"
					} else {
						field.Value = "true"
					}
					field.Dirty, m.err = true, ""
				} else if key != "enter" && len(field.Choices) > 0 && !strings.Contains(field.Type, "Slice") {
					next := slices.Index(field.Choices, field.Value) + 1
					if key == "left" {
						next -= 2
					}
					field.Value, field.Dirty = field.Choices[(next+len(field.Choices))%len(field.Choices)], true
				} else {
					m.input, m.cursor, m.editing = []rune(field.Value), len([]rune(field.Value)), true
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
			if m.loading && (key == "esc" || key == "e" || key == "r" || key == "a" || key == "s") {
				m.cancelJob()
				return m, nil
			}
			rows := m.resultItems()
			switch key {
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
				m.input = []rune(m.resultSearch)
				m.cursor = len(m.input)
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

func (m Model) updateInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.searching {
			m.input = nil
			m.syncSearch()
		}
		m.editing, m.searching = false, false
	case "enter":
		if m.editing {
			fields := m.visibleFields()
			if len(fields) > 0 {
				field := &m.form.Fields[fields[m.focus]]
				field.Value, field.Dirty = string(m.input), true
				m.err = ""
			}
		}
		m.editing, m.searching = false, false
	case "left":
		m.cursor = max(0, m.cursor-1)
	case "right":
		m.cursor = min(len(m.input), m.cursor+1)
	case "home", "ctrl+a":
		m.cursor = 0
	case "end", "ctrl+e":
		m.cursor = len(m.input)
	case "backspace":
		if m.cursor > 0 {
			m.input = slices.Delete(m.input, m.cursor-1, m.cursor)
			m.cursor--
		}
	case "delete":
		if m.cursor < len(m.input) {
			m.input = slices.Delete(m.input, m.cursor, m.cursor+1)
		}
	case "ctrl+u":
		m.input, m.cursor = nil, 0
	case "ctrl+k":
		m.input = m.input[:m.cursor]
	case "ctrl+w":
		start := m.cursor
		for start > 0 && m.input[start-1] == ' ' {
			start--
		}
		for start > 0 && m.input[start-1] != ' ' {
			start--
		}
		m.input = slices.Delete(m.input, start, m.cursor)
		m.cursor = start
	case "tab":
		if m.editing {
			m.completePath()
		}
	default:
		if msg.Text != "" {
			m.insert(msg.Text)
		}
	}
	m.syncSearch()
	return m, nil
}

func (m *Model) insert(value string) {
	runes := []rune(value)
	m.input = slices.Insert(m.input, m.cursor, runes...)
	m.cursor += len(runes)
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
