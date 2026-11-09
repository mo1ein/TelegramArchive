package cron

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mo1ein/TelegramArchive/internal/archive"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"syscall"
	"time"

	bolt "go.etcd.io/bbolt"
)

type Runner func(context.Context, Job, io.Writer) error

func Execute(ctx context.Context, job Job, output io.Writer) error {
	args := slices.Clone(job.Arguments)
	if job.ResultFile != "" {
		args = append(args, "--result-file="+job.ResultFile)
	}
	child := exec.CommandContext(ctx, job.Executable, args...)
	child.Dir = job.Directory
	child.Env = os.Environ()
	for key, value := range job.Environment {
		child.Env = append(child.Env, key+"="+value)
	}
	child.Stdout, child.Stderr = output, output
	child.WaitDelay = 30 * time.Second
	child.Cancel = func() error {
		if err := child.Process.Signal(syscall.SIGTERM); err != nil {
			return child.Process.Kill()
		}
		return nil
	}
	return child.Run()
}

// Run holds the job lock until all backup children exit. Cron can trigger while
// a long first archive is still running; that tick is skipped, never queued.
func Run(ctx context.Context, file string, output io.Writer, execute Runner) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	job, err := ReadJob(file)
	if err != nil {
		return err
	}
	log, err := os.OpenFile(job.Log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer log.Close()
	if err = log.Chmod(0600); err != nil {
		return err
	}
	if output == nil {
		output = io.Discard
	}
	out := io.MultiWriter(log, output)
	lock, err := bolt.Open(file+".run.lock", 0600, &bolt.Options{Timeout: 50 * time.Millisecond})
	if errors.Is(err, bolt.ErrTimeout) {
		fmt.Fprintf(out, "[%s] Skipped job %q | already running; no new backup started.\n", time.Now().Format(time.RFC3339), job.Name)
		return nil
	}
	if err != nil {
		return err
	}
	defer lock.Close()
	result, err := os.CreateTemp(filepath.Dir(file), ".job-result-*")
	if err != nil {
		return err
	}
	job.ResultFile = result.Name()
	result.Close()
	defer os.Remove(job.ResultFile)
	job.ResultFile, err = filepath.Abs(job.ResultFile)
	if err != nil {
		return err
	}
	started := time.Now().UTC()
	fmt.Fprintf(out, "[%s] Starting cron backup %s\n", time.Now().Format(time.RFC3339), job.Name)
	err = execute(ctx, job, out)
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	var report archive.CompletionReport
	raw, readErr := os.ReadFile(job.ResultFile)
	if readErr == nil {
		readErr = json.Unmarshal(raw, &report)
	}
	validStatus := slices.Contains([]string{"complete", "complete_with_warnings", "failed", "canceled", "skipped"}, report.Status)
	if readErr != nil || report.Kind != "backup" || !validStatus {
		report = archive.CompletionReport{Status: "unknown", Summary: archive.Summary{Error: "child did not provide a valid completion report"}}
	}
	if err == nil && (report.Status == "failed" || report.Status == "canceled") {
		err = fmt.Errorf("backup reported %s: %s", report.Status, report.Error)
	}
	if err != nil {
		report.Status = "failed"
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		report.Status = "canceled"
	}
	finished := time.Now().UTC()
	fmt.Fprint(out, archive.SummaryText(fmt.Sprintf("Finished job %q", job.Name), report.Status, report.Summary, finished, finished.Sub(started)))
	if report.Status == "skipped" && report.Error != "" {
		fmt.Fprintln(out, report.Error)
	}
	if err != nil {
		return fmt.Errorf("cron backup %s failed: %w; see %s", job.Name, err, job.Log)
	}
	return nil
}
