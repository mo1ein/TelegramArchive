package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	fasterrors "github.com/go-faster/errors"
	"github.com/gotd/td/tdsync"
	"github.com/gotd/td/tgerr"
	"github.com/mo1ein/TelegramArchive/internal/archive"
	transport "github.com/mo1ein/TelegramArchive/internal/telegram"
	"go.uber.org/zap"
)

func TestRetryTelegramTransportFailures(t *testing.T) {
	for _, failure := range []error{
		&transport.DownloadStalledError{Path: "video_files/thumbnail.jpg", IdleTimeout: time.Minute},
		fmt.Errorf("group: task readLoop: read intermediate: read length: %w", io.EOF),
		io.ErrUnexpectedEOF,
		io.ErrClosedPipe,
		net.ErrClosed,
		&net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET},
		context.DeadlineExceeded,
		context.Canceled, // Only a child context was canceled; the parent is active.
		errors.Join(context.Canceled, io.EOF),
	} {
		t.Run(failure.Error(), func(t *testing.T) {
			calls, notifications := 0, 0
			err := retryTelegram(context.Background(), backoff.WithMaxRetries(backoff.NewConstantBackOff(0), 2), func() error {
				calls++
				if calls == 1 {
					return failure
				}
				return nil
			}, func(error, time.Duration) { notifications++ })
			if err != nil || calls != 2 || notifications != 1 {
				t.Fatalf("err=%v, attempts=%d, notifications=%d", err, calls, notifications)
			}
		})
	}
}

func TestRetryTelegramPermanentFailures(t *testing.T) {
	for _, failure := range []error{
		tgerr.New(401, "AUTH_KEY_UNREGISTERED"),
		&os.PathError{Op: "write", Path: "archive.db", Err: os.ErrPermission},
		errors.New("archive settings changed"),
		errors.Join(io.EOF, tgerr.New(400, "CHANNEL_PRIVATE")),
		errors.Join(io.EOF, &os.PathError{Op: "write", Path: "result.json", Err: os.ErrPermission}),
	} {
		t.Run(failure.Error(), func(t *testing.T) {
			calls := 0
			err := retryTelegram(context.Background(), backoff.WithMaxRetries(backoff.NewConstantBackOff(0), 2), func() error {
				calls++
				return failure
			}, nil)
			if !errors.Is(err, failure) || calls != 1 {
				t.Fatalf("err=%v, attempts=%d", err, calls)
			}
		})
	}
}

func TestRetryTelegramInternalSessionCancellation(t *testing.T) {
	parent := context.Background()
	attempts := 0
	err := retryTelegram(parent, backoff.WithMaxRetries(backoff.NewConstantBackOff(0), 2), func() error {
		attempts++
		return runTelegramSession(parent, func(ctx context.Context, callback func(context.Context) error) error {
			child, cancel := context.WithCancel(ctx)
			defer cancel()
			if attempts == 1 {
				cancel()
			}
			_ = callback(child)
			return nil // gotd Run suppresses errors wrapping context.Canceled.
		}, func(ctx context.Context) error { return ctx.Err() })
	}, nil)
	if err != nil || attempts != 2 {
		t.Fatalf("internal cancellation was lost: err=%v, sessions=%d", err, attempts)
	}
}

func TestRetryTelegramReadLoopErrorChain(t *testing.T) {
	parent := context.Background()
	attempts := 0
	err := retryTelegram(parent, backoff.WithMaxRetries(backoff.NewConstantBackOff(0), 2), func() error {
		attempts++
		if attempts > 1 {
			return nil
		}
		group := tdsync.NewLogGroup(parent, zap.NewNop())
		group.Go("download", func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		})
		group.Go("readLoop", func(context.Context) error {
			return fasterrors.Wrap(io.EOF, "read: read intermediate: read length")
		})
		return fasterrors.Wrap(group.Wait(), "group")
	}, nil)
	if err != nil || attempts != 2 || parent.Err() != nil {
		t.Fatalf("readLoop EOF was not recovered: err=%v, sessions=%d", err, attempts)
	}
}

func TestRunTelegramSessionPreservesFailures(t *testing.T) {
	callbackFailure := errors.New("archive settings changed")
	for _, clientFailure := range []error{nil, io.EOF} {
		err := runTelegramSession(context.Background(), func(ctx context.Context, callback func(context.Context) error) error {
			_ = callback(ctx)
			return clientFailure
		}, func(context.Context) error { return callbackFailure })
		if !errors.Is(err, callbackFailure) {
			t.Fatalf("callback failure was lost: %v", err)
		}
		if clientFailure != nil && !errors.Is(err, clientFailure) {
			t.Fatalf("transport failure was lost: %v", err)
		}
	}
}

func TestRunTelegramSessionRequiresCallback(t *testing.T) {
	err := runTelegramSession(context.Background(), func(context.Context, func(context.Context) error) error {
		return nil
	}, func(context.Context) error {
		t.Fatal("callback should not have been invoked by this client")
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("an operation that never started was reported as successful: %v", err)
	}
}

func TestRetryTelegramLimit(t *testing.T) {
	calls := 0
	err := retryTelegram(context.Background(), backoff.WithMaxRetries(backoff.NewConstantBackOff(0), 2), func() error {
		calls++
		return io.EOF
	}, nil)
	if !errors.Is(err, io.EOF) || calls != 3 {
		t.Fatalf("err=%v, attempts=%d", err, calls)
	}
}

func TestRetryTelegramCancellation(t *testing.T) {
	t.Run("before attempt", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := retryTelegram(ctx, backoff.NewConstantBackOff(0), func() error {
			t.Fatal("started an attempt after cancellation")
			return nil
		}, nil)
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	})
	t.Run("during attempt", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		err := retryTelegram(ctx, backoff.NewConstantBackOff(0), func() error {
			cancel()
			return nil // gotd can return nil when its parent context is canceled.
		}, nil)
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	})
	t.Run("during backoff", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		calls := 0
		err := retryTelegram(ctx, backoff.NewConstantBackOff(time.Hour), func() error {
			calls++
			return io.EOF
		}, func(error, time.Duration) { cancel() })
		if !errors.Is(err, context.Canceled) || calls != 1 {
			t.Fatalf("err=%v, attempts=%d", err, calls)
		}
	})
}

type reconnectArchiveSource struct {
	fail      bool
	downloads map[string]int
}

func (s *reconnectArchiveSource) History(_ context.Context, _ archive.Chat, cursor string) (archive.Page, error) {
	offset, _ := strconv.Atoi(cursor)
	page := archive.Page{}
	for id := 4; id > 0; id-- {
		if offset != 0 && id >= offset {
			continue
		}
		page.Items = append(page.Items, archive.Item{
			Message: archive.Message{ID: id, Type: "message", Text: strconv.Itoa(id)},
			Files:   []archive.File{{Kind: "videos", Field: "file", Path: fmt.Sprintf("video_files/%d.mp4", id), Size: 4}},
		})
		page.Next = strconv.Itoa(id)
		if len(page.Items) == 2 {
			break
		}
	}
	page.Done = len(page.Items) < 2 || page.Next == "1"
	return page, nil
}

func (s *reconnectArchiveSource) Download(_ context.Context, file archive.File, dest string) error {
	s.downloads[file.Path]++
	if s.fail && file.Path == "video_files/1.mp4" {
		if err := os.WriteFile(dest, []byte("bad"), 0600); err != nil {
			return err
		}
		return fmt.Errorf("read intermediate: read length: %w", io.EOF)
	}
	return os.WriteFile(dest, []byte("data"), 0600)
}

func TestRetryTelegramResumesArchiveAfterEOF(t *testing.T) {
	dir := t.TempDir()
	downloads := map[string]int{}
	attempts := 0
	err := retryTelegram(context.Background(), backoff.WithMaxRetries(backoff.NewConstantBackOff(0), 2), func() error {
		attempts++
		source := &reconnectArchiveSource{fail: attempts == 1, downloads: downloads}
		_, err := archive.Run(context.Background(), source, dir, archive.Chat{ID: 42, AccountID: 7, Name: "test"}, archive.Options{Concurrency: 1, Resume: true, Media: []string{"videos"}})
		return err
	}, nil)
	if err != nil || attempts != 2 {
		t.Fatalf("err=%v, sessions=%d", err, attempts)
	}
	for id := 1; id <= 4; id++ {
		want := 1
		if id == 1 {
			want = 2
		}
		name := fmt.Sprintf("video_files/%d.mp4", id)
		if downloads[name] != want {
			t.Fatalf("%s downloaded %d times; want %d", name, downloads[name], want)
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(data) != "data" {
			t.Fatalf("invalid completed media %s: %q, %v", name, data, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result struct{ Messages []archive.Message }
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Messages) != 4 {
		t.Fatalf("lost messages: %d", len(result.Messages))
	}
	for i, message := range result.Messages {
		if message.ID != i+1 {
			t.Fatalf("message ordering or duplication: %+v", result.Messages)
		}
	}
	temps, err := filepath.Glob(filepath.Join(dir, "video_files", ".download-*"))
	if err != nil || len(temps) != 0 {
		t.Fatalf("interrupted files were not cleaned up: %v, %v", temps, err)
	}
}
