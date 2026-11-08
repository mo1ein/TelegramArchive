package telegram

import (
	"context"
	"github.com/gotd/td/bin"
	tgclient "github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"ta/internal/archive"
	"testing"
)

func TestCustomEmojiAssetsAreBatchedAndStable(t *testing.T) {
	calls := 0
	s := &Source{DownloadCustomEmoji: true, api: tg.NewClient(tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		calls++
		req := input.(*tg.MessagesGetCustomEmojiDocumentsRequest)
		if len(req.DocumentID) != 1 || req.DocumentID[0] != 99 {
			t.Fatal(req)
		}
		output.(*tg.DocumentClassVector).Elems = []tg.DocumentClass{&tg.Document{ID: 99, Size: 4, MimeType: "image/webp"}}
		return nil
	}))}
	items := []archive.Item{{Message: archive.Message{ID: 1, TextEntities: []archive.TextEntity{{Type: "custom_emoji", DocumentID: "99"}}}}, {Message: archive.Message{ID: 2, TextEntities: []archive.TextEntity{{Type: "custom_emoji", DocumentID: "99"}}}}}
	if err := s.attachCustomEmojis(context.Background(), items); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(items[0].Files) != 1 || items[0].Files[0].Path != items[1].Files[0].Path {
		t.Fatalf("calls=%d items=%+v", calls, items)
	}
}
