package tui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	ink           = lipgloss.Color("#E3E9F3")
	muted         = lipgloss.Color("#8995AD")
	mint          = lipgloss.Color("#8BE5CD")
	violet        = lipgloss.Color("#B9AAFF")
	line          = lipgloss.Color("#343E55")
	panel         = lipgloss.Color("#191F30")
	textStyle     = lipgloss.NewStyle().Foreground(ink)
	dim           = lipgloss.NewStyle().Foreground(muted)
	accent        = lipgloss.NewStyle().Foreground(mint).Bold(true)
	titleStyle    = lipgloss.NewStyle().Foreground(ink).Bold(true)
	selectedStyle = lipgloss.NewStyle().Background(panel).Foreground(mint)
	badge         = lipgloss.NewStyle().Background(mint).Foreground(lipgloss.Color("#102C2C")).Bold(true).Padding(0, 1)
)

func (m Model) View() tea.View {
	w, h := m.width, m.height
	var content string
	if w < 55 || h < 20 {
		content = "ta · Telegram Archive\n\nResize to at least 55 × 20.\nCtrl+C to exit."
	} else {
		width, height := w-4, h-9
		header := badge.Render("ta") + "  " + titleStyle.Render("TELEGRAM ARCHIVE") + dim.Render("  /  your workspace")
		if w >= 85 {
			namespace := m.globals["ns"]
			if namespace == "" {
				namespace = "inherited"
			}
			right := dim.Render("account ") + accent.Render(clip(namespace, 18))
			header += strings.Repeat(" ", max(1, width-lipgloss.Width(header)-lipgloss.Width(right))) + right
		}
		var body string
		if m.pathDialog != nil {
			body = m.pathDialogView(width, height)
		} else if m.calendar != nil && !m.inlineCalendar() {
			body = m.datePickerView()
		} else if m.picker != nil && !m.inlinePicker() {
			body = m.pickerView(width, height)
		} else if m.help {
			body = m.helpView(width, height)
		} else {
			switch m.screen {
			case dashboard:
				body = m.dashboardView(width, height)
			case configure:
				body = m.formView(width, height)
			case review:
				body = m.reviewView(width, height)
			case result, running:
				body = m.taskView(width, height)
			}
		}
		body = fit(body, width, height) + "\n\n" + m.buttonBar(width)
		footer := m.footer()
		content = "\n  " + header + "\n  " + dim.Render(strings.Repeat("─", width)) + "\n" + indent(body, "  ") + "\n  " + dim.Render(strings.Repeat("─", width)) + "\n  " + clip(footer, width)
	}
	view := tea.NewView(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	if os.Getenv("NO_COLOR") == "" {
		view.BackgroundColor = lipgloss.Color("#101522")
		view.ForegroundColor = ink
	}
	view.WindowTitle = "ta · Telegram Archive"
	return view
}

func (m Model) dashboardView(width, height int) string {
	if width < 86 {
		return dim.Render("‹ ") + accent.Render(Groups[m.group]) + dim.Render(" ›   Tab to change section") + "\n\n" + m.actionList(width, height-2)
	}
	sideWidth := 20
	var nav strings.Builder
	nav.WriteString(dim.Render("YOUR ARCHIVE") + "\n\n")
	for i, name := range Groups {
		display := name
		if name == "Quick start" {
			display = "Start here"
		} else if name == "All commands" {
			display = "Command library"
		}
		label := "  " + display
		if i == m.group {
			label = "▸ " + display
			nav.WriteString(selectedStyle.Width(sideWidth).Render(label))
		} else {
			nav.WriteString(dim.Render(label))
		}
		nav.WriteString("\n\n")
	}
	if height >= 23 {
		nav.WriteString("\n" + dim.Render("/ Search workflows\nTab Change section\nEnter Open selected\n? Keyboard guide"))
	}
	mainWidth := width - sideWidth - 3
	body := m.actionList(mainWidth, height)
	return lipgloss.JoinHorizontal(lipgloss.Top, fit(nav.String(), sideWidth, height), "   ", body)
}

func (m Model) actionList(width, height int) string {
	actions := m.filteredActions()
	heading, description := "What would you like to back up?", "Choose a task. We’ll guide you through the settings."
	if m.search != "" {
		heading, description = "Find your next task", "Search matches names and descriptions across every section."
	} else if Groups[m.group] != "Quick start" {
		heading, description = Groups[m.group], "Every command is available here, including installed extensions."
	}
	count := dim.Render(fmt.Sprintf("%d tasks", len(actions)))
	header := titleStyle.Render(clip(heading, max(1, width-lipgloss.Width(count)-2)))
	header += strings.Repeat(" ", max(1, width-lipgloss.Width(header)-lipgloss.Width(count))) + count
	search := dim.Render("/  Search workflows and commands…")
	if m.search != "" {
		search = accent.Render("/  ") + textStyle.Render(m.search)
	}
	if m.searching {
		search = accent.Render("/  ") + m.inputView(width-7, false)
	}
	search = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(line).Padding(0, 1).Width(width).Render(clip(search, width-4))
	lines := []string{header, dim.Render(clip(description, width)), search, ""}
	if len(actions) == 0 {
		return strings.Join(append(lines, dim.Render("No matches. Try archive, login, or schedule.")), "\n")
	}
	rowHeight, page, start := m.actionListGeometry(height)
	for i := start; i < min(len(actions), start+page); i++ {
		action := actions[i]
		name := "  " + action.Title
		style := textStyle
		if i == m.selected {
			name, style = "▸ "+action.Title, selectedStyle.Bold(true).Width(width)
		}
		lines = append(lines, style.Render(clip(name, width)), dim.Render(clip("  "+action.Description, width)))
		if rowHeight == 3 {
			lines = append(lines, "")
		}
	}
	lines = append(lines, dim.Render(fmt.Sprintf("%d / %d", m.selected+1, len(actions)))+"  "+accent.Render("Enter to continue"))
	return strings.Join(lines, "\n")
}

func (m Model) actionListGeometry(height int) (rowHeight, page, start int) {
	rowHeight = 2
	if height >= 25 {
		rowHeight = 3
	}
	page = max(1, (height-7)/rowHeight)
	start = max(0, m.selected-page+1)
	return
}

func (m Model) reviewView(width, height int) string {
	lines := []string{accent.Render("READY TO START"), "", titleStyle.Render(m.plan.Title), ""}
	for _, field := range m.form.Fields {
		if field.Name == "arguments" && field.Value == "" {
			continue
		}
		if !field.Dirty && field.Advanced {
			continue
		}
		lines = append(lines, dim.Render(field.Label+"  ")+textStyle.Render(clip(field.Display(), max(1, width-len(field.Label)-2))))
	}
	if len(lines) == 4 {
		lines = append(lines, dim.Render("Using the default settings for this workflow."))
	}
	if len(m.plan.Tasks) > 0 {
		lines = append(lines, "", accent.Render(fmt.Sprintf("%d account backups · one at a time", len(m.plan.Tasks))))
		for _, task := range m.plan.Tasks {
			lines = append(lines, textStyle.Render(clip(safeText(task.Account)+" → "+safeText(task.Directory), width)))
		}
	}
	if len(m.plan.Path) > 0 && m.plan.Path[0] == "login" {
		lines = append(lines, "", warning.Render("This replaces the saved login in the selected namespace."))
	}
	if strings.Join(m.plan.Path, " ") == "cron remove" {
		lines = append(lines, "", warning.Render("Remove this schedule? Its archives and logs will be kept."))
	}
	lines = append(lines, "", dim.Render("Results and progress stay in the workspace. You can cancel an active task."), "", accent.Render("Enter to start · Esc to edit")+dim.Render(" · c command preview"))
	if m.showCommand {
		lines = append(lines, "", dim.Render("COMMAND PREVIEW"), ansi.Hardwrap(m.plan.Preview(), width, false))
	}
	return scrolled(strings.Join(lines, "\n"), width, height, m.scroll)
}

func (m Model) helpView(width, height int) string {
	content := titleStyle.Render("A few keys. Every command.") + "\n\n" + accent.Render("WORKSPACE") + "\n↑/↓ or j/k   Choose a command\nTab / ←/→    Change section\n/            Search across every command\nEnter        Configure the selected command\n\n" + accent.Render("OPTIONS") + "\n↑/↓ or Tab   Move between fields\nEnter        Edit text or toggle a switch\nSpace / ←/→  Toggle a switch or cycle choices\nCtrl+A       Show or hide advanced options\nCtrl+F       Find any option, including advanced ones\nCtrl+D       Reset a field to its inherited default\nCtrl+R       Validate and review the command\n\n" + accent.Render("CHOICE DROPDOWNS") + "\nEnter / click  Open the list beside the argument\n↑↓ / wheel     Scroll the list; Enter/Space toggles items\nType to filter; checked items persist while scrolling\n← saves and closes; Tab saves and moves on; Esc undoes\nCtrl+A / Ctrl+D Select all / clear\nCtrl+L         Refresh saved accounts\n\n" + accent.Render("FILES & FOLDERS") + "\nEnter / Browse Open the system file or folder chooser\nAlt+O          Browse while editing a path\nType path      Enter a destination manually\nEsc            Cancel the chooser and keep your input\n\n" + accent.Render("EDITING") + "\nType or paste to edit · Enter saves · Esc undoes\nHome/End · ←/→ · Ctrl+W delete word\nTab saves and moves on · Alt+/ completes a path\n\nRESULTS & TASKS\nTab / Shift+Tab   Focus action buttons\nEnter / click    Activate the focused button\n↑/↓ · /          Select or search results\nt / l / d        Results / Activity / Details\ni                Respond to an active prompt (Esc ends input)\nCtrl+C           Cancel active task; keep workspace open\n\nEsc goes back · Ctrl+C exits when idle · ? closes this guide"
	return scrolled(content, width, height, m.scroll)
}

func (m Model) footer() string {
	if m.calendar != nil {
		return accent.Render("Enter choose · Esc cancel") + dim.Render("   Arrows day   PgUp/PgDn month   I type")
	}
	if m.pathDialog != nil {
		if m.pathDialog.browser == nil {
			return accent.Render("Choose in the system dialog") + dim.Render("   Esc cancel")
		}
		return accent.Render("Enter open · Space select") + dim.Render("   Tab actions   Type to filter   Ctrl+H hidden   Esc cancel")
	}
	if m.picker != nil {
		if m.inlinePicker() {
			if !m.picker.multi {
				return accent.Render("Enter select · Tab next") + dim.Render("   ↑↓ / wheel scroll   Type to filter   Esc undo")
			}
			if m.picker.custom {
				return accent.Render("Enter add/toggle · ← save") + dim.Render("   Add/Done buttons   Esc undo")
			}
			return accent.Render("Enter toggle · ← save") + dim.Render("   Click items/Done   ↑↓ scroll   Esc undo")
		}
		return accent.Render("Space select · Enter apply") + dim.Render("   Type to filter   ↓ list   Ctrl+A all   Esc cancel")
	}
	if m.help {
		return dim.Render("↑↓ scroll   Esc / ? close guide")
	}
	if m.editing {
		if fields := m.visibleFields(); m.screen == configure && len(fields) > 0 && m.focus < len(fields) && m.form.Fields[fields[m.focus]].PathKind != "" {
			return accent.Render("Enter save · Alt+O browse") + dim.Render("   Tab next   Esc undo   Ctrl+U clear")
		}
		return accent.Render("Enter save") + dim.Render("   Tab next   Esc undo   Ctrl+U clear   Alt+/ complete path")
	}
	if m.searching {
		return accent.Render("Type to search") + dim.Render("   Enter finish   Esc clear")
	}
	switch m.screen {
	case configure:
		if fields := m.visibleFields(); len(fields) > 0 && m.focus < len(fields) && m.form.Fields[fields[m.focus]].PathKind != "" {
			return accent.Render("Enter browse · Alt+O chooser") + dim.Render("   Type path   Tab next   Ctrl+R review   Esc back")
		}
		if fields := m.visibleFields(); len(fields) > 0 && m.focus < len(fields) && choiceField(m.form.Fields[fields[m.focus]]) {
			if m.form.Fields[fields[m.focus]].Multi {
				return accent.Render("Click to multi-select") + dim.Render("   Tab next   Ctrl+R review   Esc back")
			}
			return accent.Render("Enter select · Tab next") + dim.Render("   Click a choice   Ctrl+R review   Ctrl+F find option   Esc back")
		}
		return accent.Render("Type to edit") + dim.Render("   Tab next   Ctrl+R review   Ctrl+F find option   Esc back   ? help")
	case review:
		return accent.Render("Enter run") + dim.Render("   Esc edit   ↑↓ scroll   ? help")
	case running:
		if m.responding {
			return accent.Render("Responding") + dim.Render("   Enter submit   Esc controls   Ctrl+C cancel")
		}
		return accent.Render("Tab buttons / Enter activate") + dim.Render("   i respond   ↑↓ scroll   f follow   Ctrl+C cancel")
	case result:
		return accent.Render("↑↓ select · Tab buttons · Enter activate") + dim.Render("   / search   t results   l activity   e edit   Esc home")
	default:
		return accent.Render("Enter select") + dim.Render("   ↑↓ move   Tab section   F6 buttons   / search   ? help   q quit")
	}
}

func (m Model) inputView(width int, secret bool) string {
	editor := m.editor
	editor.SetWidth(max(1, width-1))
	return editor.View()
}

func clip(value string, width int) string { return ansi.Truncate(value, max(1, width), "…") }

func fit(value string, width, height int) string {
	lines := strings.Split(value, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i := range lines {
		lines[i] = clip(lines[i], width)
	}
	return strings.Join(lines[:min(len(lines), max(1, height))], "\n")
}

func scrolled(value string, width, height, offset int) string {
	lines := strings.Split(value, "\n")
	offset = min(offset, max(0, len(lines)-height))
	return fit(strings.Join(lines[offset:], "\n"), width, height)
}

func indent(value, prefix string) string {
	return prefix + strings.ReplaceAll(value, "\n", "\n"+prefix)
}
