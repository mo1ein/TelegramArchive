package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gotd/td/bin"
	tgclient "github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"
	"github.com/mo1ein/TelegramArchive/internal/archive"
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

func TestAccountSummaryCountsMediaAndFilters(t *testing.T) {
	api := tg.NewClient(tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		switch input.(type) {
		case *tg.PhotosGetUserPhotosRequest:
			output.(*tg.PhotosPhotosBox).Photos = &tg.PhotosPhotos{Photos: []tg.PhotoClass{
				&tg.Photo{ID: 1, Date: 100, DCID: 4, Sizes: []tg.PhotoSizeClass{&tg.PhotoSize{Type: "x", Size: 4, W: 10, H: 10}}},
				&tg.Photo{ID: 2, Date: 100, DCID: 4, Sizes: []tg.PhotoSizeClass{&tg.PhotoSize{Type: "x", Size: 8, W: 10, H: 10}}},
			}}
		case *tg.UploadGetFileRequest:
			output.(*tg.UploadFileBox).File = &tg.UploadFile{Type: &tg.StorageFileJpeg{}, Bytes: []byte("file")}
		default:
			t.Fatalf("unexpected account request %T", input)
		}
		return nil
	}))
	source := &Source{api: api, accountID: 42, self: &tg.User{ID: 42}, threads: 1, pool: watchdogTestPool{client: func(context.Context, int) *tg.Client { return api }}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	report, err := source.ExportAccount(ctx, t.TempDir(), []string{"profile_photos"}, archive.Options{Concurrency: 1, Format: "both", Media: []string{"photos"}, MaxFileSize: 4})
	counts := report.Summary.Totals()
	if err != nil || !report.Summary.MediaKnown || counts.Downloaded != 1 || counts.Filtered != 1 || report.Summary.DownloadedBytes != 4 {
		t.Fatalf("account outcomes missing: summary=%+v err=%v", report.Summary, err)
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
