package cmd

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	managedcron "github.com/mo1ein/TelegramArchive/internal/cron"
)

func TestTUIJobLoaderUsesCronListWithoutChangingJobs(t *testing.T) {
	dir := t.TempDir()
	table := &testCronTable{}
	manager := managedcron.Manager{Directory: filepath.Join(dir, "jobs"), Table: table}
	job := managedcron.Job{Name: "daily", Schedule: "0 1 * * *", AtStartup: true, Executable: "/bin/true", Directory: dir, Arguments: []string{"sync"}, Log: filepath.Join(dir, "daily.log")}
	if err := manager.Install(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	before, writes := table.content, table.writes
	jobs, err := tuiCronJobs(manager)(context.Background())
	if err != nil || len(jobs) != 1 || jobs[0].Name != "daily" || !jobs[0].Installed || !strings.Contains(jobs[0].Schedule, "startup") {
		t.Fatal(jobs, err)
	}
	if table.content != before || table.writes != writes {
		t.Fatal("listing jobs modified the crontab")
	}
	table.content = ""
	jobs, err = tuiCronJobs(manager)(context.Background())
	if err != nil || len(jobs) != 1 || jobs[0].Installed {
		t.Fatal("saved job missing from crontab should remain selectable", jobs, err)
	}
}
