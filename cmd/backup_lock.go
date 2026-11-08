package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

func lockBackupOutput(ctx context.Context, dir string, skipBusy bool, output io.Writer) (*bolt.DB, error) {
	notified := false
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		lock, err := bolt.Open(filepath.Join(dir, ".backup.lock"), 0600, &bolt.Options{Timeout: 100 * time.Millisecond})
		if !errors.Is(err, bolt.ErrTimeout) {
			if err != nil {
				return nil, fmt.Errorf("open backup lock in %s: %w", dir, err)
			}
			return lock, nil
		}
		if skipBusy {
			fmt.Fprintf(output, "Another backup is using %s; skipping this cycle.\n", dir)
			return nil, nil
		}
		if !notified {
			fmt.Fprintf(output, "Waiting for the current backup in %s to finish; Ctrl+C cancels waiting.\n", dir)
			notified = true
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
