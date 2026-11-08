package archive

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTMLPaginationEscapingAndLocalMedia(t *testing.T) {
	dir := t.TempDir()
	messages := []Message{
		{ID: 1, Type: "message", Date: "2026-01-01T10:00:00", From: "<Alice>", Text: []any{"hello ", TextEntity{Type: "bold", Text: "😀 سلام"}, TextEntity{Type: "text_link", Text: "unsafe", Href: "javascript:alert(1)"}}, Photo: "photos/my photo.jpg"},
		{ID: 2, Type: "message", Date: "2026-01-01T10:01:00", Text: "<script>globalThis.pwned=true</script>", ReplyTo: 1, File: "files/report.pdf", FileName: "report.pdf"},
		{ID: 3, Type: "message", Date: "2026-01-02T10:01:00", Text: "last", ReplyTo: 1, File: "../outside", Thumbnail: Excluded},
	}
	paths, stats, err := WriteHTML(context.Background(), dir, Chat{Name: "<My chat>", ID: 1}, 2, func(yield func(Message) error) error {
		for _, m := range messages {
			if err := yield(m); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || stats.Messages != 3 {
		t.Fatalf("%v %+v", paths, stats)
	}
	first, _ := os.ReadFile(paths[0])
	second, _ := os.ReadFile(paths[1])
	for _, want := range []string{"&lt;My chat&gt;", "&lt;Alice&gt;", "photos/my%20photo.jpg", "<strong>😀 سلام</strong>", "&lt;script&gt;", "messages2.html", "files/report.pdf"} {
		if !strings.Contains(string(first), want) {
			t.Fatalf("missing %q", want)
		}
	}
	for _, bad := range []string{"javascript:", "<script>", `href="../outside"`, `src="../outside"`} {
		if strings.Contains(string(first)+string(second), bad) {
			t.Fatalf("unsafe HTML: %q", bad)
		}
	}
	if !strings.Contains(string(second), `href="messages.html#go_to_message1"`) {
		t.Fatal("cross-page reply link missing")
	}
	if !strings.Contains(string(second), `href="messages.html"`) {
		t.Fatal("previous-page navigation missing")
	}
}

func TestHTMLFailurePreservesPublishedOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "messages.html")
	os.WriteFile(path, []byte("previous"), 0600)
	want := errors.New("source error")
	_, _, err := WriteHTML(context.Background(), dir, Chat{}, 1, func(yield func(Message) error) error {
		if err := yield(Message{ID: 1, Text: "new"}); err != nil {
			return err
		}
		return want
	})
	if !errors.Is(err, want) {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "previous" {
		t.Fatal("lost previous output")
	}
}

func TestFormatSwitchRendersCheckpointWithoutRedownload(t *testing.T) {
	dir := t.TempDir()
	source := &fakeSource{items: fakeItems(2, 1), pageSize: 2}
	opts := Options{Concurrency: 1, Media: []string{"photos"}, Resume: true, Format: "json"}
	if _, err := Run(context.Background(), source, dir, Chat{ID: 1, Name: "Test"}, opts); err != nil {
		t.Fatal(err)
	}
	result, err := Render(context.Background(), dir, "html", 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Paths) != 2 || source.downloads.Load() != 2 {
		t.Fatalf("%+v downloads %d", result, source.downloads.Load())
	}
}
