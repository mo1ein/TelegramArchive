package telegram

import (
	"context"
	"github.com/gotd/td/bin"
	tgclient "github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"
	"ta/internal/archive"
	"testing"
)

func TestMigratedBasicGroupKeepsOriginalPeerID(t *testing.T) {
	api := tg.NewClient(tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		switch input.(type) {
		case *tg.ChannelsGetFullChannelRequest:
			output.(*tg.MessagesChatFull).FullChat = &tg.ChannelFull{MigratedFromChatID: 7}
		case *tg.MessagesGetChatsRequest:
			output.(*tg.MessagesChatsBox).Chats = &tg.MessagesChats{Chats: []tg.ChatClass{&tg.Chat{ID: 7, Title: "Original group", Deactivated: true}}}
		default:
			t.Fatalf("unexpected %T", input)
		}
		return nil
	}))
	s := &Source{api: api, manager: peers.Options{}.Build(api), chats: map[string]tg.InputPeerClass{"channel_9": &tg.InputPeerChannel{ChannelID: 9}}, accountID: 1}
	chats, err := s.MigratedGroups(context.Background(), []archive.Chat{{ID: 9, Key: "channel_9", Type: "private_supergroup", Name: "Group"}})
	if err != nil || len(chats) != 2 || chats[1].Key != "chat_7" || chats[1].MigratedToID != 9 {
		t.Fatalf("%+v %v", chats, err)
	}
}

func TestProfilePicturesReuseStableMediaNames(t *testing.T) {
	s := &Source{accountID: 42, self: &tg.User{ID: 42, FirstName: "Me"}, api: tg.NewClient(tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		if _, ok := input.(*tg.PhotosGetUserPhotosRequest); !ok {
			t.Fatalf("unexpected %T", input)
		}
		output.(*tg.PhotosPhotosBox).Photos = &tg.PhotosPhotos{Photos: []tg.PhotoClass{&tg.Photo{ID: 123, Date: 100, Sizes: []tg.PhotoSizeClass{&tg.PhotoSize{Type: "x", Size: 4, W: 50, H: 50}}}}}
		return nil
	}))}
	data, files, err := s.accountMedia(context.Background(), t.TempDir(), "profile_photos", archive.Options{Concurrency: 1, Format: "both"})
	if err != nil || len(files) != 2 || data == nil {
		t.Fatalf("%v %v %v", data, files, err)
	}
}
