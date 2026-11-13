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
		heading := "Working"
		if m.stage != "" {
			heading = m.stage
		}
		if m.cancelling {
			heading = "Stopping safely…"
		}
		elapsed := time.Since(m.started).Round(time.Second).String()
		title := m.plan.Title
		if len(m.plan.Tasks) > 0 {
			title = fmt.Sprintf("Account %d / %d · %s", m.batchIndex+1, len(m.plan.Tasks), m.plan.Tasks[m.batchIndex].Account)
		}
		lines := []string{accent.Render(m.loader.View()+" "+clip(heading, width-len(elapsed)-4)) + dim.Render("  "+elapsed), titleStyle.Render(clip(title, width)), ""}
		if m.status != nil {
			lines = append(lines, m.progressView(width), "")
		} else if len(m.progress) == 3 {
			lines = append(lines, clip(m.progress[0], width), accent.Render(clip(m.progress[1], width)), dim.Render(clip(m.progress[2], width)), "")
		}
		activity := "ACTIVITY"
		if m.responding {
			activity = "INPUT ACTIVE · Enter submits · Esc returns to controls"
		} else if m.status == nil {
			activity += " · i to respond to a prompt"
		}
		lines = append(lines, dim.Render(clip(activity, width)))
		output := m.output
		if output == "" {
			output = "Preparing your backup…"
			if m.status != nil {
				output = "Files and updates will appear here."
			}
			if len(m.plan.Path) > 0 && m.plan.Path[0] == "chat" {
				output = "Connecting to Telegram. Chats appear as each page arrives."
			}
		}
		used := 0
		for _, value := range lines {
			used += lipgloss.Height(value)
		}
		available := max(1, height-used-1)
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
		status = fmt.Sprintf("%s Loading · %d chats available", m.loader.View(), len(m.data.Items))
	}
	style := accent
	if m.last.err != nil {
		status = "! Task stopped"
		style = warning
	}
	if m.cancelling {
		status = "■ Cancelled"
	}
	if m.data.Kind == "account backups" {
		completed, failed, skipped := 0, 0, 0
		for _, row := range m.data.Items {
			switch row.Status {
			case "completed":
				completed++
			case "failed":
				failed++
			default:
				skipped++
			}
		}
		status = fmt.Sprintf("%d completed · %d failed · %d not run", completed, failed, skipped)
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
		lines[3] = dim.Render(clip(m.stage+" · Select a chat and choose Archive when ready", width))
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
		if m.data.Kind == "accounts" || m.data.Kind == "chat ls" {
			search = dim.Render(fmt.Sprintf("%d items · %d checked · Space check · / search", len(rows), len(m.checkedResults())))
		}
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
			check := ""
			if m.data.Kind == "accounts" || m.data.Kind == "chat ls" {
				check = "[ ] "
				if m.selectedResults[row.ID] {
					check = "[x] "
				}
				if m.data.Kind == "accounts" && row.Status != "ready" {
					check = "[-] "
				}
			}
			style := textStyle
			if i == m.resultSelected {
				marker = "▸ "
				style = selectedStyle.Bold(true).Width(width)
			}
			lines = append(lines, style.Render(clip(marker+check+row.Title, width)), dim.Render(clip("  "+row.Subtitle, width)))
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
