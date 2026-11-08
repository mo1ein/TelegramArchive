package archive

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnchangedSyncReusesOutputsAndRepairsMissingPages(t *testing.T) {
	ctx, dir := context.Background(), t.TempDir()
	source := &fakeSource{items: fakeItems(3, 2, 1), pageSize: 2}
	opts := Options{Concurrency: 1, Resume: true, Format: "both", HTMLPageSize: 2}
	chat := Chat{ID: 1, Name: "Team"}
	first, err := Run(ctx, source, dir, chat, opts)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "result.json")
	before, _ := os.Stat(output)
	second, err := Run(ctx, source, dir, chat, opts)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(output)
	if !before.ModTime().Equal(after.ModTime()) || second.Stats.Messages != first.Stats.Messages {
		t.Fatal("unchanged sync rewrote full history or lost statistics")
	}
	os.Remove(filepath.Join(dir, "messages2.html"))
	if _, err := Run(ctx, source, dir, chat, opts); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "messages2.html")); err != nil {
		t.Fatal("missing page was not repaired", err)
	}
	chat.Name = "Renamed team"
	if _, err := Run(ctx, source, dir, chat, opts); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(output)
	if !strings.Contains(string(b), "Renamed team") {
		t.Fatal("metadata change reused stale output")
	}
}

type unavailableHistory struct{ *fakeSource }

func (unavailableHistory) History(context.Context, Chat, string) (Page, error) {
	return Page{}, errors.New("offline")
}

func TestMetadataChangeInvalidatesCacheEvenWhenSyncFails(t *testing.T) {
	ctx, dir := context.Background(), t.TempDir()
	s := &fakeSource{items: fakeItems(1), pageSize: 2}
	opts := Options{Concurrency: 1, Resume: true}
	chat := Chat{ID: 1, Name: "Original"}
	if _, err := Run(ctx, s, dir, chat, opts); err != nil {
		t.Fatal(err)
	}
	chat.Name = "Renamed"
	if _, err := Run(ctx, unavailableHistory{s}, dir, chat, opts); err == nil {
		t.Fatal("expected interrupted sync")
	}
	if _, err := Run(ctx, s, dir, chat, opts); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "result.json"))
	if !strings.Contains(string(b), "Renamed") {
		t.Fatal("interrupted metadata update reused stale output")
	}
}

func BenchmarkUnchangedSync(b *testing.B) {
	for _, count := range []int{100, 10000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			ids := make([]int, count)
			for i := range ids {
				ids[i] = count - i
			}
			s := &fakeSource{items: fakeItems(ids...), pageSize: 100}
			dir := b.TempDir()
			opts := Options{Concurrency: 1, Resume: true}
			ctx, chat := context.Background(), Chat{ID: 1}
			if _, err := Run(ctx, s, dir, chat, opts); err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := Run(ctx, s, dir, chat, opts); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
