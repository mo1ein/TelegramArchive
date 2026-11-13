package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/progress"
	"charm.land/lipgloss/v2"
)

func newProgress() progress.Model {
	return progress.New(progress.WithColors(mint, violet), progress.WithoutPercentage())
}

func (m Model) progressView(width int) string {
	status := m.status
	inner := max(1, width-4)
	lines := []string{titleStyle.Render(clip(status.Label, inner))}
	if status.Determinate {
		meter := m.meter
		meter.SetWidth(max(1, inner-8))
		bar := meter.View()
		if meter.Percent() != status.Percent {
			bar = meter.ViewAs(status.Percent)
		}
		lines = append(lines, bar+accent.Render(fmt.Sprintf(" %5.1f%%", status.Percent*100)))
	} else {
		lines = append(lines, accent.Render(m.loader.View()+" Discovering messages and media…"))
	}
	detail := status.Detail
	if status.Determinate && status.TotalFiles > 0 {
		detail = fmt.Sprintf("%d / %d files   ·   %s", status.CompletedFiles, status.TotalFiles, detail)
	}
	lines = append(lines, textStyle.Render(clip(detail, inner)), dim.Render(clip(status.Summary, inner)))
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(line).Padding(0, 1).Width(width).Render(strings.Join(lines, "\n"))
}
