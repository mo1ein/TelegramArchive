package archive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteSplitAndReadStats(t *testing.T) {
	dir := t.TempDir()
	chat := Chat{Name: "گفتگو", Type: "personal_chat", ID: 42}
	messages := []Message{
		{ID: 1, Type: "message", Date: "2026-01-02T03:04:05", DateUnix: "1767323045", FromID: "user42", Text: "سلام دنیا"},
		{ID: 2, Type: "message", Date: "2026-01-02T04:04:05", DateUnix: "1767326645", FromID: "user7", Text: []any{"hi ", TextEntity{Type: "bold", Text: "there"}}, ReplyTo: 1, File: "files/2.txt", FileSize: 9},
		{ID: 3, Type: "service", Date: "2026-01-03T04:04:05", Text: "", Action: "pin_message"},
	}
	files, stats, err := Write(context.Background(), dir, chat, 500, func(yield func(Message) error) error {
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
	if len(files) < 2 {
		t.Fatalf("expected split output, got %v", files)
	}
	var ids []int
	for _, name := range files {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Name     string
			ID       int64
			Messages []Message
		}
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		if result.Name != chat.Name || result.ID != chat.ID {
			t.Fatal("split part lost chat metadata")
		}
		for _, m := range result.Messages {
			ids = append(ids, m.ID)
		}
		if len(data) > 500 && len(result.Messages) != 1 {
			t.Fatalf("part exceeded size: %d", len(data))
		}
	}
	if len(ids) != 3 || ids[0] != 1 || ids[2] != 3 {
		t.Fatalf("message order: %v", ids)
	}
	if stats.Messages != 2 || stats.ServiceMessages != 1 || stats.Words != 4 || stats.Replies != 1 {
		t.Fatalf("wrong stats: %+v", stats)
	}
	var offline Stats
	for _, name := range files {
		f, err := os.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		err = ReadStats(context.Background(), f, &offline)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	if offline.Messages != stats.Messages || offline.Words != stats.Words {
		t.Fatalf("offline stats mismatch: %+v", offline)
	}
}

func TestWriterFailurePreservesPublishedArchive(t *testing.T) {
	dir := t.TempDir()
	old := []byte(`{"messages":[]}`)
	if err := os.WriteFile(filepath.Join(dir, "result.json"), old, 0600); err != nil {
		t.Fatal(err)
	}
	want := errors.New("disk/source failed")
	_, _, err := Write(context.Background(), dir, Chat{}, 0, func(yield func(Message) error) error {
		if err := yield(Message{ID: 1, Text: "new"}); err != nil {
			return err
		}
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("got %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "result.json"))
	if !bytes.Equal(old, got) {
		t.Fatal("published archive changed after failure")
	}
}

func TestReadStatsRejectsInvalidAndCancelledInput(t *testing.T) {
	for _, data := range []string{`{"messages":[`, `{"name":"no messages"}`, `{"messages":[]}junk`, `[]`} {
		if err := ReadStats(context.Background(), strings.NewReader(data), &Stats{}); err == nil {
			t.Fatalf("accepted %q", data)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ReadStats(ctx, strings.NewReader(`{"messages":[]}`), &Stats{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func BenchmarkStats(b *testing.B) {
	m := Message{ID: 1, Type: "message", Date: "2026-01-02T03:04:05", FromID: "user42", Text: "hello world سلام دنیا", MediaType: "photo"}
	var stats Stats
	b.ReportAllocs()
	for b.Loop() {
		stats.Observe(m)
	}
}

func BenchmarkReadStats(b *testing.B) {
	data := []byte(`{"messages":[` + strings.Repeat(`{"id":1,"type":"message","date":"2026-01-02T03:04:05","from_id":"user42","text":"hello world"},`, 999) + `{"id":1000,"type":"message","text":"last"}]}`)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		if err := ReadStats(context.Background(), bytes.NewReader(data), &Stats{}); err != nil && err != io.EOF {
			b.Fatal(err)
		}
	}
}
