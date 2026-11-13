package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) taskView(width, height int) string {
	if m.screen == running {
		spinner := []string{"◐", "◓", "◑", "◒"}[int(time.Since(m.started).Milliseconds()/150)%4]
		heading := spinner + " Working"
		if m.stage != "" {
			heading = spinner + " " + m.stage
		}
		if m.cancelling {
			heading = "◐ Cancelling…"
		}
		lines := []string{accent.Render(heading) + dim.Render("   "+time.Since(m.started).Round(time.Second).String()), "", titleStyle.Render(m.plan.Title), dim.Render("Task stays here · Ctrl+C cancels without closing the workspace"), ""}
		if m.responding {
			lines = append(lines, accent.Render("INPUT ACTIVE · Type your response; Enter submits · Esc returns to controls"))
		} else {
			lines = append(lines, dim.Render("Choose Respond (i) if the task asks for input. Passwords stay masked by the prompt."))
		}
		lines = append(lines, "")
		if len(m.progress) == 3 {
			lines = append(lines, selectedStyle.Width(width).Render(m.progress[0]), accent.Render(m.progress[1]), dim.Render(m.progress[2]), "")
		}
		output := m.output
		if output == "" {
			if len(m.progress) > 0 {
				output = "Activity will appear here as files finish."
			} else {
				output = "Preparing the task…"
				if len(m.plan.Path) > 0 && m.plan.Path[0] == "chat" {
					output = "Connecting to Telegram · the browser will show each page as it arrives."
				}
			}
		}
		available := max(1, height-len(lines)-2)
		offset := m.scroll
		if m.follow {
			offset = max(0, len(strings.Split(output, "\n"))-available)
		}
		lines = append(lines, scrolled(output, width, available, offset))
		if m.err != "" {
			lines = append(lines, warning.Render(clip(m.err, width)))
		}
		return strings.Join(lines, "\n")
	}
	status := "✓ Completed"
	if !m.cachedAt.IsZero() {
		status = "✓ Chat list · saved " + m.cachedAt.Format("15:04:05")
	}
	if m.loading {
		status = fmt.Sprintf("◐ Loading · %d chats available", len(m.data.Items))
	}
	style := accent
	if m.last.err != nil {
		status = "! Task stopped"
		style = warning
	}
	if m.cancelling {
		status = "■ Cancelled"
	}
	elapsed := m.last.elapsed
	if m.loading {
		elapsed = time.Since(m.started)
	}
	elapsedLabel := "   " + elapsed.Round(time.Second).String()
	if !m.cachedAt.IsZero() {
		elapsedLabel = ""
	}
	lines := []string{style.Render(status) + dim.Render(elapsedLabel), "", titleStyle.Render(m.plan.Title), ""}
	if m.loading {
		lines[3] = dim.Render(clip(m.stage+" · Stop loading to use a selected chat", width))
	}
	tabs := []string{"Results", "Activity", "Details"}
	parts := []string{}
	for i, label := range tabs {
		if i == m.resultTab {
			parts = append(parts, accent.Render(label))
		} else {
			parts = append(parts, dim.Render(label))
		}
	}
	lines = append(lines, strings.Join(parts, "    ")+dim.Render("    t / l / d"), "")
	if m.last.err != nil && !m.cancelling {
		lines[3] = warning.Render(clip(m.last.err.Error(), width))
	}
	switch m.resultTab {
	case 1:
		content := m.output
		if content == "" {
			content = "The task produced no output."
		}
		lines = append(lines, scrolled(content, width, max(1, height-len(lines)-1), m.scroll))
	case 2:
		rows := m.resultItems()
		content := "No item selected."
		if m.last.err != nil {
			content = m.last.err.Error()
		}
		if len(rows) > 0 && m.resultSelected < len(rows) {
			content = rows[m.resultSelected].Detail
		}
		lines = append(lines, scrolled(ansi.Hardwrap(content, width, false), width, max(1, height-len(lines)-1), m.scroll))
	default:
		if len(m.data.Metrics) > 0 {
			cards := []string{}
			cellWidth := max(12, width/3-1)
			for _, v := range m.data.Metrics[:min(3, len(m.data.Metrics))] {
				cards = append(cards, selectedStyle.Width(cellWidth).Render(clip(" "+v.Value+" "+v.Label, cellWidth)))
			}
			lines = append(lines, strings.Join(cards, " "), "")
		}
		rows := m.resultItems()
		search := dim.Render(fmt.Sprintf("%d items · / search · ↑↓ select · Enter details", len(rows)))
		if m.searching {
			search = accent.Render("/ ") + m.inputView(width-3, false)
		} else if m.resultSearch != "" {
			search = accent.Render("/ "+m.resultSearch) + dim.Render(fmt.Sprintf(" · %d matches", len(rows)))
		}
		lines = append(lines, clip(search, width))
		page := max(1, (height-len(lines)-2)/2)
		start := max(0, m.resultSelected-page+1)
		for i := start; i < min(len(rows), start+page); i++ {
			row := rows[i]
			marker := "  "
			style := textStyle
			if i == m.resultSelected {
				marker = "▸ "
				style = selectedStyle.Bold(true).Width(width)
			}
			lines = append(lines, style.Render(clip(marker+row.Title, width)), dim.Render(clip("  "+row.Subtitle, width)))
		}
		if len(rows) == 0 && len(m.data.Metrics) == 0 {
			message := "No results. Try changing the filters or refreshing."
			switch m.data.Kind {
			case "accounts":
				message = "No saved accounts. Connect an account from the Accounts section."
			case "cron list":
				message = "No scheduled backups. Choose Schedule a backup from Quick start."
			}
			lines = append(lines, "", dim.Render(ansi.Hardwrap(message, width, false)))
		}
	}
	if m.err != "" {
		lines = append(lines, warning.Render(clip(m.err, width)))
	}
	return fit(strings.Join(lines, "\n"), width, height)
}

var warning = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFAFB8"))
