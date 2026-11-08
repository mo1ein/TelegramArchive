package archive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func renderTestItems(ids ...int) []Item {
	items := fakeItems(ids...)
	for i := range items {
		m := &items[i].Message
		m.Date = fmt.Sprintf("2026-01-%02dT10:00:00", m.ID)
		m.FromID, m.From = fmt.Sprintf("user%d", m.ID%2), "Example"
		m.Text = []any{"hello ", TextEntity{Type: "bold", Text: "سلام دنیا"}}
		m.ReplyTo = max(0, m.ID-3)
		switch m.ID {
		case 2:
			m.Type, m.Action, m.Actor = "service", "pin_message", "Example"
		case 3:
			m.Edited = "2026-01-04T10:00:00"
		case 4:
			m.ForwardedFromID = "user9"
		case 5:
			m.Contact = &Contact{FirstName: "Contact", Phone: "123"}
		case 6:
			m.Location = &Location{Latitude: 35, Longitude: 51}
		case 7:
			m.Poll = map[string]any{"question": "Question?", "total_voters": 0, "answers": []any{}}
		case 8:
			m.PhotoSize, m.FileSize = 42, 123
		}
	}
	return items
}

func pageTimes(t *testing.T, dir string, count int) []time.Time {
	t.Helper()
	var times []time.Time
	for i := 1; i <= count; i++ {
		st, err := os.Stat(filepath.Join(dir, htmlPageName(i)))
		if err != nil {
			t.Fatal(err)
		}
		times = append(times, st.ModTime())
	}
	return times
}

func assertPagesRetained(t *testing.T, dir string, before []time.Time, pages ...int) {
	t.Helper()
	for _, page := range pages {
		st, err := os.Stat(filepath.Join(dir, htmlPageName(page)))
		if err != nil || !st.ModTime().Equal(before[page-1]) {
			t.Fatalf("unchanged page %d was rewritten: %v", page, err)
		}
	}
}

// Compare public artifacts to an independent full render of the same snapshot.
func assertFullRenderEquivalent(t *testing.T, dir string, opts Options, got Result) {
	t.Helper()
	full := t.TempDir()
	data, err := os.ReadFile(filepath.Join(dir, "archive.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(full, "archive.db"), data, 0600); err != nil {
		t.Fatal(err)
	}
	db, err := bolt.Open(filepath.Join(full, "archive.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	var directStats Stats
	err = db.Update(func(tx *bolt.Tx) error {
		// Reconstruct metadata from messages rather than trusting the index
		// produced by the incremental path under test.
		if err := tx.DeleteBucket(metadataRowsBucket); err != nil {
			return err
		}
		return tx.Bucket(messageBucket).ForEach(func(_, row []byte) error {
			m, err := decodeMessage(row)
			if err != nil {
				return err
			}
			directStats.Observe(m)
			return nil
		})
	})
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Stats, directStats) {
		t.Fatalf("page aggregates differ from message statistics:\ngot %+v\nwant %+v", got.Stats, directStats)
	}
	want, err := Render(context.Background(), full, opts.Format, opts.PartSize, opts.HTMLPageSize)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Stats, want.Stats) {
		t.Fatalf("incremental statistics differ:\ngot %+v\nwant %+v", got.Stats, want.Stats)
	}
	if len(got.Paths) != len(want.Paths) {
		t.Fatalf("output count differs: %v vs %v", got.Paths, want.Paths)
	}
	paths := append(append([]string(nil), got.Paths...), filepath.Join(dir, "archive-metadata.json"), filepath.Join(dir, "statistics.json"))
	for _, path := range paths {
		actual, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		expected, err := os.ReadFile(filepath.Join(full, filepath.Base(path)))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(actual, expected) {
			t.Fatalf("incremental %s differs from full render", filepath.Base(path))
		}
	}
}

func TestIncrementalRenderAppendsAndEdits(t *testing.T) {
	for _, format := range []string{"html", "both", "json"} {
		t.Run(format, func(t *testing.T) {
			ctx, dir := context.Background(), t.TempDir()
			chat := Chat{ID: 1, Name: "Team"}
			opts := Options{Concurrency: 1, Resume: true, Format: format, HTMLPageSize: 2, PartSize: 1200}
			s := &fakeSource{items: renderTestItems(6, 5, 4, 3, 2, 1), pageSize: 100}
			if _, err := Run(ctx, s, dir, chat, opts); err != nil {
				t.Fatal(err)
			}
			for _, newest := range []int{7, 8} {
				var before []time.Time
				if format != "json" {
					before = pageTimes(t, dir, newest/2)
				}
				s.items = append(renderTestItems(newest), s.items...)
				result, err := Run(ctx, s, dir, chat, opts)
				if err != nil {
					t.Fatal(err)
				}
				if format != "json" {
					assertPagesRetained(t, dir, before, 1, 2)
					if newest == 8 {
						assertPagesRetained(t, dir, before, 3)
					}
				}
				assertFullRenderEquivalent(t, dir, opts, result)
			}
			for _, withMetadata := range []bool{true, false} {
				var before []time.Time
				if format != "json" {
					before = pageTimes(t, dir, 4)
				}
				m := &s.items[len(s.items)-1].Message
				m.FromID, m.Date, m.Text = "changed_sender", "2030-01-01T12:00:00", "edited"
				if withMetadata {
					m.ExtraMedia = map[string]int64{"document_id": 9223372036854775806}
				} else {
					m.ExtraMedia = nil
				}
				opts.Refresh = true
				result, err := Run(ctx, s, dir, chat, opts)
				if err != nil {
					t.Fatal(err)
				}
				if format != "json" {
					assertPagesRetained(t, dir, before, 2, 3, 4)
				}
				assertFullRenderEquivalent(t, dir, opts, result)
			}
		})
	}
}

func TestIncrementalRenderRepairsOnlyDamagedPages(t *testing.T) {
	ctx, dir := context.Background(), t.TempDir()
	chat := Chat{ID: 1, Name: "Team"}
	opts := Options{Concurrency: 1, Resume: true, Format: "both", HTMLPageSize: 2}
	s := &fakeSource{items: renderTestItems(6, 5, 4, 3, 2, 1), pageSize: 100}
	if _, err := Run(ctx, s, dir, chat, opts); err != nil {
		t.Fatal(err)
	}
	before := pageTimes(t, dir, 3)
	if err := os.Remove(filepath.Join(dir, "messages2.html")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "messages3.html"), []byte("damaged"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := Run(ctx, s, dir, chat, opts)
	if err != nil {
		t.Fatal(err)
	}
	assertPagesRetained(t, dir, before, 1)
	assertFullRenderEquivalent(t, dir, opts, result)
}

func TestIncrementalRenderReflowsInsertedHistoryAndPageSize(t *testing.T) {
	ctx, dir := context.Background(), t.TempDir()
	chat := Chat{ID: 1, Name: "Team"}
	opts := Options{Concurrency: 1, Resume: true, Format: "both", HTMLPageSize: 2}
	s := &fakeSource{items: renderTestItems(8, 6, 4, 2), pageSize: 100}
	if _, err := Run(ctx, s, dir, chat, opts); err != nil {
		t.Fatal(err)
	}
	s.items, opts.Refresh = renderTestItems(8, 6, 4, 3, 2), true
	result, err := Run(ctx, s, dir, chat, opts)
	if err != nil {
		t.Fatal(err)
	}
	assertFullRenderEquivalent(t, dir, opts, result)
	opts.HTMLPageSize = 4
	result, err = Run(ctx, s, dir, chat, opts)
	if err != nil {
		t.Fatal(err)
	}
	assertFullRenderEquivalent(t, dir, opts, result)
	if _, err := os.Stat(filepath.Join(dir, "messages3.html")); !os.IsNotExist(err) {
		t.Fatal("obsolete page was retained")
	}
}

func TestIncrementalRenderKeepsChangesAfterInterruptedRefresh(t *testing.T) {
	ctx, dir := context.Background(), t.TempDir()
	chat := Chat{ID: 1, Name: "Team"}
	opts := Options{Concurrency: 1, Resume: true, Format: "html", HTMLPageSize: 2}
	s := &fakeSource{items: renderTestItems(8, 7, 6, 5, 4, 3, 2, 1), pageSize: 2}
	if _, err := Run(ctx, s, dir, chat, opts); err != nil {
		t.Fatal(err)
	}
	before := pageTimes(t, dir, 4)
	s.items[1].Message.Text, s.failOffset, opts.Refresh = "edited before interruption", 7, true
	if _, err := Run(ctx, s, dir, chat, opts); err == nil {
		t.Fatal("expected interrupted refresh")
	}
	assertPagesRetained(t, dir, before, 1, 2, 3, 4)
	s.failOffset = 0
	result, err := Run(ctx, s, dir, chat, opts)
	if err != nil {
		t.Fatal(err)
	}
	assertPagesRetained(t, dir, before, 1, 2, 3)
	assertFullRenderEquivalent(t, dir, opts, result)
	db, err := bolt.Open(filepath.Join(dir, "archive.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.View(func(tx *bolt.Tx) error {
		if tx.Bucket(renderChangesBucket).Stats().KeyN != 0 {
			t.Fatal("published changes were not cleared")
		}
		var cache renderCache
		return json.Unmarshal(tx.Bucket(metaBucket).Get(renderCacheKey), &cache)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestIncrementalRenderRetriesFailedPublication(t *testing.T) {
	ctx, dir := context.Background(), t.TempDir()
	chat := Chat{ID: 1, Name: "Team"}
	opts := Options{Concurrency: 1, Resume: true, Format: "html", HTMLPageSize: 2}
	s := &fakeSource{items: renderTestItems(4, 3, 2, 1), pageSize: 100}
	if _, err := Run(ctx, s, dir, chat, opts); err != nil {
		t.Fatal(err)
	}
	statsPath := filepath.Join(dir, "statistics.json")
	if err := os.Remove(statsPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(statsPath, 0700); err != nil {
		t.Fatal(err)
	}
	s.items = append(renderTestItems(5), s.items...)
	if _, err := Run(ctx, s, dir, chat, opts); err == nil {
		t.Fatal("expected publication failure")
	}
	if err := os.Remove(statsPath); err != nil {
		t.Fatal(err)
	}
	result, err := Run(ctx, s, dir, chat, opts)
	if err != nil {
		t.Fatal(err)
	}
	assertFullRenderEquivalent(t, dir, opts, result)
}

func TestIncrementalRenderUpgradesLegacyMetadata(t *testing.T) {
	ctx, dir := context.Background(), t.TempDir()
	chat := Chat{ID: 1, Name: "Team"}
	opts := Options{Concurrency: 1, Resume: true, Format: "html", HTMLPageSize: 2}
	s := &fakeSource{items: renderTestItems(4, 3, 2, 1), pageSize: 100}
	s.items[3].Message.ExtraMedia = map[string]int64{"document_id": 9223372036854775806}
	if _, err := Run(ctx, s, dir, chat, opts); err != nil {
		t.Fatal(err)
	}
	db, err := bolt.Open(filepath.Join(dir, "archive.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket(metadataRowsBucket); err != nil {
			return err
		}
		if err := tx.DeleteBucket(renderChangesBucket); err != nil {
			return err
		}
		return tx.Bucket(metaBucket).Delete(renderCacheKey)
	})
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	result, err := Run(ctx, s, dir, chat, opts)
	if err != nil {
		t.Fatal(err)
	}
	assertFullRenderEquivalent(t, dir, opts, result)
	metadata, err := os.ReadFile(filepath.Join(dir, "archive-metadata.json"))
	if err != nil || !bytes.Contains(metadata, []byte("9223372036854775806")) {
		t.Fatalf("legacy upgrade lost metadata precision: %s, %v", metadata, err)
	}
}

func BenchmarkAppendSync(b *testing.B) {
	for _, count := range []int{10000, 100000} {
		for _, format := range []string{"html", "json", "both"} {
			b.Run(fmt.Sprintf("%d/%s", count, format), func(b *testing.B) {
				message := func(id int) Message {
					return Message{ID: id, Type: "message", Date: "2026-01-02T03:04:05", DateUnix: "1767323045", FromID: "user42", From: "Example User", Text: []any{strings.Repeat("hello world ", 10), TextEntity{Type: "bold", Text: "formatted text"}, " سلام دنیا"}}
				}
				items := make([]Item, count)
				for i := range items {
					items[i].Message = message(count - i)
				}
				ctx, dir, chat := context.Background(), b.TempDir(), Chat{ID: 1, Name: "Synthetic archive", Type: "personal_chat"}
				opts := Options{Concurrency: 4, Resume: true, Format: format, HTMLPageSize: 1000}
				// Seed in one page/transaction; setup is outside the timed append.
				source := &fakeSource{items: items, pageSize: count + 1}
				if _, err := Run(ctx, source, dir, chat, opts); err != nil {
					b.Fatal(err)
				}
				source.pageSize = 100
				latest := count
				b.ReportAllocs()
				for b.Loop() {
					latest++
					source.items = []Item{{Message: message(latest)}}
					if _, err := Run(ctx, source, dir, chat, opts); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
