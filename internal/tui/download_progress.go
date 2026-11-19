package tui

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The upstream downloader exposes terminal trackers rather than progress events.
// Adapt only its recognized rows; prompts, errors and unfamiliar output remain
// in Activity. Keeping this boundary here avoids copying the transfer engine.
var downloadTracker = regexp.MustCompile(`^(.*?) \.\.\.\s+([0-9]+(?:\.[0-9]+)?%|\?\?\?)\s+\[[#.<> ]*\](?: \[(.*)\])?\s*$`)
var downloadOverall = regexp.MustCompile(`^\[[#.<> ]+\](?: \[.*\])?\s*$`)

type downloadTransfer struct {
	label, detail string
	percent       float64
	determinate   bool
	meter         progress.Model
}

func (p Plan) downloadProgress() bool {
	return strings.Join(p.Path, " ") == "download" && !slices.Contains(p.Args, "--serve=true") && !slices.Contains(p.Args, "--serve")
}

func (p Plan) terminalSize(width, height int) (int, int) {
	// Preserve the upstream row's measurements even in a narrow workspace.
	// The user-facing cards are independently fitted to the actual terminal.
	if p.downloadProgress() {
		width = max(240, width)
	}
	return max(20, min(width, 500)), max(5, min(height, 200))
}

func parseDownloadProgress(output string) ([]downloadTransfer, string) {
	var transfers []downloadTransfer
	var activity []string
	for _, row := range strings.Split(output, "\n") {
		match := downloadTracker.FindStringSubmatch(row)
		if match == nil {
			activity = append(activity, row)
			continue
		}
		percent, err := strconv.ParseFloat(strings.TrimSuffix(match[2], "%"), 64)
		transfers = append(transfers, downloadTransfer{
			label: strings.TrimSpace(match[1]), detail: strings.ReplaceAll(match[3], "; ", " · "),
			percent: max(0, min(1, percent/100)), determinate: err == nil,
		})
	}
	if len(transfers) > 0 {
		activity = slices.DeleteFunc(activity, func(row string) bool { return downloadOverall.MatchString(row) })
	}
	return transfers, strings.TrimSpace(strings.Join(activity, "\n"))
}

func (m *Model) syncDownloadProgress() []tea.Cmd {
	transfers, activity := parseDownloadProgress(m.output)
	var commands []tea.Cmd
	used := make([]bool, len(m.downloads))
	for i := range transfers {
		row := &transfers[i]
		row.meter = newProgress()
		for j, previous := range m.downloads {
			if !used[j] && previous.label == row.label {
				row.meter = previous.meter
				used[j] = true
				break
			}
		}
		if row.determinate && row.meter.Percent() != row.percent {
			commands = append(commands, row.meter.SetPercent(row.percent))
		}
	}
	m.downloads, m.output = transfers, activity
	m.downloadsStarted = m.downloadsStarted || len(transfers) > 0
	return commands
}

func (m Model) downloadProgressView(width, height int) string {
	inner := max(1, width-4)
	heading := "DOWNLOADS"
	if len(m.downloads) > 0 {
		heading += fmt.Sprintf(" · %d active", len(m.downloads))
	}
	lines := []string{accent.Render(clip(heading, inner))}
	if len(m.downloads) == 0 {
		label := "Connecting and preparing files…"
		if m.downloadsStarted {
			label = "Finalizing downloads…"
		}
		lines = append(lines, dim.Render(clip(m.loader.View()+" "+label, inner)))
	} else {
		// Leave space for the task heading, Activity and cancellation controls.
		count := min(len(m.downloads), max(1, (height-10)/4))
		compact := height < 14
		if compact {
			lines = nil
			count = 1
		}
		for _, row := range m.downloads[:count] {
			if !compact {
				lines = append(lines, "")
			}
			label := row.label
			if _, destination, ok := strings.Cut(label, " -> "); ok {
				label = destination
			}
			lines = append(lines, titleStyle.Render(clip(label, inner)))
			if row.determinate {
				meter := row.meter
				meter.SetWidth(max(1, inner-8))
				lines = append(lines, meter.View()+accent.Render(fmt.Sprintf(" %5.1f%%", row.percent*100)))
			} else {
				lines = append(lines, dim.Render(clip(m.loader.View()+" Waiting for file size…", inner)))
			}
			lines = append(lines, dim.Render(clip(row.detail, inner)))
		}
		if remaining := len(m.downloads) - count; remaining > 0 && !compact {
			lines = append(lines, dim.Render(fmt.Sprintf("+ %d more active downloads", remaining)))
		}
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(line).Padding(0, 1).Width(width).Render(strings.Join(lines, "\n"))
}
