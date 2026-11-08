package archive

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestPlanningReportsScannedMessagesWithoutMedia(t *testing.T) {
	source := &fakeSource{items: fakeItems(3, 2, 1), pageSize: 2}
	for i := range source.items {
		source.items[i].Files = nil
	}
	chats := []Chat{{ID: 1, Key: "user_1", Name: "Fadi"}, {ID: 2, Key: "user_2", Name: "Other"}}
	var updates []DownloadPlan
	_, err := RunBatch(context.Background(), source, t.TempDir(), chats, Options{
		Concurrency: 1, OnPlan: func(p DownloadPlan) { updates = append(updates, p) },
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var first, second bool
	for _, p := range updates {
		if p.Ready {
			continue
		}
		first = first || (p.Chat == "Fadi" && p.Messages == 2)
		second = second || (p.Chat == "Other" && p.Messages == 5)
	}
	last := updates[len(updates)-1]
	if !first || !second || !last.Ready || last.Messages != 6 || last.Files != 0 || last.Bytes != 0 {
		t.Fatalf("missing scan progress when no media was found: %+v", updates)
	}
}

type timedOutHistorySource struct {
	fakeSource
	calls int
}

func (s *timedOutHistorySource) History(context.Context, Chat, string) (Page, error) {
	s.calls++
	return Page{}, os.ErrDeadlineExceeded
}

func TestPlanningReturnsHistoryTimeoutForSessionRecovery(t *testing.T) {
	for _, resume := range []bool{false, true} {
		source := &timedOutHistorySource{}
		_, err := RunBatch(context.Background(), source, t.TempDir(), []Chat{{ID: 1}, {ID: 2}}, Options{
			Concurrency: 1, Resume: resume, OnPlan: func(DownloadPlan) {},
		}, nil)
		want := 2
		if resume {
			want = 1
		}
		if !errors.Is(err, os.ErrDeadlineExceeded) || source.calls != want || source.downloads.Load() != 0 {
			t.Fatalf("resume=%v err=%v history calls=%d downloads=%d", resume, err, source.calls, source.downloads.Load())
		}
	}
}

func TestBatchPlansWholeBackupBeforeDownloading(t *testing.T) {
	root := t.TempDir()
	source := &fakeSource{items: fakeItems(3, 2, 1), pageSize: 2}
	// A shared attachment must count once per chat, including across pages.
	source.items[2].Files = source.items[0].Files
	chats := []Chat{{ID: 1, Key: "user_1"}, {ID: 2, Key: "user_2"}}
	var plan DownloadPlan
	ready := false
	var transferred int64
	opts := Options{Concurrency: 1, Resume: true, Media: []string{"photos"}, OnPlan: func(p DownloadPlan) {
		if source.downloads.Load() != 0 {
			t.Fatal("planning continued after downloads started")
		}
		plan, ready = p, p.Ready
	}, OnTransfer: func(event TransferEvent) {
		if !ready {
			t.Error("download started before the whole backup was planned")
		}
		if event.Kind == TransferProgressed {
			transferred += event.Delta
		}
	}}
	if _, err := RunBatch(context.Background(), source, root, chats, opts, nil); err != nil {
		t.Fatal(err)
	}
	if !plan.Ready || plan.Bytes != 16 || plan.Files != 4 || transferred != 16 {
		t.Fatalf("plan=%+v transferred=%d", plan, transferred)
	}
	// Incremental planning excludes both old history and a completed file from
	// an interrupted page, but includes an incomplete local file in full.
	source.items = fakeItems(5, 4, 3, 2, 1)
	for _, chat := range chats {
		dir := filepath.Join(root, "ChatExport_"+chat.Key, "photos")
		if err := os.WriteFile(filepath.Join(dir, "5.jpg"), []byte("file"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "4.jpg"), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	before := source.downloads.Load()
	opts.OnPlan = func(p DownloadPlan) { plan = p }
	if _, err := RunBatch(context.Background(), source, root, chats, opts, nil); err != nil {
		t.Fatal(err)
	}
	if plan.Bytes != 8 || plan.Files != 2 || source.downloads.Load()-before != 2 {
		t.Fatalf("incremental plan=%+v downloads=%d", plan, source.downloads.Load()-before)
	}
}

func TestBatchPlanningPreservesIndependentChatFailures(t *testing.T) {
	source := &batchSource{fakeSource: fakeSource{items: fakeItems(1), pageSize: 2}}
	var plan DownloadPlan
	report, err := RunBatch(context.Background(), source, t.TempDir(), []Chat{{ID: 1, Key: "user_1"}, {ID: 2, Key: "user_2"}}, Options{
		Concurrency: 1, Media: []string{"photos"}, OnPlan: func(p DownloadPlan) { plan = p },
	}, nil)
	if err == nil || len(report.Chats) != 2 || report.Chats[0].Error == "" || report.Chats[1].Error != "" {
		t.Fatalf("report=%+v err=%v", report, err)
	}
	if !plan.Ready || plan.Bytes != 4 || plan.Files != 1 {
		t.Fatalf("plan=%+v", plan)
	}
}

func TestBatchPlanningCancellationDoesNotDownload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	source := &fakeSource{items: fakeItems(2, 1), pageSize: 1}
	_, err := RunBatch(ctx, source, t.TempDir(), []Chat{{ID: 1}}, Options{
		Concurrency: 1, Media: []string{"photos"}, OnPlan: func(DownloadPlan) { cancel() },
	}, nil)
	if !errors.Is(err, context.Canceled) || source.downloads.Load() != 0 {
		t.Fatalf("err=%v downloads=%d", err, source.downloads.Load())
	}
}

func TestPlanningSnapshotLeavesNewMessagesForNextSync(t *testing.T) {
	source := &fakeSource{items: fakeItems(3, 2, 1), pageSize: 2}
	root := t.TempDir()
	chats := []Chat{{ID: 1, Key: "user_1"}}
	opts := Options{Concurrency: 1, Resume: true, Media: []string{"photos"}, OnPlan: func(plan DownloadPlan) {
		if plan.Ready {
			source.items = fakeItems(4, 3, 2, 1)
		}
	}}
	report, err := RunBatch(context.Background(), source, root, chats, opts, nil)
	if err != nil || report.Chats[0].Stats.Messages != 3 {
		t.Fatalf("snapshot report=%+v err=%v", report, err)
	}
	report, err = RunBatch(context.Background(), source, root, chats, opts, nil)
	if err != nil || report.Chats[0].Stats.Messages != 4 || source.downloads.Load() != 4 {
		t.Fatalf("next sync report=%+v err=%v downloads=%d", report, err, source.downloads.Load())
	}
}

func TestPlanningUsesDownloadFiltersAndCountsGeneratedFiles(t *testing.T) {
	source := &fakeSource{items: fakeItems(3, 2, 1), pageSize: 2}
	source.items[0].Files = []File{{Kind: "documents", Field: "file", Path: "files/document.pdf", Size: 4}, {Kind: "documents", Field: "file", Path: "files/large.pdf", Size: 10}}
	source.items[1].Files = []File{{Kind: "contacts", Field: "contact_vcard", Path: "contacts/person.vcf", Data: []byte("card")}}
	var plan DownloadPlan
	opts := Options{Concurrency: 1, Resume: true, Media: []string{"documents", "contacts"}, MaxFileSize: 5, OnlyMedia: true, OnPlan: func(p DownloadPlan) { plan = p }}
	root := t.TempDir()
	chats := []Chat{{ID: 1, Key: "user_1"}}
	for _, refresh := range []bool{false, true} {
		opts.Refresh = refresh
		if _, err := RunBatch(context.Background(), source, root, chats, opts, nil); err != nil {
			t.Fatal(err)
		}
		want := int64(8)
		if refresh {
			want = 0
		}
		if !plan.Ready || plan.Bytes != want || plan.Files != want/4 {
			t.Fatalf("refresh=%v plan=%+v", refresh, plan)
		}
	}
}

type chunkProgressSource struct{ fakeSource }

func (s *chunkProgressSource) DownloadWithProgress(ctx context.Context, file File, dest string, progress func(int64)) error {
	if err := s.Download(ctx, file, dest); err != nil {
		return err
	}
	var chunks sync.WaitGroup
	for range 2 {
		chunks.Go(func() { progress(2) })
	}
	chunks.Wait()
	return nil
}

func TestProgressReportsParallelChunksOnceAndIntegrityFailures(t *testing.T) {
	source := &chunkProgressSource{fakeSource: fakeSource{items: fakeItems(2, 1), pageSize: 2}}
	var mu sync.Mutex
	var transferred int64
	var finished []TransferEvent
	opts := Options{Concurrency: 2, Media: []string{"photos"}, OnTransfer: func(event TransferEvent) {
		mu.Lock()
		defer mu.Unlock()
		if event.Kind == TransferProgressed {
			transferred += event.Delta
		}
		if event.Kind == TransferFinished {
			finished = append(finished, event)
		}
	}}
	if _, err := Run(context.Background(), source, t.TempDir(), Chat{ID: 1}, opts); err != nil {
		t.Fatal(err)
	}
	if transferred != 8 || len(finished) != 2 || finished[0].Err != nil || finished[1].Err != nil {
		t.Fatalf("bytes=%d finished=%+v", transferred, finished)
	}
	source.items = fakeItems(1)
	source.items[0].Files[0].Size = 5
	finished = nil
	if _, err := Run(context.Background(), source, t.TempDir(), Chat{ID: 1}, opts); err == nil || len(finished) != 1 || finished[0].Err == nil {
		t.Fatalf("integrity failure lost: err=%v finished=%+v", err, finished)
	}
}
