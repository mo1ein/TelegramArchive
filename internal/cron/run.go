package cron

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	bolt "go.etcd.io/bbolt"
)

type Runner func(context.Context, Job, io.Writer) error

func Execute(ctx context.Context, job Job, output io.Writer) error {
	child := exec.CommandContext(ctx, job.Executable, job.Arguments...)
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
		fmt.Fprintf(out, "[%s] %s is already running; skipping this tick.\n", time.Now().Format(time.RFC3339), job.Name)
		return nil
	}
	if err != nil {
		return err
	}
	defer lock.Close()
	fmt.Fprintf(out, "[%s] Starting cron backup %s\n", time.Now().Format(time.RFC3339), job.Name)
	err = execute(ctx, job, out)
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil {
		fmt.Fprintf(out, "[%s] Backup failed: %v\n", time.Now().Format(time.RFC3339), err)
		return fmt.Errorf("cron backup %s failed: %w; see %s", job.Name, err, job.Log)
	}
	fmt.Fprintf(out, "[%s] Backup complete; future runs reuse these checkpoints.\n", time.Now().Format(time.RFC3339))
	return nil
}
