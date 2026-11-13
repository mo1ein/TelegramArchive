package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

func (m *Model) startInput(value, placeholder string, secret bool) {
	m.editor = textinput.New()
	m.editor.Prompt = ""
	m.editor.Placeholder = placeholder
	m.editor.CharLimit = 4096
	m.editor.SetVirtualCursor(true)
	styles := textinput.DefaultDarkStyles()
	styles.Focused.Text = textStyle
	styles.Focused.Placeholder = dim
	styles.Cursor.Color, styles.Cursor.Blink = mint, false
	m.editor.SetStyles(styles)
	m.editor.SetWidth(m.editorWidth())
	if secret {
		m.editor.EchoMode = textinput.EchoPassword
		m.editor.EchoCharacter = '•'
	}
	m.editor.SetValue(value)
	m.editor.CursorEnd()
	m.editor.Focus()
	m.input, m.cursor = []rune(value), len([]rune(value))
}

func (m *Model) editField() {
	fields := m.visibleFields()
	if len(fields) == 0 || m.focus >= len(fields) {
		return
	}
	field := m.form.Fields[fields[m.focus]]
	if choiceField(field) {
		m.openPicker(field)
		return
	}
	m.startInput(field.Value, placeholder(field), field.Secret)
	m.editing, m.buttonsFocused = true, false
	m.err = ""
}

func (m *Model) saveInput() bool {
	if !m.editing {
		return true
	}
	fields := m.visibleFields()
	if len(fields) == 0 || m.focus >= len(fields) {
		return false
	}
	field := m.form.Fields[fields[m.focus]]
	field.Value, field.Dirty = string(m.input), true
	err := validate(field)
	if field.Required && strings.TrimSpace(field.Value) == "" {
		err = fmt.Errorf("enter a value")
	}
	if (field.Name == "since" || field.Name == "until") && field.Value != "" && !validDate(field.Value) {
		err = fmt.Errorf("use YYYY-MM-DD, RFC3339 date/time, or Unix seconds")
	}
	if err != nil {
		m.err = field.Error(err).Error()
		return false
	}
	m.form.Fields[fields[m.focus]] = field
	m.form.reconcileSelection(field.Name)
	m.editing, m.err = false, ""
	m.editor.Blur()
	return true
}

func (m *Model) updateEditor(msg tea.Msg) tea.Cmd {
	var command tea.Cmd
	m.editor.SetWidth(m.editorWidth())
	m.editor, command = m.editor.Update(msg)
	m.input, m.cursor = []rune(m.editor.Value()), m.editor.Position()
	m.syncSearch()
	return command
}

func (m Model) editorWidth() int {
	width := m.width - 4
	switch m.screen {
	case configure:
		if m.searching {
			return max(1, width-1)
		}
		layout := m.formLayout(width, m.height-9)
		return max(1, layout.controlWidth-3)
	case dashboard:
		if width >= 86 {
			width -= 23
		}
		return max(1, width-8)
	default:
		return max(1, width-4)
	}
}

func (m Model) updateInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.searching {
			m.input = nil
			m.syncSearch()
		}
		m.editing, m.searching = false, false
		m.editor.Blur()
	case "enter":
		if m.saveInput() {
			m.searching = false
			m.editor.Blur()
		}
	case "tab", "shift+tab", "up", "down":
		if m.searching {
			m.searching = false
			m.editor.Blur()
			return m, nil
		}
		if m.saveInput() {
			delta := 1
			if msg.String() == "shift+tab" || msg.String() == "up" {
				delta = -1
			}
			next := m.focus + delta
			if msg.String() == "tab" && next >= len(m.visibleFields()) {
				m.buttonsFocused, m.buttonFocus = true, 1
			} else {
				m.focus = max(0, min(len(m.visibleFields())-1, next))
			}
		}
	case "ctrl+r", "ctrl+enter":
		if !m.searching && m.saveInput() {
			m.prepareReview()
		}
	case "ctrl+f":
		if m.screen == configure && m.saveInput() {
			m.searching = true
			m.startInput(m.fieldSearch, "Find an option…", false)
		}
	case "alt+/", "ctrl+space":
		if m.editing {
			m.completePath()
			m.editor.SetValue(string(m.input))
			m.editor.CursorEnd()
		}
	case "alt+o":
		if !m.searching {
			fields := m.visibleFields()
			if len(fields) > 0 {
				return m, m.openPathDialog(m.form.Fields[fields[m.focus]])
			}
		}
	default:
		return m, m.updateEditor(msg)
	}
	return m, nil
}

func placeholder(field Field) string {
	switch field.Name {
	case "chat":
		return "@channel, chat ID, or me for Saved Messages"
	case "output", "dir", "path":
		return "Folder path · Alt+/ completes a path"
	case "desktop":
		return "Leave empty to find your installed Telegram app"
	case "since", "until":
		return "YYYY-MM-DD or RFC3339 · empty means no limit"
	case "proxy":
		return "socks5://127.0.0.1:1080 · optional"
	case "arguments":
		if field.Required {
			return "Enter " + field.Label
		}
	}
	if field.Secret {
		return "Hidden while you type"
	}
	return "Optional · leave empty to use the default"
}
