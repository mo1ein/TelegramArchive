package telegram

import (
	"context"
	"errors"
	"fmt"
	"github.com/mo1ein/TelegramArchive/internal/archive"
	"testing"
	"time"

	"github.com/gotd/td/bin"
	tgclient "github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"
)

func TestJoinedChannelsIncludeArchiveAndContinuePagination(t *testing.T) {
	seen := map[string]int{}
	api := tg.NewClient(tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		req := input.(*tg.MessagesGetDialogsRequest)
		folder, ok := req.GetFolderID()
		if !ok {
			t.Fatal("folder must be explicit")
		}
		key := fmt.Sprintf("%d/%d", folder, req.OffsetID)
		seen[key]++
		if seen[key] > 1 {
			t.Fatal("dialog pagination loop")
		}
		result := &tg.MessagesDialogsSlice{Count: 3}
		switch key {
		case "0/0":
			result.Dialogs = []tg.DialogClass{&tg.Dialog{Peer: &tg.PeerChannel{ChannelID: 1}, TopMessage: 10}, &tg.Dialog{Peer: &tg.PeerChannel{ChannelID: 99}, TopMessage: 9}}
			result.Chats = []tg.ChatClass{&tg.Channel{ID: 1, AccessHash: 100, Title: "Public", Broadcast: true, Username: "public"}, &tg.ChannelForbidden{ID: 99, AccessHash: 999, Title: "Unavailable", Broadcast: true}}
			result.Messages = []tg.MessageClass{&tg.Message{ID: 10, Date: 100, PeerID: &tg.PeerChannel{ChannelID: 1}}, &tg.Message{ID: 9, Date: 99, PeerID: &tg.PeerChannel{ChannelID: 99}}}
		case "0/9":
			result.Dialogs = []tg.DialogClass{&tg.Dialog{Peer: &tg.PeerChannel{ChannelID: 2}, TopMessage: 8}}
			result.Chats = []tg.ChatClass{&tg.Channel{ID: 2, AccessHash: 200, Title: "Private", Broadcast: true}}
			result.Messages = []tg.MessageClass{&tg.Message{ID: 8, Date: 98, PeerID: &tg.PeerChannel{ChannelID: 2}}}
		case "1/0":
			result.Dialogs = []tg.DialogClass{&tg.Dialog{Peer: &tg.PeerChannel{ChannelID: 3}, TopMessage: 7}}
			result.Chats = []tg.ChatClass{&tg.Channel{ID: 3, AccessHash: 300, Title: "Archived", Broadcast: true}}
			result.Messages = []tg.MessageClass{&tg.Message{ID: 7, Date: 97, PeerID: &tg.PeerChannel{ChannelID: 3}}}
		}
		output.(*tg.MessagesDialogsBox).Dialogs = result
		return nil
	}))
	s := &Source{api: api, manager: peers.Options{}.Build(api), chats: map[string]tg.InputPeerClass{}}
	chats, err := s.Dialogs(context.Background(), []string{"channels"})
	if err != nil {
		t.Fatal(err)
	}
	if len(chats) != 3 {
		t.Fatalf("lost joined channels: %+v", chats)
	}
	if len(s.Warnings) == 0 {
		t.Fatal("unavailable peer was hidden")
	}
	if _, ok := seen["1/0"]; !ok {
		t.Fatal("archive folder not queried")
	}
}

func TestStreamDialogsEmitsBeforeNextPageAndDoesNotFetchTopics(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pages := make(chan int, 1)
	api := tg.NewClient(tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		req, ok := input.(*tg.MessagesGetDialogsRequest)
		if !ok {
			return fmt.Errorf("unexpected eager per-chat request %T", input)
		}
		if req.OffsetID != 0 {
			<-ctx.Done()
			return ctx.Err()
		}
		output.(*tg.MessagesDialogsBox).Dialogs = &tg.MessagesDialogsSlice{Count: 2, Dialogs: []tg.DialogClass{&tg.Dialog{Peer: &tg.PeerChannel{ChannelID: 42}, TopMessage: 10}}, Chats: []tg.ChatClass{&tg.Channel{ID: 42, AccessHash: 42, Title: "Synthetic forum", Megagroup: true, Forum: true}}, Messages: []tg.MessageClass{&tg.Message{ID: 10, Date: 100, PeerID: &tg.PeerChannel{ChannelID: 42}}}}
		return nil
	}))
	source := &Source{api: api, manager: peers.Options{}.Build(api), chats: map[string]tg.InputPeerClass{}}
	done := make(chan error, 1)
	go func() {
		done <- source.StreamDialogs(ctx, []string{"groups"}, func(chats []archive.Chat) error { pages <- len(chats); return nil })
	}()
	select {
	case count := <-pages:
		if count != 1 {
			t.Fatal("wrong first page size")
		}
	case <-time.After(time.Second):
		t.Fatal("chat page hidden behind remaining requests")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation lost: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("stream did not stop")
	}
}
