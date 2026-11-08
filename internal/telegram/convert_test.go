package telegram

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/tg"
)

func TestUTF16EntitiesPreserveText(t *testing.T) {
	text, entities := convertText("Hi 😀 سلام!", []tg.MessageEntityClass{&tg.MessageEntityBold{Offset: 3, Length: 2}, &tg.MessageEntityTextURL{Offset: 6, Length: 4, URL: "https://example.com"}})
	b, err := json.Marshal(text)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "😀") || entities[1].Text != "😀" || entities[3].Text != "سلام" {
		t.Fatalf("bad entities: %s %+v", b, entities)
	}
	if entities[3].Href != "https://example.com" || entities[3].Type != "text_link" {
		t.Fatal("lost link URL")
	}
}

func TestOutgoingPrivateSenderAndCaption(t *testing.T) {
	entities := peer.NewEntities(map[int64]*tg.User{1: {ID: 1, FirstName: "Me"}, 7: {ID: 7, FirstName: "Recipient"}}, nil, nil)
	item := Convert(&tg.Message{ID: 1, Out: true, PeerID: &tg.PeerUser{UserID: 7}, Message: "photo caption"}, entities, 1)
	if item.Message.FromID != "user1" || item.Message.From != "Me" {
		t.Fatalf("wrong outgoing sender: %+v", item.Message)
	}
}

func TestEditedAttachmentHasDifferentCachePath(t *testing.T) {
	doc := &tg.Document{ID: 9, Size: 42, Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeFilename{FileName: "same.txt"}}}
	m := &tg.Message{ID: 1, Media: &tg.MessageMediaDocument{Document: doc}}
	before := Convert(m, peer.Entities{}, 0)
	doc.ID = 10
	after := Convert(m, peer.Entities{}, 0)
	if before.Files[0].Path == after.Files[0].Path {
		t.Fatal("replacement attachment could reuse stale content of the same size")
	}
}

func TestDocumentClassificationAndSafeFileNames(t *testing.T) {
	for _, tc := range []struct {
		attr []tg.DocumentAttributeClass
		kind string
	}{
		{[]tg.DocumentAttributeClass{&tg.DocumentAttributeVideo{RoundMessage: true}}, "video_messages"},
		{[]tg.DocumentAttributeClass{&tg.DocumentAttributeAnimated{}, &tg.DocumentAttributeVideo{}}, "animations"},
		{[]tg.DocumentAttributeClass{&tg.DocumentAttributeAudio{Voice: true}}, "voice_messages"},
		{[]tg.DocumentAttributeClass{&tg.DocumentAttributeSticker{}, &tg.DocumentAttributeVideo{}}, "stickers"},
	} {
		attrs := append(tc.attr, &tg.DocumentAttributeFilename{FileName: "../../same.txt"})
		m := &tg.Message{ID: 2, Date: 1767323045, Message: "caption", Media: &tg.MessageMediaDocument{Document: &tg.Document{ID: 9, Size: 42, MimeType: "video/mp4", Attributes: attrs}}}
		item := Convert(m, peer.Entities{}, 0)
		if len(item.Files) == 0 || item.Files[0].Kind != tc.kind || strings.Contains(item.Files[0].Path, "..") {
			t.Fatalf("bad file: %+v", item.Files)
		}
		if item.Message.Text != "caption" {
			t.Fatal("lost caption")
		}
	}
}

func TestSenderAndServiceAndEmptyPhoto(t *testing.T) {
	entities := peer.NewEntities(map[int64]*tg.User{7: {ID: 7, FirstName: "Alice"}}, nil, map[int64]*tg.Channel{7: {ID: 7, Title: "Channel"}})
	item := Convert(&tg.Message{ID: 1, FromID: &tg.PeerUser{UserID: 7}, Media: &tg.MessageMediaPhoto{Photo: &tg.Photo{ID: 1}}}, entities, 0)
	if item.Message.From != "Alice" || item.Message.FromID != "user7" {
		t.Fatal(item.Message)
	}
	service := Convert(&tg.MessageService{ID: 2, FromID: &tg.PeerUser{UserID: 7}, Action: &tg.MessageActionPinMessage{}, ReplyTo: &tg.MessageReplyHeader{ReplyToMsgID: 1}}, entities, 0)
	if service.Message.Type != "service" || service.Message.Actor != "Alice" || service.Message.Action != "pin_message" || service.Message.ReplyTo != 1 {
		t.Fatal(service.Message)
	}
}
