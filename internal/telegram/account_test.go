package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gotd/td/bin"
	tgclient "github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"
	"ta/internal/archive"
)

func TestAccountMetadataExcludesAuthorizationHashes(t *testing.T) {
	s := &Source{accountID: 42, self: &tg.User{ID: 42, FirstName: "Me"}, api: tg.NewClient(tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		switch input.(type) {
		case *tg.AccountGetAuthorizationsRequest:
			output.(*tg.AccountAuthorizations).Authorizations = []tg.Authorization{{Hash: 9223372036854775806, DeviceModel: "Laptop", AppName: "Telegram", IP: "127.0.0.1", DateCreated: 100, DateActive: 200}}
		case *tg.AccountGetWebAuthorizationsRequest:
			output.(*tg.AccountWebAuthorizations).Authorizations = []tg.WebAuthorization{{Hash: 999, BotID: 7, Domain: "example.com"}}
		default:
			t.Fatalf("unexpected account request %T", input)
		}
		return nil
	}))}
	dir := t.TempDir()
	report, err := s.ExportAccount(context.Background(), dir, []string{"sessions", "web_sessions"}, archive.Options{Concurrency: 1, Format: "both"})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Sections) != 2 {
		t.Fatal(report)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "account.json"))
	var data map[string]any
	if json.Unmarshal(b, &data) != nil {
		t.Fatal(string(b))
	}
	if data["sessions"] == nil || data["web_sessions"] == nil {
		t.Fatal(data)
	}
	if bytes.Contains(b, []byte("9223372036854775806")) || bytes.Contains(b, []byte(`"hash"`)) {
		t.Fatal("authorization hash leaked into backup")
	}
}

func TestSavedMessagesAreNotPartOfAllPersonalChats(t *testing.T) {
	api := tg.NewClient(tgclient.InvokeFunc(func(context.Context, bin.Encoder, bin.Decoder) error { return nil }))
	s := &Source{manager: peers.Options{}.Build(api), chats: map[string]tg.InputPeerClass{}, accountID: 42}
	chat := s.addChat(s.manager.User(&tg.User{ID: 42, Self: true, FirstName: "Me"}))
	if chat.Type != "saved_messages" || matches(chat, []string{"personal"}) {
		t.Fatalf("saved messages must live in account backup: %+v", chat)
	}
}
