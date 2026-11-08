package telegram

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/td/bin"
	tgclient "github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	transferdownload "github.com/iyear/tdl/core/downloader"
	"github.com/iyear/tdl/core/tmedia"
	"github.com/mo1ein/TelegramArchive/internal/archive"
)

type watchdogTestPool struct {
	client func(context.Context, int) *tg.Client
}

func (p watchdogTestPool) Client(ctx context.Context, dc int) *tg.Client { return p.client(ctx, dc) }
func (p watchdogTestPool) Default(ctx context.Context) *tg.Client        { return p.Client(ctx, 4) }
func (p watchdogTestPool) Takeout(ctx context.Context, dc int) *tg.Client {
	return p.Client(ctx, dc)
}
func (watchdogTestPool) Close() error { return nil }

func watchdogTestFile(t *testing.T, size int64) (archive.File, string) {
	t.Helper()
	file := archive.File{Path: "files/stalled.bin", Size: size, Remote: &tmedia.Media{
		DC: 4, Size: size, InputFileLoc: &tg.InputDocumentFileLocation{ID: 123},
	}}
	dest := filepath.Join(t.TempDir(), "partial")
	if err := os.WriteFile(dest, nil, 0600); err != nil {
		t.Fatal(err)
	}
	return file, dest
}

func TestDownloadWatchdogStopsStalledSetupAndFileRequests(t *testing.T) {
	for _, stallSetup := range []bool{false, true} {
		t.Run(map[bool]string{false: "file request", true: "data center setup"}[stallSetup], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			api := tg.NewClient(DownloadActivityMiddleware().Handle(tgclient.InvokeFunc(func(ctx context.Context, _ bin.Encoder, _ bin.Decoder) error {
				<-ctx.Done()
				return ctx.Err()
			})))
			source := &Source{threads: 1, DownloadTimeout: 60 * time.Millisecond, pool: watchdogTestPool{
				client: func(ctx context.Context, dc int) *tg.Client {
					if stallSetup {
						<-ctx.Done()
					}
					return api
				},
			}}
			file, dest := watchdogTestFile(t, 4)
			err := source.DownloadWithProgress(ctx, file, dest, nil)
			var stalled *DownloadStalledError
			var network net.Error
			if !errors.As(err, &stalled) || !errors.As(err, &network) || !network.Timeout() {
				t.Fatalf("stalled transfer did not produce a retryable timeout: %v", err)
			}
			if stalled.Path != file.Path || stalled.IdleTimeout != source.DownloadTimeout {
				t.Fatalf("timeout lost its filename or configured duration: %+v", stalled)
			}
			if ctx.Err() != nil {
				t.Fatal("watchdog waited for the whole operation's deadline")
			}
		})
	}
}

func TestDownloadWatchdogResetsForHealthyChunks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	body := bytes.Repeat([]byte{'x'}, transferdownload.MaxPartSize*3+1)
	api := tg.NewClient(DownloadActivityMiddleware().Handle(tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		request := input.(*tg.UploadGetFileRequest)
		timer := time.NewTimer(100 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
		start := min(int(request.Offset), len(body))
		end := min(start+request.Limit, len(body))
		output.(*tg.UploadFileBox).File = &tg.UploadFile{Type: &tg.StorageFileUnknown{}, Bytes: body[start:end]}
		return nil
	})))
	source := &Source{threads: 1, DownloadTimeout: 250 * time.Millisecond, pool: watchdogTestPool{client: func(context.Context, int) *tg.Client { return api }}}
	file, dest := watchdogTestFile(t, int64(len(body)))
	var transferred atomic.Int64
	if err := source.DownloadWithProgress(ctx, file, dest, func(n int64) { transferred.Add(n) }); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest)
	if err != nil || !bytes.Equal(data, body) || transferred.Load() != int64(len(body)) {
		t.Fatalf("healthy transfer was truncated: len=%d reported=%d err=%v", len(data), transferred.Load(), err)
	}
}

func TestDownloadWatchdogHonorsTelegramFloodWait(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	var calls atomic.Int32
	rpc := tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		if calls.Add(1) == 1 {
			return fmt.Errorf("media request: %w", tgerr.New(420, "FLOOD_WAIT_1"))
		}
		output.(*tg.UploadFileBox).File = &tg.UploadFile{Type: &tg.StorageFileUnknown{}, Bytes: []byte("data")}
		return nil
	})
	// The observer must see FLOOD_WAIT before the waiter consumes it. A
	// legitimate one-second server delay exceeds this transfer's idle budget.
	api := tg.NewClient(floodwait.NewSimpleWaiter().Handle(DownloadActivityMiddleware().Handle(rpc)))
	source := &Source{threads: 1, DownloadTimeout: 150 * time.Millisecond, pool: watchdogTestPool{client: func(context.Context, int) *tg.Client { return api }}}
	file, dest := watchdogTestFile(t, 4)
	if err := source.DownloadWithProgress(ctx, file, dest, nil); err != nil {
		t.Fatalf("Telegram flood wait was mistaken for a stalled connection: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "data" || calls.Load() != 2 {
		t.Fatalf("flood-wait retry did not finish: data=%q calls=%d err=%v", data, calls.Load(), err)
	}
}

func TestDownloadWatchdogKeepsAnotherWorkersFloodWaitWhenBytesArrive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	body := bytes.Repeat([]byte{'x'}, transferdownload.MaxPartSize+4)
	floodIssued := make(chan struct{})
	var flooded atomic.Bool
	rpc := tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		request := input.(*tg.UploadGetFileRequest)
		if request.Offset == 0 && flooded.CompareAndSwap(false, true) {
			close(floodIssued)
			return tgerr.New(420, "FLOOD_WAIT_1")
		}
		if request.Offset > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-floodIssued:
			}
			// Let the first worker record its flood wait, then deliver bytes in
			// the second worker while that server-requested wait is still active.
			timer := time.NewTimer(100 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
			}
		}
		start := min(int(request.Offset), len(body))
		end := min(start+request.Limit, len(body))
		output.(*tg.UploadFileBox).File = &tg.UploadFile{Type: &tg.StorageFileUnknown{}, Bytes: body[start:end]}
		return nil
	})
	api := tg.NewClient(floodwait.NewSimpleWaiter().Handle(DownloadActivityMiddleware().Handle(rpc)))
	source := &Source{threads: 2, DownloadTimeout: 400 * time.Millisecond, pool: watchdogTestPool{client: func(context.Context, int) *tg.Client { return api }}}
	file, dest := watchdogTestFile(t, int64(len(body)))
	if err := source.DownloadWithProgress(ctx, file, dest, nil); err != nil {
		t.Fatalf("a concurrent worker shortened the pending flood wait: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil || !bytes.Equal(data, body) {
		t.Fatalf("parallel transfer after flood wait was truncated: len=%d err=%v", len(data), err)
	}
}

func TestDownloadWatchdogPreservesParentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	api := tg.NewClient(tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		cancel()
		<-ctx.Done()
		return ctx.Err()
	}))
	source := &Source{threads: 1, DownloadTimeout: time.Second, pool: watchdogTestPool{client: func(context.Context, int) *tg.Client { return api }}}
	file, dest := watchdogTestFile(t, 4)
	err := source.DownloadWithProgress(ctx, file, dest, nil)
	var stalled *DownloadStalledError
	if !errors.Is(err, context.Canceled) || errors.As(err, &stalled) {
		t.Fatalf("parent cancellation became a transfer timeout: %v", err)
	}
}

func TestDownloadWatchdogCanBeDisabled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	api := tg.NewClient(tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		<-ctx.Done()
		return ctx.Err()
	}))
	source := &Source{threads: 1, DownloadTimeout: 0, pool: watchdogTestPool{client: func(context.Context, int) *tg.Client { return api }}}
	file, dest := watchdogTestFile(t, 4)
	err := source.DownloadWithProgress(ctx, file, dest, nil)
	var stalled *DownloadStalledError
	if !errors.Is(err, context.DeadlineExceeded) || errors.As(err, &stalled) {
		t.Fatalf("disabled watchdog ignored the caller's deadline: %v", err)
	}
}
