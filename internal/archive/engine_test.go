package archive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeSource struct {
	items        []Item
	pageSize     int
	failOffset   int
	failDownload bool
	downloads    atomic.Int32
	active       atomic.Int32
	peak         atomic.Int32
	gate         chan struct{}
	gateOnce     sync.Once
}

func (s *fakeSource) History(ctx context.Context, chat Chat, cursor string) (Page, error) {
	offset, _ := strconv.Atoi(cursor)
	if s.failOffset > 0 && offset == s.failOffset {
		return Page{}, errors.New("connection lost")
	}
	var page Page
	for _, item := range s.items {
		if offset == 0 || item.Message.ID < offset {
			page.Items = append(page.Items, item)
			if len(page.Items) == s.pageSize {
				break
			}
		}
	}
	page.Done = len(page.Items) < s.pageSize
	if len(page.Items) > 0 {
		page.Next = strconv.Itoa(page.Items[len(page.Items)-1].Message.ID)
	}
	return page, nil
}

func (s *fakeSource) Download(ctx context.Context, file File, path string) error {
	s.downloads.Add(1)
	n := s.active.Add(1)
	defer s.active.Add(-1)
	for old := s.peak.Load(); n > old; old = s.peak.Load() {
		if s.peak.CompareAndSwap(old, n) {
			break
		}
	}
	if s.gate != nil {
		if n == 2 {
			s.gateOnce.Do(func() { close(s.gate) })
		}
		select {
		case <-s.gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if s.failDownload {
		return errors.New("download failed")
	}
	return os.WriteFile(path, []byte("file"), 0600)
}

func fakeItems(ids ...int) []Item {
	var items []Item
	for _, id := range ids {
		items = append(items, Item{Message: Message{ID: id, Type: "message", Text: fmt.Sprint(id)}, Files: []File{{Kind: "photos", Field: "photo", Path: fmt.Sprintf("photos/%d.jpg", id), Size: 4}}})
	}
	return items
}

func exportedIDs(t *testing.T, dir string) []int {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	var data struct{ Messages []Message }
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatal(err)
	}
	var ids []int
	for _, m := range data.Messages {
		ids = append(ids, m.ID)
	}
	return ids
}

func TestResumeThenIncrementalArchive(t *testing.T) {
	dir := t.TempDir()
	s := &fakeSource{items: fakeItems(5, 4, 3, 2, 1), pageSize: 2, failOffset: 4}
	opts := Options{Media: []string{"photos"}, Concurrency: 2, Resume: true}
	chat := Chat{ID: 42, Name: "test", Type: "personal_chat"}
	if _, err := Run(context.Background(), s, dir, chat, opts); err == nil {
		t.Fatal("expected interruption")
	}
	if _, err := os.Stat(filepath.Join(dir, "result.json")); !os.IsNotExist(err) {
		t.Fatal("published incomplete archive")
	}
	s.failOffset = 0
	if _, err := Run(context.Background(), s, dir, chat, opts); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(exportedIDs(t, dir)); got != "[1 2 3 4 5]" {
		t.Fatal(got)
	}
	if s.downloads.Load() != 5 {
		t.Fatalf("re-downloaded cached files: %d", s.downloads.Load())
	}
	s.items = fakeItems(7, 6, 5, 4, 3, 2, 1)
	if _, err := Run(context.Background(), s, dir, chat, opts); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(exportedIDs(t, dir)); got != "[1 2 3 4 5 6 7]" {
		t.Fatal(got)
	}
	if s.downloads.Load() != 7 {
		t.Fatalf("incremental downloads: %d", s.downloads.Load())
	}
	opts.Media = []string{"documents"}
	if _, err := Run(context.Background(), s, dir, chat, opts); err == nil {
		t.Fatal("allowed changed resume settings")
	}
}

func TestBoundedParallelDownloads(t *testing.T) {
	s := &fakeSource{items: fakeItems(4, 3, 2, 1), pageSize: 4, gate: make(chan struct{})}
	_, err := Run(context.Background(), s, t.TempDir(), Chat{ID: 1}, Options{Media: []string{"photos"}, Concurrency: 2})
	if err != nil {
		t.Fatal(err)
	}
	if s.peak.Load() != 2 {
		t.Fatalf("concurrency: %d", s.peak.Load())
	}
}

func TestDownloadFailureDoesNotAdvanceCheckpoint(t *testing.T) {
	dir := t.TempDir()
	s := &fakeSource{items: fakeItems(2, 1), pageSize: 2, failDownload: true}
	opts := Options{Media: []string{"photos"}, Concurrency: 1, Resume: true}
	if _, err := Run(context.Background(), s, dir, Chat{ID: 1}, opts); err == nil {
		t.Fatal("download failure hidden")
	}
	s.failDownload = false
	if _, err := Run(context.Background(), s, dir, Chat{ID: 1}, opts); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(exportedIDs(t, dir)); got != "[1 2]" {
		t.Fatal(got)
	}
}

func TestMediaSelectionSizeAndUnsafePaths(t *testing.T) {
	for _, tc := range []struct {
		name    string
		opts    Options
		path    string
		want    string
		wantErr bool
	}{
		{"excluded", Options{Concurrency: 1}, "photos/1.jpg", Excluded, false},
		{"too large", Options{Concurrency: 1, Media: []string{"photos"}, MaxFileSize: 3}, "photos/1.jpg", TooLarge, false},
		{"unsafe", Options{Concurrency: 1, Media: []string{"photos"}}, "../escape", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			s := &fakeSource{items: fakeItems(1), pageSize: 2}
			s.items[0].Files[0].Path = tc.path
			_, err := Run(context.Background(), s, dir, Chat{ID: 1}, tc.opts)
			if tc.wantErr {
				if err == nil {
					t.Fatal("accepted unsafe path")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			b, _ := os.ReadFile(filepath.Join(dir, "result.json"))
			var data struct{ Messages []Message }
			json.Unmarshal(b, &data)
			if data.Messages[0].Photo != tc.want || s.downloads.Load() != 0 {
				t.Fatalf("unexpected media: %+v", data)
			}
		})
	}
}

func TestEmptyChatCanReceiveNewMessages(t *testing.T) {
	dir := t.TempDir()
	s := &fakeSource{pageSize: 2}
	opts := Options{Concurrency: 1, Resume: true}
	if _, err := Run(context.Background(), s, dir, Chat{ID: 1}, opts); err != nil {
		t.Fatal(err)
	}
	s.items = fakeItems(1)
	if _, err := Run(context.Background(), s, dir, Chat{ID: 1}, opts); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(exportedIDs(t, dir)); got != "[1]" {
		t.Fatal(got)
	}
}

func TestCancellationAndAccountMismatch(t *testing.T) {
	dir := t.TempDir()
	s := &fakeSource{items: fakeItems(1), pageSize: 2}
	opts := Options{Concurrency: 1, Resume: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Run(ctx, s, dir, Chat{ID: 1}, opts); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), s, dir, Chat{ID: 1, AccountID: 7}, opts); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), s, dir, Chat{ID: 1, AccountID: 8}, opts); err == nil {
		t.Fatal("mixed accounts")
	}
	opts.Resume = false
	if _, err := Run(context.Background(), s, dir, Chat{ID: 1, AccountID: 8}, opts); err == nil {
		t.Fatal("rebuild reused another account's media")
	}
}

func TestInterruptedIncrementalCatchupDoesNotSkipGap(t *testing.T) {
	dir := t.TempDir()
	s := &fakeSource{items: fakeItems(3, 2, 1), pageSize: 2}
	opts := Options{Concurrency: 2, Media: []string{"photos"}, Resume: true}
	if _, err := Run(context.Background(), s, dir, Chat{ID: 1}, opts); err != nil {
		t.Fatal(err)
	}
	s.items = fakeItems(8, 7, 6, 5, 4, 3, 2, 1)
	s.failOffset = 7
	if _, err := Run(context.Background(), s, dir, Chat{ID: 1}, opts); err == nil {
		t.Fatal("expected catch-up interruption")
	}
	s.failOffset = 0
	if _, err := Run(context.Background(), s, dir, Chat{ID: 1}, opts); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(exportedIDs(t, dir)); got != "[1 2 3 4 5 6 7 8]" {
		t.Fatal(got)
	}
}

func TestDateIntervalAndSizeVerification(t *testing.T) {
	dir := t.TempDir()
	s := &fakeSource{items: fakeItems(3, 2, 1), pageSize: 2}
	for i := range s.items {
		s.items[i].Message.DateUnix = strconv.Itoa(s.items[i].Message.ID * 100)
	}
	opts := Options{Concurrency: 1, Since: time.Unix(100, 0), Until: time.Unix(300, 0)}
	if _, err := Run(context.Background(), s, dir, Chat{ID: 1}, opts); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(exportedIDs(t, dir)); got != "[1 2]" {
		t.Fatal(got)
	}
	opts.Since = time.Time{}
	opts.Until = time.Time{}
	opts.Media = []string{"photos"}
	s.items[0].Files[0].Size = 99
	if _, err := Run(context.Background(), s, dir, Chat{ID: 1}, opts); err == nil {
		t.Fatal("accepted truncated download")
	}
}

func TestTelegramMetadataKeeps64BitIDs(t *testing.T) {
	dir := t.TempDir()
	s := &fakeSource{items: fakeItems(1), pageSize: 2}
	s.items[0].Message.ExtraMedia = map[string]int64{"document_id": 9223372036854775806}
	if _, err := Run(context.Background(), s, dir, Chat{ID: 1}, Options{Concurrency: 1}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "archive-metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte("9223372036854775806")) {
		t.Fatalf("64-bit metadata was rounded: %s", b)
	}
}
