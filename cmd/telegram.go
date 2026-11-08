package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tgerr"
	"github.com/iyear/tdl/core/storage"
	tclientcore "github.com/iyear/tdl/core/tclient"
	"github.com/iyear/tdl/pkg/consts"
	"github.com/iyear/tdl/pkg/kv"
	"github.com/iyear/tdl/pkg/tclient"
	transport "github.com/mo1ein/TelegramArchive/internal/telegram"
	"github.com/spf13/viper"
)

const maxArchiveReconnects = 5

const flagDownloadTimeout = "download-timeout"

// A dead gotd client cannot be reused. Recreate the client and transport pools;
// the archive callback resumes its on-disk checkpoints with fresh file references.
func tRunArchive(ctx context.Context, resume bool, output io.Writer, f func(context.Context, *telegram.Client, storage.Storage) error) error {
	policy := backoff.NewExponentialBackOff()
	policy.InitialInterval = time.Second
	policy.Multiplier = 2
	policy.MaxInterval = 10 * time.Second
	// Limit attempts instead of wall time: the first session can run for hours.
	policy.MaxElapsedTime = 0
	retries := uint64(0)
	if resume {
		retries = maxArchiveReconnects
	}
	attempt := 0
	return retryTelegram(ctx, backoff.WithMaxRetries(policy, retries), func() error {
		return tRun(ctx, f)
	}, func(err error, delay time.Duration) {
		attempt++
		fmt.Fprintf(output, "Telegram connection interrupted: %v\nReconnecting in %s (%d/%d); resuming existing checkpoints and completed media.\n", err, delay.Round(time.Millisecond), attempt, retries)
	})
}

func retryTelegram(ctx context.Context, policy backoff.BackOff, run func() error, notify backoff.Notify) error {
	return backoff.RetryNotify(func() error {
		if err := ctx.Err(); err != nil {
			return backoff.Permanent(err)
		}
		err := run()
		if canceled := ctx.Err(); canceled != nil {
			return backoff.Permanent(canceled)
		}
		if err != nil && !transientTelegramError(err) {
			return backoff.Permanent(err)
		}
		return err
	}, backoff.WithContext(policy, ctx), notify)
}

func transientTelegramError(err error) bool {
	var rpc *tgerr.Error
	var file *os.PathError
	if errors.As(err, &rpc) || errors.As(err, &file) {
		return false
	}
	// retryTelegram checks the parent context first. Cancellation here came
	// from a child/session context, so recreating the client can recover it.
	var network net.Error
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, io.ErrClosedPipe) || errors.Is(err, net.ErrClosed) ||
		errors.Is(err, context.Canceled) ||
		errors.As(err, &network)
}

// gotd Run can suppress a callback error wrapping context.Canceled. Preserve it
// so a dead child context cannot make an interrupted archive look successful.
func runTelegramSession(ctx context.Context, run func(context.Context, func(context.Context) error) error, callback func(context.Context) error) error {
	var callbackErr error
	called := false
	runErr := run(ctx, func(child context.Context) error {
		called = true
		callbackErr = callback(child)
		return callbackErr
	})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if runErr == nil {
		if !called {
			return fmt.Errorf("Telegram session ended before the operation started: %w", context.Canceled)
		}
		return callbackErr
	}
	if callbackErr == nil || errors.Is(runErr, callbackErr) {
		return runErr
	}
	return errors.Join(runErr, callbackErr)
}

func tRun(ctx context.Context, f func(context.Context, *telegram.Client, storage.Storage) error) error {
	kvd, err := kv.From(ctx).Open(viper.GetString(consts.FlagNamespace))
	if err != nil {
		return fmt.Errorf("open session: %w", err)
	}
	proxyURL := telegramProxy()
	c, err := tclient.New(ctx, tclient.Options{KV: kvd, Proxy: proxyURL, NTP: viper.GetString(consts.FlagNTP), ReconnectTimeout: viper.GetDuration(consts.FlagReconnectTimeout)}, false, transport.DownloadActivityMiddleware())
	if err != nil {
		return err
	}
	err = runTelegramSession(ctx, func(ctx context.Context, callback func(context.Context) error) error {
		return tclientcore.RunWithAuth(ctx, c, callback)
	}, func(ctx context.Context) error { return f(ctx, c, kvd) })
	if err != nil && strings.Contains(err.Error(), "dial failed") {
		if proxyURL == "" {
			return fmt.Errorf("%w; Telegram direct connection failed, set --proxy or TA_PROXY/ALL_PROXY/HTTPS_PROXY/HTTP_PROXY to a reachable SOCKS5/HTTP proxy", err)
		}
		return fmt.Errorf("%w; check that the configured proxy can reach Telegram", err)
	}
	return err
}

func telegramProxy() string {
	if proxyURL := strings.TrimSpace(viper.GetString(consts.FlagProxy)); proxyURL != "" {
		return proxyURL
	}
	for _, key := range []string{"ALL_PROXY", "all_proxy", "HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		if proxyURL := strings.TrimSpace(os.Getenv(key)); proxyURL != "" {
			return proxyURL
		}
	}
	return ""
}
