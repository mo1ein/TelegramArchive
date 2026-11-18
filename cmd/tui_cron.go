package cmd

import (
	"context"
	managedcron "github.com/mo1ein/TelegramArchive/internal/cron"
	"github.com/mo1ein/TelegramArchive/internal/tui"
)

// Use the same manager and metadata as `cron list`, without executing jobs
// or changing the crontab.
func tuiCronJobs(manager managedcron.Manager) tui.CronJobLoader {
	return func(ctx context.Context) ([]tui.CronJobChoice, error) {
		jobs, err := manager.List(ctx)
		if err != nil {
			return nil, err
		}
		choices := make([]tui.CronJobChoice, 0, len(jobs))
		for _, job := range jobs {
			schedule := job.Schedule
			if job.AtStartup {
				schedule += " + startup"
			}
			choices = append(choices, tui.CronJobChoice{Name: job.Name, Schedule: schedule, Installed: job.Installed})
		}
		return choices, nil
	}
}
