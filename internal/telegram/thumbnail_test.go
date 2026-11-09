package telegram

import (
	"bytes"
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gotd/td/bin"
	tgclient "github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	transferdownload "github.com/iyear/tdl/core/downloader"
	"github.com/mo1ein/TelegramArchive/internal/archive"
)

func thumbnailTestSource(invoke tgclient.InvokeFunc) *Source {
	// Real Telegram timeouts are not an instantaneous busy loop. Pace the fake
	// so the old downloader's unlimited retry path reaches a bounded watchdog.
	paced := tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		timer := time.NewTimer(10 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return invoke(ctx, input, output)
		}
	})
	api := tg.NewClient(DownloadActivityMiddleware().Handle(paced))
	return &Source{threads: 1, DownloadTimeout: 150 * time.Millisecond, pool: watchdogTestPool{client: func(context.Context, int) *tg.Client { return api }}}
}

func TestThumbnailRepeatedServerTimeoutBecomesUnavailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var calls atomic.Int32
	source := thumbnailTestSource(tgclient.InvokeFunc(func(context.Context, bin.Encoder, bin.Decoder) error {
		calls.Add(1)
		return tgerr.New(-503, "Timeout")
	}))
	file, dest := watchdogTestFile(t, 4)
	file.Field, file.Kind, file.Path = "thumbnail", "videos", "video_files/700_thumb.jpg"
	err := source.DownloadWithProgress(ctx, file, dest, nil)
	if !errors.Is(err, archive.ErrThumbnailUnavailable) {
		t.Fatalf("thumbnail still stalled the transfer instead of being unavailable: calls=%d err=%v", calls.Load(), err)
	}
	if calls.Load() > 3 || calls.Load() == 0 || ctx.Err() != nil {
		t.Fatalf("thumbnail retries were not bounded: calls=%d parent=%v", calls.Load(), ctx.Err())
	}
	if tgerr.Is(err, tg.ErrTimeout) {
		t.Fatal("unavailable thumbnail still wraps the raw server Timeout, causing the downloader to retry forever")
	}
}

func TestThumbnailTransientTimeoutsRecoverAndSuccessResetsRetryBudget(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	body := bytes.Repeat([]byte{'x'}, transferdownload.MaxPartSize+4)
	attempts := map[int64]int{}
	source := thumbnailTestSource(tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		request := input.(*tg.UploadGetFileRequest)
		attempts[request.Offset]++
		if attempts[request.Offset] <= 2 {
			return tgerr.New(-503, "Timeout")
		}
		start := min(int(request.Offset), len(body))
		end := min(start+request.Limit, len(body))
		output.(*tg.UploadFileBox).File = &tg.UploadFile{Type: &tg.StorageFileUnknown{}, Bytes: body[start:end]}
		return nil
	}))
	file, dest := watchdogTestFile(t, int64(len(body)))
	file.Field, file.Kind, file.Path = "thumbnail", "videos", "video_files/recovered_thumb.jpg"
	if err := source.DownloadWithProgress(ctx, file, dest, nil); err != nil {
		t.Fatalf("a recoverable thumbnail was skipped: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil || !bytes.Equal(data, body) {
		t.Fatalf("recovered thumbnail was truncated: size=%d err=%v", len(data), err)
	}
	if attempts[0] != 3 || attempts[transferdownload.MaxPartSize] != 3 {
		t.Fatalf("expected transient timeouts on both chunks: %v", attempts)
	}
}

func TestPrimaryMediaTimeoutDoesNotBecomeUnavailableThumbnail(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var calls atomic.Int32
	source := thumbnailTestSource(tgclient.InvokeFunc(func(context.Context, bin.Encoder, bin.Decoder) error {
		calls.Add(1)
		return tgerr.New(-503, "Timeout")
	}))
	file, dest := watchdogTestFile(t, 4)
	file.Field, file.Kind, file.Path = "file", "videos", "video_files/700.mp4"
	err := source.DownloadWithProgress(ctx, file, dest, nil)
	var stalled *DownloadStalledError
	if !errors.As(err, &stalled) || errors.Is(err, archive.ErrThumbnailUnavailable) || calls.Load() <= 3 {
		t.Fatalf("primary media was skipped: calls=%d err=%v", calls.Load(), err)
	}
}

func TestThumbnailCancellationIsNotUnavailable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	source := thumbnailTestSource(tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		if calls.Add(1) == 2 {
			cancel()
			return ctx.Err()
		}
		return tgerr.New(-503, "Timeout")
	}))
	file, dest := watchdogTestFile(t, 4)
	file.Field, file.Kind = "thumbnail", "videos"
	err := source.DownloadWithProgress(ctx, file, dest, nil)
	if !errors.Is(err, context.Canceled) || errors.Is(err, archive.ErrThumbnailUnavailable) || calls.Load() != 2 {
		t.Fatalf("cancellation was treated as a missing thumbnail: calls=%d err=%v", calls.Load(), err)
	}
}

func TestThumbnailLocalWriteFailureIsNotUnavailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	source := thumbnailTestSource(tgclient.InvokeFunc(func(context.Context, bin.Encoder, bin.Decoder) error {
		t.Error("network request started despite an unusable destination")
		return tgerr.New(-503, "Timeout")
	}))
	file, _ := watchdogTestFile(t, 4)
	file.Field, file.Kind = "thumbnail", "videos"
	err := source.DownloadWithProgress(ctx, file, t.TempDir(), nil)
	var local *os.PathError
	if !errors.As(err, &local) || errors.Is(err, archive.ErrThumbnailUnavailable) {
		t.Fatalf("local write failure was treated as a missing thumbnail: %v", err)
	}
}
