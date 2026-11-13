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
			right := dim.Render("namespace ") + accent.Render(clip(namespace, 18))
			header += strings.Repeat(" ", max(1, width-lipgloss.Width(header)-lipgloss.Width(right))) + right
		}
		var body string
		if m.help {
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
	nav.WriteString(dim.Render("WORKSPACE") + "\n\n")
	for i, name := range Groups {
		label := "  " + name
		if i == m.group {
			label = "▸ " + name
			nav.WriteString(selectedStyle.Width(sideWidth).Render(label))
		} else {
			nav.WriteString(dim.Render(label))
		}
		nav.WriteString("\n\n")
	}
	if height >= 23 {
		nav.WriteString("\n" + dim.Render("YOUR NEXT BACKUP,\nA FEW KEYS AWAY.\n\n/ Search everything\n? Keyboard guide"))
	}
	mainWidth := width - sideWidth - 3
	body := m.actionList(mainWidth, height)
	return lipgloss.JoinHorizontal(lipgloss.Top, fit(nav.String(), sideWidth, height), "   ", body)
}

func (m Model) actionList(width, height int) string {
	actions := m.filteredActions()
	heading := "Choose a workflow"
	if m.search != "" {
		heading = "Search results"
	} else if Groups[m.group] != "Quick start" {
		heading = Groups[m.group]
	}
	count := dim.Render(fmt.Sprintf("%d available", len(actions)))
	header := titleStyle.Render(heading)
	header += strings.Repeat(" ", max(1, width-lipgloss.Width(header)-lipgloss.Width(count))) + count
	search := dim.Render("/  Search commands, features and descriptions…")
	if m.search != "" {
		search = accent.Render("/  ") + textStyle.Render(m.search)
	}
	if m.searching {
		search = accent.Render("/  ") + m.inputView(width-4, false)
	}
	lines := []string{header, "", clip(search, width), ""}
	if len(actions) == 0 {
		return strings.Join(append(lines, dim.Render("No matches. Try “archive”, “cron” or “login”.")), "\n")
	}
	rowHeight := 3
	if height < 23 {
		rowHeight = 2
	}
	page := max(1, (height-6)/rowHeight)
	start := max(0, m.selected-page+1)
	for i := start; i < min(len(actions), start+page); i++ {
		action := actions[i]
		name := "  " + action.Title
		if i == m.selected {
			name = "▸ " + action.Title
		}
		if i == m.selected {
			lines = append(lines, selectedStyle.Bold(true).Width(width).Render(clip(name, width)))
		} else {
			lines = append(lines, textStyle.Render(clip(name, width)))
		}
		if rowHeight == 3 {
			lines = append(lines, dim.Render(clip("  "+action.Description, width)))
		}
		lines = append(lines, "")
	}
	lines = append(lines, dim.Render(fmt.Sprintf("%02d / %02d", m.selected+1, len(actions)))+"  "+dim.Render("Enter to open · Review before making changes"))
	return strings.Join(lines, "\n")
}

func (m Model) formView(width, height int) string {
	fields := m.visibleFields()
	top := accent.Render("01 CONFIGURE") + dim.Render("   ───   02 REVIEW   ───   03 RUN")
	heading := titleStyle.Render(m.form.Action.Title)
	mode := "Common options"
	if m.advanced {
		mode = "All options"
	}
	if m.fieldSearch != "" {
		mode = "Matching options"
	}
	unit := "fields"
	if len(fields) == 1 {
		unit = "field"
	}
	status := dim.Render(fmt.Sprintf("%s · %d %s · Ctrl+A to switch", mode, len(fields), unit))
	if m.searching {
		status = accent.Render("/ ") + m.inputView(width-3, false)
	}
	lines := []string{top, "", heading, clip(status, width), ""}
	page := max(1, height-11)
	start := max(0, m.focus-page+1)
	labelWidth := min(25, width/2)
	for n := start; n < min(len(fields), start+page); n++ {
		field := m.form.Fields[fields[n]]
		marker := "  "
		if n == m.focus {
			marker = "▸ "
		}
		label := field.Label
		if field.Required {
			label += " *"
		}
		label = lipgloss.NewStyle().Width(labelWidth).Render(clip(marker+label, labelWidth))
		value := field.Display()
		if m.editing && n == m.focus {
			value = m.inputView(width-labelWidth-4, field.Secret)
		}
		change := " "
		if field.Dirty {
			change = "•"
		}
		row := label + " " + change + " " + clip(value, width-labelWidth-3)
		if n == m.focus {
			lines = append(lines, selectedStyle.Width(width).Render(row))
		} else {
			lines = append(lines, textStyle.Render(row))
		}
	}
	if len(fields) == 0 {
		lines = append(lines, dim.Render("No matching options. Esc clears the search."))
	}
	for len(lines) < height-5 {
		lines = append(lines, "")
	}
	if len(fields) > 0 {
		field := m.form.Fields[fields[m.focus]]
		help := field.Help
		if field.Type == "stringArray" || field.Type == "stringSlice" {
			help += " Comma-separated values; quote a value containing a comma."
		}
		if len(field.Choices) > 0 {
			help += " Choices: " + strings.Join(field.Choices, ", ") + "."
		}
		lines = append(lines, dim.Render(strings.Repeat("─", width)))
		lines = append(lines, dim.Render(fit(ansi.Wrap(help, width, ""), width, 2)))
		lines = append(lines, dim.Render(fmt.Sprintf("%d / %d", m.focus+1, len(fields)))+"  "+accent.Render("Ctrl+R  Review command"))
	}
	if m.err != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#FFAFB8")).Render(clip("! "+m.err, width)))
	}
	return strings.Join(lines, "\n")
}

func (m Model) reviewView(width, height int) string {
	lines := []string{accent.Render("READY TO START"), "", titleStyle.Render(m.plan.Title), ""}
	for _, field := range m.form.Fields {
		if field.Name == "arguments" && field.Value == "" {
			continue
		}
		if !field.Dirty {
			continue
		}
		lines = append(lines, dim.Render(field.Label+"  ")+textStyle.Render(clip(field.Display(), max(1, width-len(field.Label)-2))))
	}
	if len(lines) == 4 {
		lines = append(lines, dim.Render("Using the default settings for this workflow."))
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
	content := titleStyle.Render("A few keys. Every command.") + "\n\n" + accent.Render("WORKSPACE") + "\n↑/↓ or j/k   Choose a command\nTab / ←/→    Change section\n/            Search across every command\nEnter        Configure the selected command\n\n" + accent.Render("OPTIONS") + "\n↑/↓ or Tab   Move between fields\nEnter        Edit text or toggle a switch\nSpace / ←/→  Toggle a switch or cycle choices\nCtrl+A       Show or hide advanced options\n/            Find any option, including advanced ones\nCtrl+D       Reset a field to its inherited default\nCtrl+R       Validate and review the command\n\n" + accent.Render("EDITING") + "\nEnter saves · Esc cancels · Ctrl+U clears\nHome/End · ←/→ · Ctrl+W delete word\nTab completes a unique file or directory path\n\nRESULTS & TASKS\nTab / Shift+Tab   Focus action buttons\nEnter / click    Activate the focused button\n↑/↓ · /          Select or search results\nt / l / d        Results / Activity / Details\ni                Respond to an active prompt (Esc ends input)\nCtrl+C           Cancel active task; keep workspace open\n\nEsc goes back · Ctrl+C exits when idle · ? closes this guide"
	return scrolled(content, width, height, m.scroll)
}

func (m Model) footer() string {
	if m.help {
		return dim.Render("↑↓ scroll   Esc / ? close guide")
	}
	if m.editing {
		return accent.Render("Enter save") + dim.Render("   Esc cancel   Ctrl+U clear   Tab complete path")
	}
	if m.searching {
		return accent.Render("Type to search") + dim.Render("   Enter finish   Esc clear")
	}
	switch m.screen {
	case configure:
		return accent.Render("Enter edit") + dim.Render("   ↑↓ move   Ctrl+A advanced   Ctrl+R review   F6 buttons   Esc back   ? help")
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
	value := string(m.input)
	if secret {
		value = strings.Repeat("•", len(m.input))
	}
	runes := []rune(value)
	start := max(0, m.cursor-max(1, width-3))
	before := string(runes[start:m.cursor])
	current, after := " ", ""
	if m.cursor < len(runes) {
		current, after = string(runes[m.cursor]), string(runes[m.cursor+1:])
	}
	return clip(before+lipgloss.NewStyle().Reverse(true).Render(current)+after, width)
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
