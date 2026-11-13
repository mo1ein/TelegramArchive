package cron

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type memoryTable struct {
	content          string
	loadErr, saveErr error
	saves            int
}

func (t *memoryTable) Load(context.Context) (string, error) { return t.content, t.loadErr }
func (t *memoryTable) Save(_ context.Context, value string) error {
	t.saves++
	if t.saveErr != nil {
		return t.saveErr
	}
	t.content = value
	return nil
}

func sampleJob(dir string) Job {
	return Job{Name: "channel", Schedule: "0 */6 * * *", Executable: filepath.Join(dir, "ta"), Directory: dir, Arguments: []string{"sync", "--chat=Private group", "--resume=true"}, Log: filepath.Join(dir, "channel.log"), Environment: map[string]string{"TA_PROXY": "socks5://user:password@localhost:1080"}}
}

func TestSchedule(t *testing.T) {
	for _, tc := range []struct{ duration, want string }{{"1m", "* * * * *"}, {"15m", "*/15 * * * *"}, {"30m", "*/30 * * * *"}, {"1h", "0 * * * *"}, {"6h", "0 */6 * * *"}, {"12h", "0 */12 * * *"}, {"24h", "0 0 * * *"}} {
		d, _ := time.ParseDuration(tc.duration)
		got, err := Schedule(d)
		if err != nil || got != tc.want {
			t.Fatalf("%s: %q %v", tc.duration, got, err)
		}
	}
	for _, value := range []time.Duration{0, -time.Hour, time.Second, 7 * time.Minute, 90 * time.Minute, 25 * time.Hour, 48 * time.Hour} {
		if _, err := Schedule(value); err == nil {
			t.Fatalf("accepted misleading cron interval %s", value)
		}
	}
}

func TestInstallPreservesOtherJobsAndUpdatesWithoutDuplicates(t *testing.T) {
	dir := t.TempDir()
	table := &memoryTable{content: "MAILTO=operator@example.test\n# private user job\n17 3 * * * /usr/bin/true\n"}
	original := table.content
	m := Manager{Directory: filepath.Join(dir, "jobs"), Table: table}
	job := sampleJob(dir)
	if err := m.Install(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(table.content, original) || strings.Contains(table.content, "password") {
		t.Fatal("changed other jobs or exposed proxy credentials")
	}
	job.Schedule = "0 * * * *"
	if err := m.Install(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if strings.Count(table.content, begin(job.Name)) != 1 || strings.Contains(table.content, "*/6") {
		t.Fatal("job was duplicated or not replaced")
	}
	st, _ := os.Stat(m.File(job.Name))
	if st.Mode().Perm() != 0600 {
		t.Fatal("job settings must be private", st.Mode())
	}
	jobs, err := m.List(context.Background())
	if err != nil || len(jobs) != 1 || !jobs[0].Installed {
		t.Fatal(jobs, err)
	}
	if err := m.Remove(context.Background(), job.Name); err != nil {
		t.Fatal(err)
	}
	if table.content != original {
		t.Fatal("removing a job changed unrelated crontab contents")
	}
	if _, err := os.Stat(m.File(job.Name)); !os.IsNotExist(err) {
		t.Fatal("job settings not removed")
	}
}

func TestInstallFailureRollsBackAndReadErrorsNeverOverwrite(t *testing.T) {
	dir := t.TempDir()
	table := &memoryTable{}
	m := Manager{Directory: filepath.Join(dir, "jobs"), Table: table}
	job := sampleJob(dir)
	table.loadErr = errors.New("permission denied")
	if err := m.Install(context.Background(), job); err == nil || table.saves != 0 {
		t.Fatal("overwrote unreadable crontab")
	}
	table.loadErr = nil
	if err := m.Install(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	previous, _ := os.ReadFile(m.File(job.Name))
	oldTable := table.content
	table.saveErr = errors.New("install failed")
	job.Schedule = "0 * * * *"
	if err := m.Install(context.Background(), job); err == nil {
		t.Fatal("swallowed installation failure")
	}
	current, _ := os.ReadFile(m.File(job.Name))
	if string(previous) != string(current) || oldTable != table.content {
		t.Fatal("failed installation changed previous job")
	}
	job.Name = "new"
	if err := m.Install(context.Background(), job); err == nil {
		t.Fatal("swallowed new job failure")
	}
	if _, err := os.Stat(m.File(job.Name)); !os.IsNotExist(err) {
		t.Fatal("failed job left installed settings")
	}
}

func TestStartupJobKeepsScheduleAndCanBeUpdatedAndRemoved(t *testing.T) {
	dir := t.TempDir()
	table := &memoryTable{content: "MAILTO=operator@example.test\n17 3 * * * /usr/bin/true\n"}
	original := table.content
	m := Manager{Directory: filepath.Join(dir, "jobs"), Table: table}
	job := sampleJob(dir)
	// Use the saved representation to also cover loading an enabled startup job.
	if err := json.Unmarshal([]byte(`{"at_startup":true}`), &job); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := m.Install(context.Background(), job); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(table.content, original) || strings.Count(table.content, begin(job.Name)) != 1 {
			t.Fatalf("startup install changed other jobs or duplicated the block: %s", table.content)
		}
		if strings.Count(table.content, job.Schedule+" ") != 1 || strings.Count(table.content, "@reboot ") != 1 {
			t.Fatalf("job must run on its schedule and once at startup: %s", table.content)
		}
		startup := strings.Split(strings.Split(table.content, "@reboot ")[1], "\n")[0]
		if !strings.Contains(startup, "cron run --file "+quote(m.File(job.Name))) || !strings.Contains(startup, "--skip-busy=false") {
			t.Fatalf("startup must run the saved job and wait for shared output folders: %s", startup)
		}
		var err error
		job, err = ReadJob(m.File(job.Name))
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Remove(context.Background(), job.Name); err != nil || table.content != original {
		t.Fatalf("removing startup job changed unrelated entries: %v, %s", err, table.content)
	}
	if err := m.Install(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"at_startup":false}`), &job); err != nil {
		t.Fatal(err)
	}
	if err := m.Install(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(table.content, "@reboot") || strings.Count(table.content, job.Schedule+" ") != 1 {
		t.Fatalf("disabling startup lost the schedule or kept the startup entry: %s", table.content)
	}
}

func TestDamagedStartupBlockIsNeverOverwritten(t *testing.T) {
	for _, block := range []string{
		"# ta cron begin channel\n0 * * * * ta\n@reboot ta\n",
		"# ta cron begin channel\n0 * * * * ta\nunrelated job\n# ta cron end channel\n",
		"# ta cron begin channel\n0 * * * * ta\n@reboot ta\n# ta cron end another\n",
	} {
		if _, err := replace(block, "channel", "replacement"); err == nil {
			t.Fatalf("damaged startup block was overwritten: %s", block)
		}
	}
}

func TestStartupEntryExecutesSavedJobWithQuotedPaths(t *testing.T) {
	dir := t.TempDir()
	job := sampleJob(dir)
	job.AtStartup = true
	job.Executable = filepath.Join(dir, "ta 'quoted'")
	arguments := filepath.Join(dir, "arguments")
	if err := os.WriteFile(job.Executable, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$TA_TEST_ARGUMENTS\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	m := Manager{Directory: filepath.Join(dir, "jobs 'quoted'")}
	entries, err := m.Line(job)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(entries, "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[1], "@reboot ") {
		t.Fatalf("missing startup entry: %s", entries)
	}
	command := exec.Command("/bin/sh", "-c", strings.TrimPrefix(lines[1], "@reboot "))
	command.Env = append(os.Environ(), "TA_TEST_ARGUMENTS="+arguments)
	if err := command.Run(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(arguments)
	want := "cron\nrun\n--file\n" + m.File(job.Name) + "\n--skip-busy=false\n"
	if err != nil || string(got) != want {
		t.Fatalf("startup ran the wrong arguments: %q, %v", got, err)
	}
}

func TestDamagedBlocksAndUnsafePathsAreRejected(t *testing.T) {
	if _, err := replace("# ta cron begin channel\nuser job\n", "channel", "replacement"); err == nil {
		t.Fatal("damaged block was overwritten")
	}
	m := Manager{Directory: t.TempDir()}
	job := sampleJob(t.TempDir())
	for _, name := range []string{"../job", "", "a\n* * * * * bad", "with spaces"} {
		job.Name = name
		if _, err := m.Line(job); err == nil {
			t.Fatalf("accepted job name %q", name)
		}
	}
	job.Name = "valid"
	for _, path := range []string{"relative", "/tmp/ta%data", "/tmp/ta\njob"} {
		job.Executable = path
		if _, err := m.Line(job); err == nil {
			t.Fatalf("accepted cron path %q", path)
		}
	}
}
