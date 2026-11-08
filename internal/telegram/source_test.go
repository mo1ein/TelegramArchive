package telegram

import (
	"context"
	"strings"
	"testing"

	"github.com/gotd/td/bin"
	tgclient "github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/mo1ein/TelegramArchive/internal/archive"
)

func TestTakeoutRangesWrapAndPaginate(t *testing.T) {
	var ranges []int
	api := tg.NewClient(tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		takeout, ok := input.(*tg.InvokeWithTakeoutRequest)
		if !ok || takeout.TakeoutID != 99 {
			t.Fatalf("missing takeout wrapper: %T", input)
		}
		var wire bin.Buffer
		if err := takeout.Query.Encode(&wire); err != nil {
			return err
		}
		rangeReq := tg.InvokeWithMessagesRangeRequest{Query: &tg.MessagesGetHistoryRequest{}}
		if err := rangeReq.Decode(&wire); err != nil {
			t.Fatalf("missing message range: %v", err)
		}
		ranges = append(ranges, rangeReq.Range.MinID)
		req := rangeReq.Query.(*tg.MessagesGetHistoryRequest)
		id := 150
		if rangeReq.Range.MinID == 1 {
			id = 50
		}
		if req.OffsetID != 0 {
			t.Fatalf("range offset was not reset: %d", req.OffsetID)
		}
		output.(*tg.MessagesMessagesBox).Messages = &tg.MessagesMessages{Messages: []tg.MessageClass{&tg.Message{ID: id, Date: 100, Message: "test"}}}
		return nil
	}))
	s := &Source{api: api, takeoutID: 99, ranges: []tg.MessageRange{{MinID: 1, MaxID: 99}, {MinID: 100, MaxID: 200}}, chats: map[string]tg.InputPeerClass{"test": &tg.InputPeerSelf{}}}
	chat := archive.Chat{Key: "test"}
	page, err := s.History(context.Background(), chat, "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Done || page.Next != "1:0" || page.Items[0].Message.ID != 150 {
		t.Fatalf("wrong first range: %+v", page)
	}
	page, err = s.History(context.Background(), chat, page.Next)
	if err != nil {
		t.Fatal(err)
	}
	if !page.Done || page.Items[0].Message.ID != 50 || len(ranges) != 2 {
		t.Fatalf("lost older range: %+v %v", page, ranges)
	}
}

func TestTakeoutDelayIsExplicit(t *testing.T) {
	s := &Source{api: tg.NewClient(tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		return tgerr.New(420, "TAKEOUT_INIT_DELAY_123")
	}))}
	err := s.EnableTakeout(context.Background(), []string{"personal"}, nil, 0)
	if err == nil || !strings.Contains(err.Error(), "123 seconds") || s.takeoutID != 0 {
		t.Fatalf("delay hidden: %v", err)
	}
}

func TestNormalHistoryPagination(t *testing.T) {
	s := &Source{api: tg.NewClient(tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		req := input.(*tg.MessagesGetHistoryRequest)
		if req.OffsetID != 500 || req.Limit != 100 {
			t.Fatal(req)
		}
		output.(*tg.MessagesMessagesBox).Messages = &tg.MessagesMessagesSlice{Messages: []tg.MessageClass{&tg.Message{ID: 498, Message: "older"}, &tg.Message{ID: 499, Message: "newer"}}}
		return nil
	})), chats: map[string]tg.InputPeerClass{"test": &tg.InputPeerSelf{}}}
	page, err := s.History(context.Background(), archive.Chat{Key: "test"}, "500")
	if err != nil {
		t.Fatal(err)
	}
	if page.Next != "498" || page.Items[0].Message.ID != 499 {
		t.Fatalf("wrong order/offset: %+v", page)
	}
}

func TestHistoryDecodesCurrentTelegramResponses(t *testing.T) {
	// Constructor IDs and field order come from Telegram's wire schema, rather
	// than the generated client's constants. Older clients reject these responses.
	// https://core.telegram.org/constructor/messages.messagesSlice
	// https://core.telegram.org/constructor/messages.messages
	for _, tc := range []struct {
		name string
		id   uint32
	}{
		{"messagesSlice", 0x5f206716},
		{"messages", 0x1d73e7ea},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := tg.NewClient(tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
				req := input.(*tg.MessagesGetHistoryRequest)
				if req.OffsetID != 500 || req.Limit != 100 {
					t.Fatalf("wrong history request: %+v", req)
				}
				var wire bin.Buffer
				wire.PutID(tc.id)
				if tc.name == "messagesSlice" {
					wire.PutInt(0) // flags
					wire.PutInt(2) // count
				}
				wire.PutVectorHeader(2)
				for _, message := range []tg.Message{
					{ID: 498, PeerID: &tg.PeerUser{UserID: 1}, Date: 100, Message: "older"},
					{ID: 499, PeerID: &tg.PeerUser{UserID: 1}, Date: 101, Message: "newer"},
				} {
					if err := message.Encode(&wire); err != nil {
						return err
					}
				}
				wire.PutVectorHeader(1) // topics, added to both response types
				if err := (&tg.ForumTopicDeleted{ID: 7}).Encode(&wire); err != nil {
					return err
				}
				wire.PutVectorHeader(0) // chats
				wire.PutVectorHeader(0) // users
				if err := output.Decode(&wire); err != nil {
					return err
				}
				if wire.Len() != 0 {
					t.Fatalf("response was only partially decoded: %d remaining bytes", wire.Len())
				}
				return nil
			}))
			s := &Source{api: api, chats: map[string]tg.InputPeerClass{"test": &tg.InputPeerSelf{}}}
			page, err := s.History(context.Background(), archive.Chat{Key: "test", Name: "test"}, "500")
			if err != nil {
				t.Fatal(err)
			}
			if len(page.Items) != 2 || page.Next != "498" || page.Items[0].Message.ID != 499 || page.Items[1].Message.ID != 498 {
				t.Fatalf("lost history or pagination while decoding %s: %+v", tc.name, page)
			}
		})
	}
}

func TestChatTypeSelection(t *testing.T) {
	for _, tc := range []struct {
		kind  string
		types []string
		want  bool
	}{
		{"public_supergroup", []string{"groups"}, true},
		{"private_channel", []string{"private_channels"}, true},
		{"bot_chat", []string{"personal"}, false},
		{"private_group", []string{"public_groups"}, false},
	} {
		if got := matches(archive.Chat{Type: tc.kind}, tc.types); got != tc.want {
			t.Fatalf("%s, %v: %v", tc.kind, tc.types, got)
		}
	}
}
