package archive

import (
	"context"
	"encoding/json"
	bolt "go.etcd.io/bbolt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDesktopJSONSchema(t *testing.T) {
	dir := t.TempDir()
	rows := []Message{
		{ID: 1, Type: "message", Date: "2026-01-02T03:04:05", From: "Alice", FromID: "user42", Text: []any{"Hello ", TextEntity{Type: "bold", Text: "سلام", Offset: 6, Length: 4}}, TextEntities: []TextEntity{{Type: "bold", Text: "سلام", Offset: 6, Length: 4}}, TopicID: 9, CustomEmojiFiles: map[string]string{"123": "stickers/123.webp"}, ExportScope: "own_messages", File: "files/report.pdf", FileSize: 9, FileName: "report.pdf", MediaType: "file", ExtraMedia: map[string]int{"extra": 1}},
		{ID: 2, Type: "service", Date: "2026-01-02T04:04:05", Actor: "Alice", ActorID: "user42", Action: "pin_message", ReplyTo: 1, Text: "", ActionData: map[string]int{"internal": 7}},
	}
	_, _, err := Write(context.Background(), dir, Chat{Name: "Team", Username: "team", ID: 10, Type: "private_supergroup", OwnMessagesOnly: true}, 0, func(yield func(Message) error) error {
		for _, m := range rows {
			if err := yield(m); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got, want any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	// Field names, nesting, types and omission rules from Desktop v7.4.1.
	const fixture = `{"name":"Team","type":"private_supergroup","id":10,"messages":[{"id":1,"type":"message","date":"2026-01-02T03:04:05","from":"Alice","from_id":"user42","text":["Hello ",{"type":"bold","text":"سلام"}],"text_entities":[{"type":"plain","text":"Hello "},{"type":"bold","text":"سلام"}],"file":"files/report.pdf","file_name":"report.pdf","file_size":9},{"id":2,"type":"service","date":"2026-01-02T04:04:05","actor":"Alice","actor_id":"user42","action":"pin_message","reply_to_message_id":1,"text":"","text_entities":[]}]}`
	json.Unmarshal([]byte(fixture), &want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Desktop schema differs:\n%s", b)
	}
	if rows[0].TextEntities[0].Offset != 6 {
		t.Fatal("export serialization changed checkpoint entities")
	}
	saved, err := desktopChatJSON(Chat{Name: "Saved messages", Type: "saved_messages", ID: 42})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved), `"name"`) {
		t.Fatal("Desktop omits name for Saved Messages")
	}
}

func TestDesktopAccountSchemaAndSavedMessagePaths(t *testing.T) {
	dir := t.TempDir()
	source := &fakeSource{items: fakeItems(2, 1), pageSize: 2}
	_, err := Run(context.Background(), source, filepath.Join(dir, "saved_messages"), Chat{ID: 42, Name: "Saved Messages", Type: "saved_messages"}, Options{Concurrency: 1, Media: []string{"photos"}, PartSize: 200})
	if err != nil {
		t.Fatal(err)
	}
	report := AccountReport{AccountID: 42, Sections: []AccountSection{
		{Name: "personal_info", Data: map[string]any{"user_id": int64(42), "first_name": "Alice", "last_name": "A", "phone_number": "123", "username": "alice"}},
		{Name: "profile_photos", Data: map[string]any{"list": []any{map[string]any{"id": 1, "date": "2026-01-02T03:04:05", "date_unixtime": "1767323045", "photo": "profile_pictures/photo.jpg", "photo_file_size": 10}}}},
		{Name: "stories", Data: map[string]any{"list": []any{map[string]any{"id": 3, "date": "2026-01-02T03:04:05", "expires": "2026-01-03T03:04:05", "photo": "stories/photo.jpg", "pinned": false, "text": "caption"}}}},
		{Name: "contacts", Data: map[string]any{"source": "saved_phone_contacts", "list": []any{map[string]any{"first_name": "Roya", "last_name": "", "phone_number": "456", "date": "2026-01-02T03:04:05"}}}},
		{Name: "saved_messages", Files: []string{"saved_messages/result_part1.json"}},
	}}
	if err := WriteAccount(dir, "both", report); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"account_id", "exported_at", "saved_messages"} {
		if data[key] != nil {
			t.Fatalf("non-Desktop root field %s", key)
		}
	}
	personal := data["personal_information"].(map[string]any)
	if personal["username"] != "@alice" || personal["phone_number"] != "+123" {
		t.Fatal(personal)
	}
	pictures := data["profile_pictures"].([]any)
	if len(pictures) != 1 || pictures[0].(map[string]any)["id"] != nil {
		t.Fatal(pictures)
	}
	stories := data["stories"].([]any)
	story := stories[0].(map[string]any)
	if story["media"] != "stories/photo.jpg" || story["expires_unixtime"] == nil || story["id"] != nil || story["text"] != nil {
		t.Fatal(story)
	}
	contacts := data["contacts"].(map[string]any)
	if contacts["about"] == nil || contacts["source"] != nil || contacts["list"].([]any)[0].(map[string]any)["date_unixtime"] == nil {
		t.Fatal(contacts)
	}
	chats := data["chats"].(map[string]any)["list"].([]any)
	if len(chats) != 1 {
		t.Fatal(chats)
	}
	saved := chats[0].(map[string]any)
	if saved["name"] != nil || saved["type"] != "saved_messages" || saved["id"] != float64(42) {
		t.Fatal(saved)
	}
	messages := saved["messages"].([]any)
	if len(messages) != 2 || !strings.HasPrefix(messages[0].(map[string]any)["photo"].(string), "saved_messages/") {
		t.Fatal(messages)
	}
	alias, _ := os.ReadFile(filepath.Join(dir, "account.json"))
	if string(alias) != string(b) {
		t.Fatal("account alias differs")
	}
	result, err := Render(context.Background(), dir, "both", 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Paths) != 2 {
		t.Fatal(result.Paths)
	}
	if _, err := os.Stat(filepath.Join(dir, "saved_messages", "messages2.html")); err != nil {
		t.Fatal("account render did not update Saved Messages", err)
	}
}

func TestDesktopRenderUpgradesLegacyCacheAndRepairsAssets(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	source := &fakeSource{items: fakeItems(1), pageSize: 2}
	chat := Chat{ID: 1, Name: "Team"}
	opts := Options{Concurrency: 1, Resume: true, Format: "both"}
	if _, err := Run(ctx, source, dir, chat, opts); err != nil {
		t.Fatal(err)
	}
	db, err := bolt.Open(filepath.Join(dir, "archive.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		var cache renderCache
		if err := json.Unmarshal(tx.Bucket(metaBucket).Get(renderCacheKey), &cache); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "messages.html"), []byte("legacy layout"), 0600); err != nil {
			return err
		}
		cache.Version = 0
		for i := range cache.Files {
			if cache.Files[i].Path == "messages.html" {
				st, err := os.Stat(filepath.Join(dir, "messages.html"))
				if err != nil {
					return err
				}
				cache.Files[i].Size, cache.Files[i].Modified = st.Size(), st.ModTime()
			}
		}
		b, err := json.Marshal(cache)
		if err != nil {
			return err
		}
		return tx.Bucket(metaBucket).Put(renderCacheKey, b)
	})
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, source, dir, chat, opts); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "messages.html"))
	if !strings.Contains(string(b), "css/style.css") {
		t.Fatal("legacy output was reused")
	}
	asset := filepath.Join(dir, "css", "style.css")
	if err := os.Remove(asset); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, source, dir, chat, opts); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(asset); err != nil {
		t.Fatal("missing asset was not repaired", err)
	}
}

func TestDesktopHTMLStructureAndAssets(t *testing.T) {
	dir := t.TempDir()
	rows := []Message{{ID: 1, Type: "message", Date: "2026-01-02T10:00:00", From: "Alice Adams", FromID: "user42", Text: "Hello"}, {ID: 2, Type: "message", Date: "2026-01-02T10:01:00", From: "Alice Adams", FromID: "user42", Text: "Again", ReplyTo: 1}, {ID: 3, Type: "message", Date: "2026-01-03T10:01:00", From: "Alice Adams", FromID: "user42", Text: "Next day"}}
	_, _, err := WriteHTML(context.Background(), dir, Chat{Name: "Team"}, 1000, func(yield func(Message) error) error {
		for _, m := range rows {
			if err := yield(m); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "messages.html"))
	s := string(b)
	for _, want := range []string{`<title>Exported Data</title>`, `class="page_wrap"`, `class="page_header"`, `class="page_body chat_page"`, `class="history"`, `class="message default clearfix joined"`, `class="initials"`, `2 January 2026`, `3 January 2026`, `return GoToMessage(`} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing Desktop DOM %q", want)
		}
	}
	if strings.Count(s, `class="from_name"`) != 2 {
		t.Fatal("messages were not grouped like Desktop")
	}
	for _, path := range desktopAssetPaths() {
		got, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil {
			t.Fatal(err)
		}
		want, _ := desktopAssets.ReadFile("desktop_assets/" + path)
		if string(got) != string(want) {
			t.Fatalf("modified upstream asset %s", path)
		}
	}
}
