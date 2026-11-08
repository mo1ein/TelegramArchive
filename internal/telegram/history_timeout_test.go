package telegram

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/td/bin"
	tgclient "github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/mo1ein/TelegramArchive/internal/archive"
)

func TestHistoryWatchdogStopsStalledPage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	api := tg.NewClient(DownloadActivityMiddleware().Handle(tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		<-ctx.Done()
		return ctx.Err()
	})))
	chat := archive.Chat{Key: "user_10", Name: "Fadi"}
	source := &Source{api: api, DownloadTimeout: 60 * time.Millisecond, chats: map[string]tg.InputPeerClass{chat.Key: &tg.InputPeerSelf{}}}
	_, err := source.History(ctx, chat, "")
	var stalled *DownloadStalledError
	var network net.Error
	if !errors.As(err, &stalled) || !errors.As(err, &network) || !network.Timeout() {
		t.Fatalf("history request did not report a retryable idle timeout: %v", err)
	}
	if !strings.Contains(strings.ToLower(stalled.Path), "history") || (!strings.Contains(stalled.Path, chat.Name) && !strings.Contains(stalled.Path, chat.Key)) {
		t.Fatalf("timeout did not identify the chat's history: %+v", stalled)
	}
	if ctx.Err() != nil {
		t.Fatal("history watchdog expired the parent context")
	}
}

func TestHistoryWatchdogPreservesParentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	api := tg.NewClient(tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		cancel()
		return ctx.Err()
	}))
	chat := archive.Chat{Key: "user_10", Name: "Fadi"}
	source := &Source{api: api, DownloadTimeout: time.Second, chats: map[string]tg.InputPeerClass{chat.Key: &tg.InputPeerSelf{}}}
	_, err := source.History(ctx, chat, "")
	var stalled *DownloadStalledError
	if !errors.Is(err, context.Canceled) || errors.As(err, &stalled) {
		t.Fatalf("parent cancellation became a history timeout: %v", err)
	}
}

func TestHistoryWatchdogGivesEachPageAFreshBudget(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	var calls atomic.Int32
	api := tg.NewClient(DownloadActivityMiddleware().Handle(tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		request := input.(*tg.MessagesGetHistoryRequest)
		calls.Add(1)
		timer := time.NewTimer(100 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
		top := 300
		if request.OffsetID > 0 {
			top = request.OffsetID - 1
		}
		result := &tg.MessagesMessagesSlice{}
		for id := top; id > 0 && len(result.Messages) < 100; id-- {
			result.Messages = append(result.Messages, &tg.Message{ID: id, Message: "test"})
		}
		output.(*tg.MessagesMessagesBox).Messages = result
		return nil
	})))
	chat := archive.Chat{Key: "user_10", Name: "Fadi"}
	source := &Source{api: api, DownloadTimeout: 250 * time.Millisecond, chats: map[string]tg.InputPeerClass{chat.Key: &tg.InputPeerSelf{}}}
	// Four individually responsive pages take longer than one idle budget.
	// A single timeout around the whole scan would incorrectly cancel it.
	cursor, messages := "", 0
	for iteration := 0; ; iteration++ {
		if iteration > 4 {
			t.Fatal("history pagination did not finish")
		}
		page, err := source.History(ctx, chat, cursor)
		if err != nil {
			t.Fatal(err)
		}
		messages += len(page.Items)
		if page.Done {
			break
		}
		cursor = page.Next
	}
	if messages != 300 || calls.Load() != 4 || ctx.Err() != nil {
		t.Fatalf("history scan was truncated: messages=%d pages=%d parent=%v", messages, calls.Load(), ctx.Err())
	}
}

func TestHistoryWatchdogHonorsTelegramFloodWait(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	var calls atomic.Int32
	rpc := tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		if calls.Add(1) == 1 {
			return tgerr.New(420, "FLOOD_WAIT_1")
		}
		output.(*tg.MessagesMessagesBox).Messages = &tg.MessagesMessages{Messages: []tg.MessageClass{&tg.Message{ID: 1, Message: "test"}}}
		return nil
	})
	api := tg.NewClient(floodwait.NewSimpleWaiter().Handle(DownloadActivityMiddleware().Handle(rpc)))
	chat := archive.Chat{Key: "user_10", Name: "Fadi"}
	source := &Source{api: api, DownloadTimeout: 150 * time.Millisecond, chats: map[string]tg.InputPeerClass{chat.Key: &tg.InputPeerSelf{}}}
	page, err := source.History(ctx, chat, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].Message.ID != 1 || calls.Load() != 2 {
		t.Fatalf("history flood-wait retry did not finish: page=%+v calls=%d err=%v", page, calls.Load(), err)
	}
}
