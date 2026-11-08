package telegram

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gotd/td/bin"
	tgclient "github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

const DefaultDownloadTimeout = 2 * time.Minute

// DownloadStalledError lets the archive retry with a fresh Telegram session.
type DownloadStalledError struct {
	Path        string
	IdleTimeout time.Duration
}

func (e *DownloadStalledError) Error() string {
	return fmt.Sprintf("Telegram request stalled for %s without data or a server response: %s", e.IdleTimeout, e.Path)
}

func (*DownloadStalledError) Timeout() bool   { return true }
func (*DownloadStalledError) Temporary() bool { return true }

type downloadActivityKey struct{}

type downloadActivity struct {
	mu        sync.Mutex
	ctx       context.Context
	cancel    context.CancelCauseFunc
	timer     *time.Timer
	timeout   time.Duration
	deadline  time.Time
	waitUntil time.Time
	stopped   bool
	failure   *DownloadStalledError
}

func watchDownload(parent context.Context, path string, timeout time.Duration) (context.Context, *downloadActivity) {
	if timeout <= 0 {
		return parent, nil
	}
	ctx, cancel := context.WithCancelCause(parent)
	a := &downloadActivity{ctx: ctx, cancel: cancel, timeout: timeout, deadline: time.Now().Add(timeout), failure: &DownloadStalledError{Path: path, IdleTimeout: timeout}}
	// The callback must not see an uninitialized timer, even with tiny timeouts.
	a.mu.Lock()
	a.timer = time.AfterFunc(timeout, a.expire)
	a.mu.Unlock()
	return context.WithValue(ctx, downloadActivityKey{}, a), a
}

func (a *downloadActivity) expire() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stopped {
		return
	}
	if remaining := time.Until(a.deadline); remaining > 0 {
		a.timer.Reset(remaining)
		return
	}
	a.cancel(a.failure)
}

func (a *downloadActivity) touch() { a.wait(0) }

func (a *downloadActivity) wait(duration time.Duration) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stopped {
		return
	}
	now := time.Now()
	if until := now.Add(duration); until.After(a.waitUntil) {
		a.waitUntil = until
	}
	// Bytes from one worker must not shorten a flood wait in another worker.
	a.deadline = a.waitUntil.Add(a.timeout)
	a.timer.Reset(time.Until(a.deadline))
}

func (a *downloadActivity) finish(err error) error {
	if a == nil {
		return err
	}
	a.mu.Lock()
	a.stopped = true
	a.timer.Stop()
	stalled := context.Cause(a.ctx) == a.failure
	a.mu.Unlock()
	a.cancel(nil)
	if stalled {
		return errors.Join(a.failure, err)
	}
	return err
}

// DownloadActivityMiddleware observes replies inside the automatic flood-wait
// handler, so a requested Telegram wait is not mistaken for a stalled transfer.
// Requests outside a watched history read or download are unaffected.
func DownloadActivityMiddleware() tgclient.Middleware {
	return tgclient.MiddlewareFunc(func(next tg.Invoker) tgclient.InvokeFunc {
		return func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
			err := next.Invoke(ctx, input, output)
			if activity, _ := ctx.Value(downloadActivityKey{}).(*downloadActivity); activity != nil {
				if wait, ok := tgerr.AsFloodWait(err); ok {
					// The downloader's own flood handler adds one second of slack.
					activity.wait(max(wait, time.Second) + time.Second)
				} else if err == nil {
					activity.touch()
				}
			}
			return err
		}
	})
}
