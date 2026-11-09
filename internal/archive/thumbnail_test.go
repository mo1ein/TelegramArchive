package archive

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type thumbnailFailureSource struct {
	fakeSource
	failure  error
	field    string
	cancel   context.CancelFunc
	failures int
}

func (s *thumbnailFailureSource) Download(ctx context.Context, file File, path string) error {
	if file.Field == s.field && s.failure != nil {
		s.failures++
		if s.cancel != nil {
			s.cancel()
		}
		return s.failure
	}
	return s.fakeSource.Download(ctx, file, path)
}
func thumbnailSource() *thumbnailFailureSource {
	s := &thumbnailFailureSource{fakeSource: fakeSource{items: fakeItems(2, 1), pageSize: 2}, field: "thumbnail"}
	s.items[0].Files = append(s.items[0].Files, File{Kind: "photos", Field: "thumbnail", Path: "photos/2_thumb.jpg", Size: 4})
	return s
}
func TestUnavailableThumbnailKeepsMediaAndCanBeRefreshed(t *testing.T) {
	source := thumbnailSource()
	source.failure = ErrThumbnailUnavailable
	root := t.TempDir()
	chats := []Chat{{ID: 1, Key: "user_1"}}
	opts := Options{Concurrency: 1, Resume: true, Media: []string{"photos"}, Format: "both"}
	report, err := RunBatch(context.Background(), source, root, chats, opts, nil)
	if err != nil {
		t.Fatalf("optional thumbnail stopped backup: %v", err)
	}
	if len(report.Warnings) != 1 || !strings.Contains(report.Warnings[0], "thumbnail") {
		t.Fatalf("missing unavailable thumbnail warning: %+v", report)
	}
	check := func(wantUnavailable bool) {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(root, "ChatExport_user_1/result.json"))
		if err != nil {
			t.Fatal(err)
		}
		var out struct{ Messages []Message }
		if err = json.Unmarshal(b, &out); err != nil {
			t.Fatal(err)
		}
		if len(out.Messages) != 2 {
			t.Fatalf("lost messages: %+v", out)
		}
		for _, m := range out.Messages {
			if _, err := os.Stat(filepath.Join(root, "ChatExport_user_1", m.Photo)); err != nil {
				t.Fatalf("primary media missing: %v", err)
			}
			if m.ID == 2 {
				if wantUnavailable && m.Thumbnail != "(File unavailable, please try again later)" {
					t.Fatalf("missing thumbnail was hidden: thumbnail=%q reason=%q", m.Thumbnail, m.ThumbnailSkipReason)
				}
				if !wantUnavailable && (m.Thumbnail != "photos/2_thumb.jpg" || m.ThumbnailSkipReason != "") {
					t.Fatalf("refresh did not restore thumbnail: %+v", m)
				}
			}
		}
		metadata, err := os.ReadFile(filepath.Join(root, "ChatExport_user_1/archive-metadata.json"))
		if err != nil {
			t.Fatal(err)
		}
		var details struct{ Messages []Message }
		if err = json.Unmarshal(metadata, &details); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, m := range details.Messages {
			if m.ID == 2 && m.ThumbnailSkipReason == "unavailable" {
				found = true
			}
		}
		if found != wantUnavailable {
			t.Fatalf("unavailable thumbnail metadata: %s", metadata)
		}
	}
	check(true)
	source.failure = nil
	report, err = RunBatch(context.Background(), source, root, chats, opts, nil)
	if err != nil || report.Summary.StoredUnavailableThumbnails != 1 || len(report.Warnings) == 0 || report.Summary.Totals().Unavailable != 0 {
		t.Fatalf("incremental run hid an existing unavailable thumbnail: %+v err=%v", report, err)
	}
	opts.Refresh = true
	if report, err = RunBatch(context.Background(), source, root, chats, opts, nil); err != nil {
		t.Fatal(err)
	}
	if report.Summary.StoredUnavailableThumbnails != 0 {
		t.Fatalf("refreshed thumbnail still reported unavailable: %+v", report.Summary)
	}
	check(false)
}
func TestThumbnailFailureHandlingPreservesFatalErrors(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		failure     error
		cancel      bool
	}{
		{"primary attachment", "photo", ErrThumbnailUnavailable, false},
		{"local filesystem", "thumbnail", &os.PathError{Op: "write", Path: "thumb.jpg", Err: os.ErrPermission}, false},
		{"local close after server failure", "thumbnail", errors.Join(ErrThumbnailUnavailable, &os.PathError{Op: "close", Path: "thumb.jpg", Err: os.ErrPermission}), false},
		{"canceled thumbnail transfer", "thumbnail", errors.Join(ErrThumbnailUnavailable, context.Canceled), false},
		{"parent cancellation", "thumbnail", ErrThumbnailUnavailable, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			source := thumbnailSource()
			source.failure = tc.failure
			source.field = tc.field
			if tc.cancel {
				source.cancel = cancel
			}
			_, err := RunBatch(ctx, source, t.TempDir(), []Chat{{ID: 1}}, Options{Concurrency: 1, Media: []string{"photos"}}, nil)
			if err == nil || (!errors.Is(err, tc.failure) && !errors.Is(err, context.Canceled)) {
				t.Fatalf("fatal error was hidden: %v", err)
			}
		})
	}
}

func TestUnavailableThumbnailIsCountedOnceDuringRefresh(t *testing.T) {
	source := thumbnailSource()
	root := t.TempDir()
	chats := []Chat{{ID: 1}}
	opts := Options{Concurrency: 1, Resume: true, Media: []string{"photos"}}
	if _, err := RunBatch(context.Background(), source, root, chats, opts, nil); err != nil {
		t.Fatal(err)
	}
	newest := fakeItems(3)[0]
	newest.Files = append(newest.Files, File{Kind: "photos", Field: "thumbnail", Path: "photos/3_thumb.jpg", Size: 4})
	source.items = append([]Item{newest}, source.items...)
	source.failure = ErrThumbnailUnavailable
	opts.Refresh = true
	report, err := RunBatch(context.Background(), source, root, chats, opts, nil)
	if err != nil || source.failures != 1 || len(report.Warnings) != 1 || !strings.Contains(report.Warnings[0], "1 thumbnail") {
		t.Fatalf("refresh retried or double-counted a known unavailable thumbnail: failures=%d warnings=%v err=%v", source.failures, report.Warnings, err)
	}
}
