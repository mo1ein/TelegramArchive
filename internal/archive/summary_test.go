package archive

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSummaryRecorderDeduplicatesAndResolvesRetries(t *testing.T) {
	r, err := NewSummaryRecorder(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	photo := File{Kind: "photos", Field: "photo", Path: "photos/1.jpg", Size: 4}
	r.Observe("chat", TransferEvent{Kind: TransferFinished, File: photo, Bytes: 4})
	r.Observe("chat", TransferEvent{Kind: TransferReused, File: photo})
	document := File{Kind: "documents", Field: "file", Path: "files/2.pdf", Size: 8}
	r.Observe("chat", TransferEvent{Kind: TransferFinished, File: document, Err: errors.New("connection dropped")})
	r.Observe("chat", TransferEvent{Kind: TransferFinished, File: document, Bytes: 8})
	thumb := File{Kind: "videos", Field: "thumbnail", Path: "video_files/2_thumb.jpg"}
	for range 2 {
		r.Observe("chat", TransferEvent{Kind: TransferFinished, File: thumb, Err: ErrThumbnailUnavailable})
	}
	filtered := File{Kind: "audios", Path: "audio_files/excluded.mp3"}
	for range 2 {
		r.Observe("chat", TransferEvent{Kind: TransferFiltered, File: filtered})
	}
	r.Observe("chat", TransferEvent{Kind: TransferReused, File: File{Kind: "videos", Path: "video_files/old.mp4"}})
	r.Observe("chat", TransferEvent{Kind: TransferFinished, File: File{Kind: "photos", Path: "photos/canceled.jpg"}, Err: context.Canceled})
	r.AddMessages("chat", 2)
	s := r.Snapshot("chat")
	totals := s.Totals()
	if !s.MediaKnown || totals.Downloaded != 2 || totals.Reused != 1 || totals.Filtered != 1 || totals.Unavailable != 1 || totals.Canceled != 1 || totals.Failed != 0 || s.DownloadedBytes != 12 || s.MessagesAdded != 2 {
		t.Fatalf("incorrect final outcomes after replay/retry: %+v totals=%+v", s, totals)
	}
	if s.Media["thumbnails"].Unavailable != 1 || s.Media["videos"].Reused != 1 {
		t.Fatalf("thumbnail mixed into video totals: %+v", s.Media)
	}
	r.Observe("other", TransferEvent{Kind: TransferFinished, File: photo, Bytes: 4})
	if got := r.Snapshot("other").Totals().Downloaded; got != 1 {
		t.Fatalf("chat scopes collided: %d", got)
	}
}
func TestSummaryRecorderHandlesConcurrentFiles(t *testing.T) {
	r, err := NewSummaryRecorder(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var workers sync.WaitGroup
	for i := range 40 {
		workers.Go(func() {
			file := File{Kind: "photos", Path: fmt.Sprintf("photos/%d.jpg", i)}
			r.Observe("chat", TransferEvent{Kind: TransferFinished, File: file, Bytes: 4})
		})
	}
	workers.Wait()
	s := r.Snapshot("chat")
	if !s.MediaKnown || s.Totals().Downloaded != 40 || s.DownloadedBytes != 160 {
		t.Fatalf("lost concurrent outcomes: %+v", s)
	}
}
func TestSummaryTextMakesCompletionAndUnavailableCountsVisible(t *testing.T) {
	s := Summary{HasMessages: true, MessagesKnown: true, Messages: 120, MediaKnown: true, Media: map[string]MediaCounts{"audios": {Downloaded: 12, Reused: 8, Filtered: 2}, "thumbnails": {Unavailable: 1}}, Filters: []string{"media=all"}}
	text := SummaryText("Finished archive \"Example Channel\"", "complete_with_warnings", s, time.Now(), time.Minute)
	for _, want := range []string{"Finished archive", "completed with warnings", "120", "Downloaded", "Reused", "Filtered", "Failed", "Unavailable", "Audio", "Thumbnail", "media=all"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in summary:\n%s", want, text)
		}
	}
	text = SummaryText("Finished archive test\n\x1b[31m", "failed", Summary{Error: "summary storage unavailable"}, time.Now(), time.Second)
	if strings.Contains(text, "\x1b") || strings.Contains(text, "Downloaded") || !strings.Contains(text, "unavailable") || strings.Contains(text, "completed") {
		t.Fatalf("unknown summary reported success or invented counters: %q", text)
	}
}

func TestArchiveSummaryCountsFilteredReusedAndDownloadedFiles(t *testing.T) {
	root := t.TempDir()
	source := &fakeSource{items: fakeItems(3, 2, 1), pageSize: 2}
	source.items[1].Files[0].Kind = "documents"
	folder := filepath.Join(root, "ChatExport_user_1", "photos")
	if err := os.MkdirAll(folder, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "3.jpg"), []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	var finished []ChatExport
	report, err := RunBatch(context.Background(), source, root, []Chat{{ID: 1, Key: "user_1"}}, Options{Concurrency: 1, Media: []string{"photos"}, Resume: true, OnChatFinished: func(c ChatExport) { finished = append(finished, c) }}, nil)
	if err != nil {
		t.Fatal(err)
	}
	total := report.Summary.Totals()
	if len(finished) != 1 || finished[0].Finished.IsZero() || !report.Summary.MessagesKnown || report.Summary.Messages != 3 || report.Summary.MessagesAdded != 3 || total.Downloaded != 1 || total.Reused != 1 || total.Filtered != 1 || report.Summary.DownloadedBytes != 4 {
		t.Fatalf("incorrect archive summary: %+v totals=%+v callbacks=%+v", report.Summary, total, finished)
	}
}

func TestArchiveSummaryIncludesFailuresBeforeNetworkStarts(t *testing.T) {
	root := t.TempDir()
	source := &fakeSource{items: fakeItems(1), pageSize: 2}
	if err := os.MkdirAll(filepath.Join(root, "ChatExport_user_1"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ChatExport_user_1", "photos"), []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	var ended ChatExport
	report, err := RunBatch(context.Background(), source, root, []Chat{{ID: 1, Key: "user_1"}}, Options{Concurrency: 1, Media: []string{"photos"}, OnChatFinished: func(c ChatExport) { ended = c }}, nil)
	if err == nil || ended.Error == "" || report.Summary.Totals().Failed != 1 || report.Summary.MessagesKnown {
		t.Fatalf("failure summary invented completion: summary=%+v ended=%+v err=%v", report.Summary, ended, err)
	}
}
