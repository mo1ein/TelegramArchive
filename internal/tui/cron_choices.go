package tui

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type CronJobChoice struct {
	Name, Schedule string
	Installed      bool
}
type CronJobLoader func(context.Context) ([]CronJobChoice, error)

func WithCronJobLoader(loader CronJobLoader) Option { return func(m *Model) { m.cronLoader = loader } }

type cronJobsLoadedMsg struct {
	picker  *choicePicker
	request uint64
	jobs    []CronJobChoice
	err     error
}

func (m *Model) loadCronJobs() tea.Cmd {
	p := m.picker
	if p == nil || !p.jobs {
		return nil
	}
	if m.cronCancel != nil {
		m.cronCancel()
	}
	m.cronRequest++
	request := m.cronRequest
	p.loading, p.loadErr, p.err = true, "", ""
	// Do not allow selecting an old job while a fresh list is being fetched.
	p.choices, p.labels, p.cursor = nil, nil, 0
	if m.cronLoader == nil {
		p.loading = false
		p.loadErr = "Job listing is unavailable in this build."
		return nil
	}
	ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
	m.cronCancel = cancel
	loader := m.cronLoader
	return func() tea.Msg {
		defer cancel()
		jobs, err := loader(ctx)
		if err == nil && ctx.Err() != nil {
			err = ctx.Err()
		}
		return cronJobsLoadedMsg{p, request, jobs, err}
	}
}

func (m *Model) cronJobsLoaded(msg cronJobsLoadedMsg) {
	if m.picker != msg.picker || m.cronRequest != msg.request {
		return
	}
	p := m.picker
	p.loading = false
	if msg.err != nil {
		p.loadErr = "Could not load jobs: " + safeText(msg.err.Error())
		return
	}
	p.labels = map[string]string{}
	for _, job := range msg.jobs {
		p.choices = append(p.choices, job.Name)
		label := safeText(job.Name) + " · " + safeText(job.Schedule)
		if !job.Installed {
			label += " · not in crontab"
		}
		p.labels[job.Name] = label
	}
	slices.Sort(p.choices)
	if len(p.choices) == 0 {
		p.loadErr = "No scheduled backups. Create one with Schedule a backup first."
	}
	if field := m.form.field(p.field); field != nil {
		field.Choices, field.ChoiceLabels = slices.Clone(p.choices), maps.Clone(p.labels)
	}
}

func (m *Model) acceptCronJob() tea.Cmd {
	if m.picker == nil || !m.picker.jobs {
		return nil
	}
	if m.picker.loading {
		m.picker.err = "Wait for the job list to finish loading."
		return nil
	}
	if len(m.picker.filtered()) == 0 {
		m.picker.err = "Choose a job from the list."
		return nil
	}
	m.applyPicker()
	if m.picker != nil {
		return nil
	}
	m.prepareReview()
	if m.screen != review {
		return nil
	}
	if strings.Join(m.plan.Path, " ") == "cron logs" {
		m.plan.Title = fmt.Sprintf("Logs · %s", m.form.field("arguments").Value)
		return m.begin()
	}
	return nil
}
