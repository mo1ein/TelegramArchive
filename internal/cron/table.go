package cron

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type CommandTable struct{}

func tableCommand(ctx context.Context, args ...string) (*exec.Cmd, error) {
	path, err := exec.LookPath("crontab")
	if err != nil {
		return nil, fmt.Errorf("crontab is not installed; install and enable your system's cron service first")
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "NO_COLOR=1")
	return cmd, nil
}

func (CommandTable) Load(ctx context.Context) (string, error) {
	cmd, err := tableCommand(ctx, "-l")
	if err != nil {
		return "", err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 && strings.HasPrefix(strings.TrimSpace(stderr.String()), "no crontab for ") {
			return "", nil
		}
		return "", fmt.Errorf("read crontab: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

func (CommandTable) Save(ctx context.Context, table string) error {
	cmd, err := tableCommand(ctx, "-")
	if err != nil {
		return err
	}
	cmd.Stdin = strings.NewReader(table)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("install crontab: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
