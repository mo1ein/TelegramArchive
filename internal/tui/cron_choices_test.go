package tui

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
)

func cronChoiceModel(t *testing.T, operation string, loader CronJobLoader) Model {
	t.Helper()
	root, _ := fixture()
	cron := &cobra.Command{Use: "cron"}
	cmd := &cobra.Command{Use: operation + " NAME", Args: cobra.ExactArgs(1), RunE: func(*cobra.Command, []string) error { panic("must not execute") }}
	cron.AddCommand(cmd)
	root.AddCommand(cron)
	m := New(Catalog(root), nil, "default", t.Context(), WithCronJobLoader(loader))
	m.open(Action{Title: "cron " + operation, Path: []string{"cron", operation}, Command: cmd})
	focusField(&m, "arguments")
	return m
}

func TestCronJobSelectionOpensLogsAndReviewsRemoval(t *testing.T) {
	for _, operation := range []string{"logs", "remove"} {
		for _, mouse := range []bool{false, true} {
			m := cronChoiceModel(t, operation, func(context.Context) ([]CronJobChoice, error) {
				return []CronJobChoice{{Name: "daily", Schedule: "0 1 * * *", Installed: true}, {Name: "weekly", Schedule: "0 1 * * 1"}}, nil
			})
			runs := 0
			m.run = func(_ context.Context, plan Plan) tea.Cmd {
				runs++
				if !slices.Equal(plan.Path, []string{"cron", operation}) || plan.Args[len(plan.Args)-1] != "weekly" {
					t.Fatal(plan.Args)
				}
				return func() tea.Msg { return finishedMsg{} }
			}
			next, load := m.Update(key(tea.KeyEnter, ""))
			m = next.(Model)
			if load == nil || !m.picker.loading || !strings.Contains(ansi.Strip(m.View().Content), "Loading jobs") {
				t.Fatal("job picker did not start loading")
			}
			m = update(m, load())
			if !strings.Contains(m.picker.label("weekly"), "not in crontab") {
				t.Fatal("job status missing")
			}
			if mouse {
				m = clickChoiceText(t, m, "weekly ·")
			} else {
				m = update(m, key(tea.KeyDown, ""))
				m = update(m, key(tea.KeyEnter, ""))
			}
			if operation == "logs" {
				if runs != 1 || m.screen != running {
					t.Fatal("choosing a job did not open its logs")
				}
			} else {
				if runs != 0 || m.screen != review || !strings.Contains(ansi.Strip(m.View().Content), "weekly") {
					t.Fatal("removal must review the selected job without executing")
				}
				m = update(m, key(tea.KeyEscape, ""))
				if runs != 0 || m.screen != configure {
					t.Fatal("cancelled removal ran")
				}
				m.prepareReview()
				m = update(m, key(tea.KeyEnter, ""))
				if runs != 1 {
					t.Fatal("confirmed removal did not run")
				}
			}
		}
	}
}

func TestCronJobPickerHandlesEmptyAndFailedLists(t *testing.T) {
	for _, failure := range []error{nil, errors.New("cannot read crontab")} {
		m := cronChoiceModel(t, "remove", func(context.Context) ([]CronJobChoice, error) { return nil, failure })
		m.run = func(context.Context, Plan) tea.Cmd { t.Fatal("empty list must not run a command"); return nil }
		next, load := m.Update(key(tea.KeyEnter, ""))
		m = update(next.(Model), load())
		if m.picker.loading || m.picker.loadErr == "" {
			t.Fatal("missing empty/error explanation")
		}
		m = update(m, key(tea.KeyEnter, ""))
		if m.picker == nil || m.screen != configure {
			t.Fatal("empty list was accepted")
		}
		m.cronLoader = func(context.Context) ([]CronJobChoice, error) {
			return []CronJobChoice{{Name: "new-job", Schedule: "0 1 * * *", Installed: true}}, nil
		}
		next, load = m.Update(key('l', "", tea.ModCtrl))
		m = update(next.(Model), load())
		if !slices.Equal(m.picker.choices, []string{"new-job"}) || m.picker.loadErr != "" {
			t.Fatal("refresh did not recover")
		}
	}
}

func TestCronJobPickerIgnoresLateReloadsAndCancellation(t *testing.T) {
	m := cronChoiceModel(t, "logs", func(context.Context) ([]CronJobChoice, error) { return []CronJobChoice{{Name: "old-job"}}, nil })
	next, old := m.Update(key(tea.KeyEnter, ""))
	m = next.(Model)
	next, _ = m.Update(key('l', "", tea.ModCtrl))
	m = next.(Model)
	m = update(m, cronJobsLoadedMsg{picker: m.picker, request: m.cronRequest, jobs: []CronJobChoice{{Name: "new-job"}}})
	m = update(m, old())
	if !slices.Equal(m.picker.choices, []string{"new-job"}) {
		t.Fatal("late list overwrote current choices")
	}
	oldPicker, request := m.picker, m.cronRequest
	m = update(m, key(tea.KeyEscape, ""))
	m = update(m, cronJobsLoadedMsg{picker: oldPicker, request: request, jobs: []CronJobChoice{{Name: "late-job"}}})
	if m.picker != nil || m.form.field("arguments").Value != "" {
		t.Fatal("late result changed a cancelled picker")
	}
}

func TestCronJobNamesStayLiteralWhenOpenedFromScheduleBrowser(t *testing.T) {
	m := cronChoiceModel(t, "remove", nil)
	field := m.form.field("arguments")
	field.Value, field.Dirty = `"weekly_backup"`, true
	if !strings.Contains(field.Display(), "weekly_backup") {
		t.Fatal("job name was reformatted in the review")
	}
	plan, err := m.form.Plan(nil)
	if err != nil || plan.Args[len(plan.Args)-1] != "weekly_backup" {
		t.Fatal(plan.Args, err)
	}
}
