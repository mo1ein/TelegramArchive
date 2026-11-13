package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

type pathEntry struct {
	name, path string
	directory  bool
}

type pathBrowser struct {
	directory   string
	entries     []pathEntry
	cursor      int
	revision    uint64
	loading     bool
	showHidden  bool
	searching   bool
	naming      bool
	err         string
	query, name textinput.Model
	selected    map[string]bool
}

type pathDirectoryMsg struct {
	dialog    *pathDialog
	revision  uint64
	directory string
	entries   []pathEntry
	err       error
}

func (m *Model) openPathBrowser() tea.Cmd {
	directory, name, err := pathLocation(m.pathDialog.request.Initial)
	b := &pathBrowser{directory: directory, query: textinput.New(), name: textinput.New(), selected: map[string]bool{}}
	for _, input := range []*textinput.Model{&b.query, &b.name} {
		input.Prompt, input.CharLimit = "", 4096
		input.SetVirtualCursor(true)
		styles := textinput.DefaultDarkStyles()
		styles.Focused.Text, styles.Cursor.Blink = textStyle, false
		input.SetStyles(styles)
	}
	b.query.Placeholder = "Type to find a file or folder…"
	b.query.SetWidth(max(1, m.width-7))
	b.name.SetWidth(max(1, m.width-16))
	if m.pathDialog.request.Initial == "" {
		name = "backup.ta"
	}
	b.name.SetValue(name)
	m.pathDialog.browser = b
	if err != nil {
		b.err = safeText(err.Error())
		b.directory, _ = os.Getwd()
		return nil
	}
	return m.readPathDirectory(directory)
}

func (m *Model) readPathDirectory(directory string) tea.Cmd {
	dialog, b := m.pathDialog, m.pathDialog.browser
	b.revision++
	b.loading, b.err, b.directory, b.cursor, b.entries = true, "", directory, 0, nil
	b.query.SetValue("")
	b.searching = false
	revision := b.revision
	return func() tea.Msg {
		msg := pathDirectoryMsg{dialog: dialog, revision: revision, directory: directory}
		if msg.err = dialog.ctx.Err(); msg.err != nil {
			return msg
		}
		entries, err := os.ReadDir(directory)
		msg.err = err
		for _, entry := range entries {
			path, isDir := filepath.Join(directory, entry.Name()), entry.IsDir()
			if entry.Type()&os.ModeSymlink != 0 {
				if info, err := os.Stat(path); err == nil {
					isDir = info.IsDir()
				}
			}
			if dialog.request.Kind == "directory" && !isDir {
				continue
			}
			msg.entries = append(msg.entries, pathEntry{entry.Name(), path, isDir})
		}
		slices.SortStableFunc(msg.entries, func(a, b pathEntry) int {
			if a.directory != b.directory {
				if a.directory {
					return -1
				}
				return 1
			}
			return strings.Compare(a.name, b.name)
		})
		return msg
	}
}

func (b *pathBrowser) filtered() []pathEntry {
	var entries []pathEntry
	for _, entry := range b.entries {
		if (!strings.HasPrefix(entry.name, ".") || b.showHidden) && matches(entry.name, b.query.Value()) {
			entries = append(entries, entry)
		}
	}
	return entries
}

func (d *pathDialog) canSelect(entry pathEntry) bool {
	return d.request.Kind == "any" || (d.request.Kind == "directory" && entry.directory) || (d.request.Kind != "directory" && !entry.directory)
}

func (m Model) pathButtons() []button {
	if m.pathDialog.browser == nil {
		return []button{{"path-cancel", "Cancel chooser"}}
	}
	label := "Choose file"
	switch m.pathDialog.request.Kind {
	case "directory", "any":
		label = "Use this folder"
	case "save":
		label = "Save here"
	}
	if m.pathDialog.request.Multiple {
		label = "Use selected"
	}
	return []button{{"path-open", "Open"}, {"path-choose", label}, {"path-up", "Up"}, {"path-home", "Home"}, {"path-cancel", "Cancel"}}
}

func (m *Model) pathAction(id string) tea.Cmd {
	dialog := m.pathDialog
	if id == "path-cancel" {
		m.closePathDialog()
		m.buttonsFocused = false
		return nil
	}
	b := dialog.browser
	if b == nil || b.loading {
		return nil
	}
	rows := b.filtered()
	if b.cursor >= len(rows) {
		b.cursor = max(0, len(rows)-1)
	}
	switch id {
	case "path-up":
		return m.readPathDirectory(filepath.Dir(b.directory))
	case "path-home":
		home, err := os.UserHomeDir()
		if err != nil {
			b.err = safeText(err.Error())
			return nil
		}
		return m.readPathDirectory(home)
	case "path-open":
		if len(rows) > 0 {
			row := rows[b.cursor]
			if row.directory {
				return m.readPathDirectory(row.path)
			}
			if dialog.request.Kind == "save" {
				b.name.SetValue(row.name)
				return nil
			}
			if dialog.request.Multiple {
				b.selected[row.path] = !b.selected[row.path]
			} else {
				m.acceptPaths([]string{row.path})
			}
		}
	case "path-choose":
		var paths []string
		if dialog.request.Multiple {
			for path, checked := range b.selected {
				if checked {
					paths = append(paths, path)
				}
			}
			slices.Sort(paths)
		} else {
			switch dialog.request.Kind {
			case "directory", "any":
				paths = []string{b.directory}
			case "save":
				name := b.name.Value()
				if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
					b.err = "Enter a filename with F2. Choose the folder by browsing."
					return nil
				}
				paths = []string{filepath.Join(b.directory, name)}
			default:
				if len(rows) > 0 && !rows[b.cursor].directory {
					paths = []string{rows[b.cursor].path}
				}
			}
		}
		if len(paths) == 0 {
			b.err = "Select at least one file or folder with Space."
			return nil
		}
		m.acceptPaths(paths)
	}
	return nil
}

func (m Model) updatePathDialog(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "esc" || key == "ctrl+c" {
		m.closePathDialog()
		return m, nil
	}
	b := m.pathDialog.browser
	if b == nil {
		return m, nil
	}
	if b.naming {
		switch key {
		case "enter", "tab":
			b.naming = false
			b.name.Blur()
		default:
			b.name, _ = b.name.Update(msg)
		}
		return m, nil
	}
	if key == "tab" || key == "shift+tab" || key == "f6" {
		if !m.buttonsFocused {
			m.buttonsFocused, m.buttonFocus = true, 0
		} else {
			delta := 1
			if key == "shift+tab" {
				delta = -1
			}
			m.buttonFocus = (m.buttonFocus + len(m.buttons()) + delta) % len(m.buttons())
		}
		return m, nil
	}
	if m.buttonsFocused && key == "enter" {
		return m, m.pathAction(m.buttons()[m.buttonFocus].ID)
	}
	m.buttonsFocused = false
	rows := b.filtered()
	switch key {
	case "up", "down", "pgup", "pgdown", "home", "end":
		if b.searching {
			b.searching = false
			b.query.Blur()
			return m, nil
		}
		delta := 1
		switch key {
		case "up":
			delta = -1
		case "pgup":
			delta = -max(1, m.height-17)
		case "pgdown":
			delta = max(1, m.height-17)
		case "home":
			b.cursor = 0
			return m, nil
		case "end":
			b.cursor = max(0, len(rows)-1)
			return m, nil
		}
		b.cursor = max(0, min(max(0, len(rows)-1), b.cursor+delta))
	case "enter", "right":
		b.searching = false
		return m, m.pathAction("path-open")
	case "left", "backspace":
		if b.searching {
			b.query, _ = b.query.Update(msg)
			b.cursor = 0
		} else {
			return m, m.pathAction("path-up")
		}
	case "ctrl+enter":
		return m, m.pathAction("path-choose")
	case "ctrl+h":
		b.showHidden, b.cursor = !b.showHidden, 0
	case "f2":
		if m.pathDialog.request.Kind == "save" {
			b.naming = true
			b.name.Focus()
		}
	case "space", " ":
		if !b.searching && len(rows) > 0 {
			row := rows[b.cursor]
			if !m.pathDialog.canSelect(row) {
				return m, m.pathAction("path-open")
			}
			if m.pathDialog.request.Multiple {
				b.selected[row.path] = !b.selected[row.path]
			} else if m.pathDialog.request.Kind != "save" {
				m.acceptPaths([]string{row.path})
			}
			return m, nil
		}
		fallthrough
	default:
		if msg.Text != "" || key == "ctrl+u" || key == "delete" {
			b.searching = true
			b.query.Focus()
			b.query, _ = b.query.Update(msg)
			b.cursor = 0
		}
	}
	return m, nil
}

func (m Model) pathDialogView(width, height int) string {
	d := m.pathDialog
	caption := "Choose a file"
	if d.request.Kind == "directory" {
		caption = "Choose a folder"
	} else if d.request.Kind == "save" {
		caption = "Choose where to save"
	}
	lines := []string{titleStyle.Render(caption + " · " + d.request.Title)}
	if d.browser == nil {
		return strings.Join(append(lines, "", accent.Render("Opening the macOS chooser…"), "", dim.Render("Choose a location in the system dialog, or cancel to keep the current path.")), "\n")
	}
	b := d.browser
	query, name := b.query, b.name
	query.SetWidth(max(1, width-3))
	name.SetWidth(max(1, width-12))
	lines = append(lines, dim.Render("Enter opens · Space selects · Ctrl+Enter confirms · Ctrl+H hidden"), accent.Render(clip("Location  "+safeText(b.directory), width)), dim.Render("⌕ ")+query.View(), "")
	rows, page := b.filtered(), max(1, height-8)
	start := max(0, b.cursor-page+1)
	var list []string
	for index := start; index < min(len(rows), start+page); index++ {
		row := rows[index]
		mark, style := "  ", textStyle
		if d.request.Multiple && d.canSelect(row) {
			mark = "○ "
			if b.selected[row.path] {
				mark = "✓ "
			}
		}
		label := safeText(row.name)
		if row.directory {
			label += "/"
			style = accent
		}
		pointer := "  "
		if index == b.cursor {
			pointer, style = "▸ ", selectedStyle.Bold(true).Width(width)
		}
		list = append(list, style.Render(clip(pointer+mark+label, width)))
	}
	if len(rows) == 0 {
		text := "No matching files or folders. Try clearing the search or showing hidden items."
		if b.loading {
			text = "Loading folder…"
		}
		list = append(list, dim.Render(ansi.Wrap(text, width, "")))
	}
	lines = append(lines, fit(strings.Join(list, "\n"), width, page))
	filename := ""
	if d.request.Kind == "save" {
		filename = dim.Render("Filename  ") + name.View() + dim.Render("  F2 to edit")
	}
	lines = append(lines, clip(filename, width))
	count := fmt.Sprintf("%d items", len(rows))
	if d.request.Multiple {
		selected := 0
		for _, checked := range b.selected {
			if checked {
				selected++
			}
		}
		count += fmt.Sprintf(" · %d selected", selected)
	}
	lines = append(lines, dim.Render(count), warning.Render(clip(b.err, width)))
	return strings.Join(lines, "\n")
}

func (m Model) pathDialogClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft {
		return m, nil
	}
	if msg.Y == m.height-5 {
		return m.mouseClick(msg)
	}
	b := m.pathDialog.browser
	if b == nil || b.loading {
		return m, nil
	}
	if msg.Y == 6 {
		b.searching = true
		b.query.Focus()
		return m, nil
	}
	page, row := max(1, m.height-17), msg.Y-8
	rows := b.filtered()
	index := max(0, b.cursor-page+1) + row
	if row >= 0 && row < page && index < len(rows) {
		b.cursor, b.searching = index, false
		b.query.Blur()
		if m.pathDialog.request.Multiple && m.pathDialog.canSelect(rows[index]) && msg.X < 7 {
			b.selected[rows[index].path] = !b.selected[rows[index].path]
			return m, nil
		}
		return m, m.pathAction("path-open")
	}
	return m, nil
}
