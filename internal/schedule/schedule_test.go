package schedule

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestScheduledJobsDoNotOverlapAndRetry(t *testing.T) {
	var calls, active, peak atomic.Int32
	var failures int
	err := Run(context.Background(), Options{Every: time.Millisecond, Runs: 3, OnError: func(error) { failures++ }}, func(ctx context.Context) error {
		n := active.Add(1)
		if n > peak.Load() {
			peak.Store(n)
		}
		defer active.Add(-1)
		call := calls.Add(1)
		if call == 1 {
			return errors.New("temporary failure")
		}
		return nil
	})
	if err != nil || calls.Load() != 3 || peak.Load() != 1 || failures != 1 {
		t.Fatalf("err=%v calls=%d peak=%d failures=%d", err, calls.Load(), peak.Load(), failures)
	}
}

func TestCancellationAndJobTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	err := Run(ctx, Options{Every: time.Hour}, func(ctx context.Context) error { cancel(); return ctx.Err() })
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	var timeout bool
	err = Run(context.Background(), Options{Every: time.Millisecond, Runs: 1, Timeout: time.Millisecond, OnError: func(err error) { timeout = errors.Is(err, context.DeadlineExceeded) }}, func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() })
	if !errors.Is(err, context.DeadlineExceeded) || !timeout {
		t.Fatalf("%v timeout=%v", err, timeout)
	}
}

func TestValidationAndStopOnError(t *testing.T) {
	if err := Run(context.Background(), Options{}, func(context.Context) error { return nil }); err == nil {
		t.Fatal("invalid schedule accepted")
	}
	want := errors.New("fail")
	if err := Run(context.Background(), Options{Every: time.Millisecond, StopOnError: true}, func(context.Context) error { return want }); !errors.Is(err, want) {
		t.Fatal(err)
	}
}
