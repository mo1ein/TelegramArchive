// Package schedule runs one backup job at a time on a fixed interval.
package schedule

import (
	"context"
	"fmt"
	"time"
)

type Options struct {
	Every       time.Duration
	Runs        int
	Timeout     time.Duration
	StopOnError bool
	OnError     func(error)
	OnStart     func(int, time.Time)
	OnNext      func(time.Time)
}

// Run starts immediately, skips missed slots, and never queues overlapping jobs.
// Restarting the process immediately retries using the archive's checkpoints.
func Run(ctx context.Context, o Options, job func(context.Context) error) error {
	if o.Every <= 0 || o.Runs < 0 || o.Timeout < 0 {
		return fmt.Errorf("schedule requires a positive interval and nonnegative runs/timeout")
	}
	next := time.Now()
	for run := 1; o.Runs == 0 || run <= o.Runs; run++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if o.OnStart != nil {
			o.OnStart(run, time.Now())
		}
		jobCtx := ctx
		cancel := func() {}
		if o.Timeout > 0 {
			jobCtx, cancel = context.WithTimeout(ctx, o.Timeout)
		}
		err := job(jobCtx)
		if jobCtx.Err() != nil {
			err = jobCtx.Err()
		}
		cancel()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			if o.OnError != nil {
				o.OnError(err)
			}
			if o.StopOnError {
				return err
			}
		}
		if o.Runs > 0 && run == o.Runs {
			return err
		}
		next = next.Add(o.Every)
		if now := time.Now(); !next.After(now) {
			next = next.Add((now.Sub(next)/o.Every + 1) * o.Every)
		}
		if o.OnNext != nil {
			o.OnNext(next)
		}
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}
