package archive

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFileFiltersApplyToAttachmentAndThumbnail(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts Options
		want bool
	}{
		{"extension", Options{Extensions: []string{".PDF"}}, true},
		{"wrong extension", Options{Extensions: []string{"mp4"}}, false},
		{"mime wildcard", Options{MIMETypes: []string{"application/*"}}, true},
		{"wrong mime", Options{MIMETypes: []string{"video/*"}}, false},
		{"min size", Options{MinFileSize: 5}, false},
		{"inclusive bounds", Options{MinFileSize: 4, MaxFileSize: 4}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			s := &fakeSource{pageSize: 2, items: []Item{{Message: Message{ID: 1, Type: "message", Text: "PDF caption", FileName: "REPORT.PDF", MIME: "application/pdf", FileSize: 4}, Files: []File{{Kind: "documents", Field: "file", Path: "files/report.pdf", Size: 4}, {Kind: "documents", Field: "thumbnail", Path: "files/thumb.jpg", Size: 4}}}}}
			opts := tc.opts
			opts.Media = []string{"documents"}
			opts.Concurrency = 2
			if _, err := Run(context.Background(), s, dir, Chat{ID: 1}, opts); err != nil {
				t.Fatal(err)
			}
			want := int32(0)
			if tc.want {
				want = 2
			}
			if s.downloads.Load() != want {
				t.Fatalf("downloads=%d, want %d", s.downloads.Load(), want)
			}
			b, _ := os.ReadFile(filepath.Join(dir, "result.json"))
			var data struct{ Messages []Message }
			if err := json.Unmarshal(b, &data); err != nil {
				t.Fatal(err)
			}
			if len(data.Messages) != 1 {
				t.Fatal("file filters lost message metadata")
			}
		})
	}
}

func TestOnlyMediaFiltersMessages(t *testing.T) {
	dir := t.TempDir()
	s := &fakeSource{pageSize: 4, items: fakeItems(3, 2, 1)}
	s.items[0].Files = nil
	s.items[1].Files[0].Path = "photos/2.jpg"
	s.items[2].Files[0].Path = "photos/1.png"
	_, err := Run(context.Background(), s, dir, Chat{ID: 1}, Options{Concurrency: 1, Media: []string{"photos"}, Extensions: []string{"jpg"}, OnlyMedia: true})
	if err != nil {
		t.Fatal(err)
	}
	ids := exportedIDs(t, dir)
	if len(ids) != 1 || ids[0] != 2 {
		t.Fatal(ids)
	}
}

func TestFilterValidation(t *testing.T) {
	for _, opts := range []Options{{MinFileSize: -1}, {MinFileSize: 10, MaxFileSize: 5}, {Extensions: []string{"../pdf"}}, {MIMETypes: []string{"bad"}}} {
		opts.Concurrency = 1
		if err := opts.Validate(); err == nil {
			t.Fatalf("accepted %+v", opts)
		}
	}
}
