package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	managedcron "github.com/mo1ein/TelegramArchive/internal/cron"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type testCronTable struct {
	content string
	writes  int
}

func (t *testCronTable) Load(context.Context) (string, error) { return t.content, nil }
func (t *testCronTable) Save(_ context.Context, value string) error {
	t.content = value
	t.writes++
	return nil
}

func cronRoot(manager managedcron.Manager, run managedcron.Runner) *cobra.Command {
	root := New()
	for _, cmd := range root.Commands() {
		if cmd.Name() == "cron" {
			root.RemoveCommand(cmd)
		}
	}
	root.AddCommand(newCron(manager, run))
	return root
}

func TestCronInstallCapturesSettingsAndRunsSyncOnlyOnce(t *testing.T) {
	for _, account := range []bool{false, true} {
		t.Run(map[bool]string{false: "channel", true: "account too"}[account], func(t *testing.T) {
			viper.Reset()
			defer viper.Reset()
			dir := t.TempDir()
			t.Chdir(dir)
			t.Setenv("TA_NS", "personal")
			t.Setenv("TA_DOWNLOAD_TIMEOUT", "90s")
			t.Setenv("TA_PROXY", "socks5://user:private@127.0.0.1:1080")
			table := &testCronTable{content: "# unrelated\n0 1 * * * /usr/bin/true\n"}
			manager := managedcron.Manager{Directory: filepath.Join(dir, "jobs"), Table: table}
			calls := 0
			root := cronRoot(manager, func(ctx context.Context, job managedcron.Job, out io.Writer) error {
				calls++
				if !slices.Contains(job.Arguments, "--skip-busy=false") {
					t.Fatalf("--run-now should wait for the output folder: %v", job.Arguments)
				}
				return nil
			})
			var output bytes.Buffer
			root.SetOut(&output)
			args := []string{"--storage=type=bolt,path=sessions", "cron", "add", "channel", "--chat=Private group", "--every=6h", "--media=all", "--format=both", "--output=backup", "--run-now"}
			if account {
				args = append(args, "--account")
			}
			root.SetArgs(args)
			if err := root.ExecuteContext(context.Background()); err != nil {
				t.Fatal(err)
			}
			if calls != 1 || table.writes != 1 {
				t.Fatalf("calls=%d writes=%d", calls, table.writes)
			}
			if strings.Contains(table.content, "private") || strings.Contains(output.String(), "private") {
				t.Fatal("credentials were printed or put in crontab")
			}
			job, err := managedcron.ReadJob(manager.File("channel"))
			if err != nil {
				t.Fatal(err)
			}
			if job.Environment["TA_PROXY"] != "socks5://user:private@127.0.0.1:1080" || job.Environment["ALL_PROXY"] != "" {
				t.Fatal("proxy snapshot was not preserved")
			}
			for _, value := range []string{"--ns=personal", "--storage=path=" + filepath.Join(dir, "sessions") + ",type=bolt", "--output=" + filepath.Join(dir, "backup"), "--resume=true", "--chat=Private group", "--skip-busy=true", "--download-timeout=1m30s"} {
				if !slices.Contains(job.Arguments, value) {
					t.Fatalf("missing %s in %v", value, job.Arguments)
				}
			}
			for _, value := range job.Arguments {
				if strings.HasPrefix(value, "--every=") || strings.HasPrefix(value, "--account-include") || strings.HasPrefix(value, "--takeout") || strings.HasPrefix(value, "--run-now") {
					t.Fatalf("changed one-shot argument: %q", value)
				}
			}
			// Replay the saved arguments through the real sync planner, with only
			// the Telegram subprocess replaced. This catches flag-presence changes.
			viper.Reset()
			child := New()
			var phases [][]string
			for _, cmd := range child.Commands() {
				if cmd.Name() == "sync" {
					child.RemoveCommand(cmd)
				}
			}
			child.AddCommand(newSync(func(ctx context.Context, c *cobra.Command, args []string) error {
				phases = append(phases, args)
				return nil
			}))
			child.SetArgs(job.Arguments)
			if err := child.ExecuteContext(context.Background()); err != nil {
				t.Fatal(err)
			}
			want := 1
			if account {
				want = 2
			}
			if len(phases) != want {
				t.Fatalf("saved job backed up the wrong scope: %v", phases)
			}
		})
	}
}

func TestCronManualRunWaitsAndAutomaticRunSkipsBusyFolder(t *testing.T) {
	for _, test := range []struct {
		name, want string
		args       []string
		legacy     bool
	}{
		{name: "manual", args: []string{"run", "channel"}, want: "--skip-busy=false"},
		{name: "automatic", args: []string{"run", "--file"}, want: "--skip-busy=true"},
		{name: "automatic legacy job", args: []string{"run", "--file"}, want: "--skip-busy=true", legacy: true},
		{name: "manual skip", args: []string{"run", "channel", "--skip-busy"}, want: "--skip-busy=true"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			manager := managedcron.Manager{Directory: filepath.Join(dir, "jobs"), Table: &testCronTable{}}
			job := managedcron.Job{Name: "channel", Schedule: "0 */6 * * *", Executable: "/usr/bin/true", Directory: dir, Arguments: []string{"sync", "--skip-busy=true"}, Log: filepath.Join(dir, "channel.log")}
			if test.legacy {
				job.Arguments = []string{"sync"}
			}
			if err := manager.Install(context.Background(), job); err != nil {
				t.Fatal(err)
			}
			called := false
			command := newCron(manager, func(ctx context.Context, job managedcron.Job, output io.Writer) error {
				called = true
				if !slices.Contains(job.Arguments, test.want) {
					t.Fatalf("incorrect lock policy: %v", job.Arguments)
				}
				return nil
			})
			args := slices.Clone(test.args)
			if strings.HasPrefix(test.name, "automatic") {
				args = append(args, manager.File(job.Name))
			}
			command.SetArgs(args)
			if err := command.ExecuteContext(context.Background()); err != nil || !called {
				t.Fatalf("called=%v err=%v", called, err)
			}
			saved, err := managedcron.ReadJob(manager.File(job.Name))
			if err != nil || !slices.Equal(saved.Arguments, job.Arguments) {
				t.Fatalf("manual override changed the installed job: job=%+v err=%v", saved, err)
			}
		})
	}
}

func TestCronRejectsInvalidSetupBeforeWriting(t *testing.T) {
	for _, flags := range [][]string{{"--every=90m"}, {"--every=0"}, {"--every=1s"}, {"--resume=false"}, {"--format=invalid"}, {"--runs=2"}, {"--since=invalid"}, {"--all", "--chat=someone"}} {
		viper.Reset()
		dir := t.TempDir()
		table := &testCronTable{}
		manager := managedcron.Manager{Directory: filepath.Join(dir, "jobs"), Table: table}
		root := cronRoot(manager, func(context.Context, managedcron.Job, io.Writer) error { t.Fatal("invalid job executed"); return nil })
		root.SetArgs(append([]string{"cron", "add", "channel"}, flags...))
		if err := root.ExecuteContext(context.Background()); err == nil || table.writes != 0 {
			t.Fatalf("accepted invalid job %v", flags)
		}
	}
	viper.Reset()
}

func TestCronInitialFailureKeepsInstalledSchedule(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	dir := t.TempDir()
	table := &testCronTable{}
	manager := managedcron.Manager{Directory: filepath.Join(dir, "jobs"), Table: table}
	root := cronRoot(manager, func(context.Context, managedcron.Job, io.Writer) error { return errors.New("network unavailable") })
	root.SetOut(io.Discard)
	root.SetArgs([]string{"cron", "add", "retry", "--chat=test", "--run-now"})
	if err := root.ExecuteContext(context.Background()); err == nil {
		t.Fatal("initial failure was hidden")
	}
	jobs, err := manager.List(context.Background())
	if err != nil || len(jobs) != 1 || !jobs[0].Installed {
		t.Fatal("initial failure removed retries", jobs, err)
	}
}
