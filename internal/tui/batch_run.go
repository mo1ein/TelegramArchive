package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type accountOutcome struct {
	task   AccountTask
	status string
	err    error
	output string
}

func (m Model) taskPlan() Plan {
	plan := m.plan
	if len(plan.Tasks) > 0 && m.batchIndex < len(plan.Tasks) {
		plan.Args = plan.Tasks[m.batchIndex].Args
		plan.Tasks = nil
	}
	return plan
}

func (m *Model) finishAccount(msg finishedMsg) (tea.Cmd, bool) {
	if len(m.plan.Tasks) == 0 {
		return nil, false
	}
	task := m.plan.Tasks[m.batchIndex]
	status := "completed"
	if msg.err != nil {
		status = "failed"
	}
	if m.cancelling {
		status = "cancelled"
	}
	lines := strings.Split(m.output, "\n")
	if len(lines) > 100 {
		lines = lines[len(lines)-100:]
	}
	output := strings.Join(lines, "\n")
	if len(output) > 32<<10 {
		output = strings.ToValidUTF8(output[len(output)-(32<<10):], "")
	}
	m.batchResults = append(m.batchResults, accountOutcome{task, status, msg.err, output})
	if !m.cancelling && m.ctx.Err() == nil && m.batchIndex+1 < len(m.plan.Tasks) {
		m.batchIndex++
		return m.startTask(), true
	}
	for _, remaining := range m.plan.Tasks[m.batchIndex+1:] {
		m.batchResults = append(m.batchResults, accountOutcome{task: remaining, status: "not run"})
	}
	m.data = resultData{Kind: "account backups"}
	var failures []error
	var activity []string
	for _, outcome := range m.batchResults {
		detail := "Account: " + safeText(outcome.task.Account) + "\nFolder: " + safeText(outcome.task.Directory) + "\nStatus: " + outcome.status
		if outcome.err != nil {
			detail += "\n" + outcome.err.Error()
			failures = append(failures, fmt.Errorf("%s: %w", safeText(outcome.task.Account), outcome.err))
		}
		if outcome.output != "" {
			detail += "\n\n" + outcome.output
		}
		m.data.Items = append(m.data.Items, item{ID: outcome.task.Account, Title: safeText(outcome.task.Account), Subtitle: outcome.status + " · " + safeText(outcome.task.Directory), Detail: detail, Status: outcome.status})
		activity = append(activity, detail)
	}
	m.output = strings.Join(activity, "\n\n────────────────\n\n")
	m.last = finishedMsg{err: errors.Join(failures...), elapsed: time.Since(m.batchStarted)}
	m.screen, m.resultTab, m.resultSelected, m.scroll = result, 0, 0, 0
	m.responding, m.loading, m.buttonsFocused = false, false, false
	m.status, m.progress = nil, nil
	return nil, true
}
