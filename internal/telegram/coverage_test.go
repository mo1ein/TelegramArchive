package telegram

import (
	"context"
	"github.com/gotd/td/bin"
	tgclient "github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"ta/internal/archive"
	"testing"
)

func TestAccountTakeoutRequestsOnlySelectedCategories(t *testing.T) {
	calls := 0
	s := &Source{api: tg.NewClient(tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		calls++
		req, ok := input.(*tg.AccountInitTakeoutSessionRequest)
		if !ok {
			t.Fatalf("unexpected request %T", input)
		}
		if req.MessageUsers || req.MessageChats || req.MessageChannels || req.Files || req.Contacts {
			t.Fatal("session-only export requested unrelated data")
		}
		output.(*tg.AccountTakeout).ID = 9
		return nil
	}))}
	if err := s.EnableAccountTakeout(context.Background(), []string{"sessions"}, archive.Options{}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal(calls)
	}
}

func TestPrivateChannelFallsBackToOwnMessages(t *testing.T) {
	s := &Source{chats: map[string]tg.InputPeerClass{"test": &tg.InputPeerChannel{ChannelID: 1}}, api: tg.NewClient(tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		switch req := input.(type) {
		case *tg.MessagesGetHistoryRequest:
			return tgerr.New(403, "CHANNEL_PRIVATE")
		case *tg.MessagesSearchRequest:
			if _, ok := req.FromID.(*tg.InputPeerSelf); !ok {
				t.Fatal("fallback must request only the account's messages")
			}
			output.(*tg.MessagesMessagesBox).Messages = &tg.MessagesMessages{Messages: []tg.MessageClass{&tg.Message{ID: 1, Date: 100, Message: "my old message"}}}
			return nil
		default:
			t.Fatalf("unexpected %T", input)
		}
		return nil
	}))}
	page, err := s.History(context.Background(), archive.Chat{Key: "test"}, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("%+v %v", page, err)
	}
}

func TestServiceActionsUseDesktopNamesAndMembers(t *testing.T) {
	e := peer.NewEntities(map[int64]*tg.User{1: {ID: 1, FirstName: "Alice"}}, nil, nil)
	item := Convert(&tg.MessageService{ID: 1, Action: &tg.MessageActionChatCreate{Title: "Group", Users: []int64{1}}}, e, 0)
	if item.Message.Action != "create_group" || item.Message.Title != "Group" || len(item.Message.Members) != 1 || item.Message.Members[0] != "Alice" {
		t.Fatal(item.Message)
	}
}
