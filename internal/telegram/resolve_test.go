package telegram

import (
	"context"
	"strings"
	"testing"

	"github.com/gotd/td/bin"
	tgclient "github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

func nameResolver(t *testing.T, users ...*tg.User) *Source {
	t.Helper()
	api := tg.NewClient(tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		switch request := input.(type) {
		case *tg.MessagesGetDialogsRequest:
			result := &tg.MessagesDialogs{}
			folder, _ := request.GetFolderID()
			for _, user := range users {
				// Put personal chats in the archive folder to exercise both folders.
				if folder == 1 {
					result.Dialogs = append(result.Dialogs, &tg.Dialog{Peer: &tg.PeerUser{UserID: user.ID}})
					result.Users = append(result.Users, user)
				}
			}
			output.(*tg.MessagesDialogsBox).Dialogs = result
		case *tg.ContactsResolveUsernameRequest:
			if request.Username != "mammad" {
				return tgerr.New(400, "USERNAME_NOT_OCCUPIED")
			}
			*output.(*tg.ContactsResolvedPeer) = tg.ContactsResolvedPeer{
				Peer: &tg.PeerUser{UserID: 99}, Users: []tg.UserClass{&tg.User{ID: 99, AccessHash: 999, FirstName: "Public username owner", Username: "mammad"}},
			}
		default:
			t.Fatalf("unexpected lookup: %T", input)
		}
		return nil
	}))
	return &Source{api: api, manager: peers.Options{}.Build(api), chats: map[string]tg.InputPeerClass{}}
}

func TestResolvePrefersDisplayedNameOverPublicUsername(t *testing.T) {
	for _, ref := range []string{"mammad", " MAMMAD "} {
		source := nameResolver(t, &tg.User{ID: 10, AccessHash: 100, FirstName: "mammad", Username: "my_friend", Contact: true})
		chat, err := source.Resolve(context.Background(), ref)
		if err != nil || chat.ID != 10 || chat.Name != "mammad" {
			t.Fatalf("ref=%q resolved=%+v err=%v", ref, chat, err)
		}
		if source.chats[chat.Key] == nil {
			t.Fatal("display-name match did not retain its access hash")
		}
	}
}

func TestResolveExplicitUsernameAndURLKeepTheirMeaning(t *testing.T) {
	for _, ref := range []string{"@mammad", "https://t.me/mammad"} {
		source := nameResolver(t, &tg.User{ID: 10, AccessHash: 100, FirstName: "mammad"})
		chat, err := source.Resolve(context.Background(), ref)
		if err != nil || chat.ID != 99 {
			t.Fatalf("ref=%q resolved=%+v err=%v", ref, chat, err)
		}
	}
}

func TestResolveContactWithoutUsernameByFullDisplayName(t *testing.T) {
	for _, name := range []string{"mammad", "Mammad Ahmadi"} {
		first, last, _ := strings.Cut(name, " ")
		source := nameResolver(t, &tg.User{ID: 10, AccessHash: 100, FirstName: first, LastName: last, Contact: true})
		chat, err := source.Resolve(context.Background(), "  "+strings.ReplaceAll(strings.ToUpper(name), " ", "   ")+"  ")
		if err != nil || chat.ID != 10 || chat.Username != "" {
			t.Fatalf("name=%q resolved=%+v err=%v", name, chat, err)
		}
	}
}

func TestResolveUnmatchedDisplayedNameFallsBackToUsername(t *testing.T) {
	source := nameResolver(t, &tg.User{ID: 10, AccessHash: 100, FirstName: "Someone else"})
	chat, err := source.Resolve(context.Background(), "mammad")
	if err != nil || chat.ID != 99 {
		t.Fatalf("username fallback failed: chat=%+v err=%v", chat, err)
	}
}

func TestResolveAmbiguousDisplayedNameDoesNotChoosePublicUsername(t *testing.T) {
	source := nameResolver(t, &tg.User{ID: 10, AccessHash: 100, FirstName: "mammad"}, &tg.User{ID: 11, AccessHash: 110, FirstName: "mammad"})
	_, err := source.Resolve(context.Background(), "mammad")
	if err == nil || !strings.Contains(err.Error(), "multiple") || !strings.Contains(err.Error(), "10") || !strings.Contains(err.Error(), "11") {
		t.Fatalf("ambiguous name was not explained: %v", err)
	}
}
