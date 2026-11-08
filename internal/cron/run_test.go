package cron

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSkipsOverlapAndRetainsPrivateLog(t *testing.T) {
	dir := t.TempDir()
	table := &memoryTable{}
	manager := Manager{Directory: filepath.Join(dir, "jobs"), Table: table}
	job := sampleJob(dir)
	if err := manager.Install(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	started, finish := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), manager.File(job.Name), io.Discard, func(ctx context.Context, job Job, out io.Writer) error {
			close(started)
			<-finish
			io.WriteString(out, "downloaded new messages\n")
			return nil
		})
	}()
	<-started
	var skipped bytes.Buffer
	err := Run(context.Background(), manager.File(job.Name), &skipped, func(context.Context, Job, io.Writer) error { t.Error("overlapping backup executed"); return nil })
	close(finish)
	if first := <-done; first != nil {
		t.Fatal(first)
	}
	if err != nil || !strings.Contains(skipped.String(), "already running") {
		t.Fatal("overlap not skipped", err, skipped.String())
	}
	data, _ := os.ReadFile(job.Log)
	if !strings.Contains(string(data), "downloaded new messages") || !strings.Contains(string(data), "Backup complete") {
		t.Fatal("missing job log")
	}
	info, _ := os.Stat(job.Log)
	if info.Mode().Perm() != 0600 {
		t.Fatal("log is not private")
	}
}

func TestExecuteRestoresDirectoryAndEnvironmentWithoutShellExpansion(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "ta with spaces")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\npwd\nprintf '%s\\n' \"$TA_PROXY\" \"$HOME\" \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "injected")
	job := sampleJob(dir)
	job.Executable = binary
	job.Environment["HOME"] = filepath.Join(dir, "home")
	job.Arguments = []string{"sync", "--chat=$(touch " + marker + ")", "--output=" + filepath.Join(dir, "data space")}
	var out bytes.Buffer
	if err := Execute(context.Background(), job, &out); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{dir, job.Environment["TA_PROXY"], job.Environment["HOME"], job.Arguments[1], job.Arguments[2]} {
		if !strings.Contains(out.String(), value) {
			t.Fatal("saved execution settings were lost")
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("argument was interpreted by a shell")
	}
}

func TestCronEntryQuotesExecutableAndConfigurationPaths(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "ta 'quoted' $(touch injected)")
	output := filepath.Join(dir, "arguments")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$TA_TEST_ARGUMENTS\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	manager := Manager{Directory: filepath.Join(dir, "jobs 'with quotes'")}
	job := sampleJob(dir)
	job.Executable = binary
	line, err := manager.Line(job)
	if err != nil {
		t.Fatal(err)
	}
	shell := exec.Command("/bin/sh", "-c", strings.TrimPrefix(line, job.Schedule+" "))
	shell.Dir = dir
	shell.Env = append(os.Environ(), "TA_TEST_ARGUMENTS="+output)
	if err := shell.Run(); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(args) != "cron\nrun\n--file\n"+manager.File(job.Name)+"\n" {
		t.Fatalf("incorrect cron arguments: %s", args)
	}
	if _, err := os.Stat(filepath.Join(dir, "injected")); !os.IsNotExist(err) {
		t.Fatal("executable path was expanded by the shell")
	}
}

func TestCommandTableDistinguishesMissingFromDenied(t *testing.T) {
	dir := t.TempDir()
	program := filepath.Join(dir, "crontab")
	state := filepath.Join(dir, "table")
	script := `#!/bin/sh
if [ "$1" = "-l" ]; then
 case "$TA_TEST_CRON_MODE" in
  none) echo 'no crontab for test' >&2; exit 1;;
  denied) echo 'permission denied' >&2; exit 1;;
 esac
 /bin/cat "$TA_TEST_CRON_TABLE"
else
 /bin/cat > "$TA_TEST_CRON_TABLE"
fi
`
	if err := os.WriteFile(program, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TA_TEST_CRON_TABLE", state)
	provider := CommandTable{}
	t.Setenv("TA_TEST_CRON_MODE", "none")
	if value, err := provider.Load(context.Background()); err != nil || value != "" {
		t.Fatal(value, err)
	}
	t.Setenv("TA_TEST_CRON_MODE", "denied")
	if _, err := provider.Load(context.Background()); err == nil {
		t.Fatal("permission failure treated as an empty crontab")
	}
	t.Setenv("TA_TEST_CRON_MODE", "")
	if err := provider.Save(context.Background(), "# preserved\n0 * * * * /usr/bin/true\n"); err != nil {
		t.Fatal(err)
	}
	if value, err := provider.Load(context.Background()); err != nil || !strings.Contains(value, "# preserved") {
		t.Fatal(value, err)
	}
}
