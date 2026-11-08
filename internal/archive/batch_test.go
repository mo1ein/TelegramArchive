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

type batchSource struct{ fakeSource }

func (s *batchSource) History(ctx context.Context, chat Chat, cursor string) (Page, error) {
	if chat.ID == 1 {
		return Page{}, errors.New("channel unavailable")
	}
	return s.fakeSource.History(ctx, chat, cursor)
}

func TestBatchContinuesAfterFailedChat(t *testing.T) {
	dir := t.TempDir()
	s := &batchSource{fakeSource: fakeSource{items: fakeItems(1), pageSize: 2}}
	report, err := RunBatch(context.Background(), s, dir, []Chat{{ID: 1, Key: "channel_1", Name: "Broken"}, {ID: 2, Key: "channel_2", Name: "<Available>"}}, Options{Concurrency: 1, Format: "both"}, nil)
	if err == nil || len(report.Chats) != 2 || report.Chats[0].Error == "" || report.Chats[1].Error != "" {
		t.Fatalf("%+v %v", report, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ChatExport_channel_2", "messages.html")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "lists", "chats.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "&lt;Available&gt;") || !strings.Contains(string(b), "channel unavailable") {
		t.Fatal("index lost successes/failures")
	}
	if _, err := os.Stat(filepath.Join(dir, "run-report.json")); err != nil {
		t.Fatal(err)
	}
}

func TestBatchExportFolderNames(t *testing.T) {
	for _, tc := range []struct {
		name, username, custom, directory string
	}{
		{"username", "example_channel", "", "ChatExport_channel_42_example_channel"},
		{"private chat", "", "", "ChatExport_channel_42"},
		{"custom label", "example_channel", "My backup", "ChatExport_channel_42_My backup"},
		{"safe custom label", "example_channel", "Team/Backup", "ChatExport_channel_42_Team_Backup"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			chat := Chat{ID: 42, Key: "channel_42", Name: "Original title", Username: tc.username}
			source := &fakeSource{items: fakeItems(1), pageSize: 2}
			report, err := RunBatch(context.Background(), source, root, []Chat{chat}, Options{Concurrency: 1, Format: "both", ExportName: tc.custom}, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range report.Chats[0].Paths {
				if !strings.HasPrefix(name, tc.directory+"/") {
					t.Fatalf("unexpected export path %q", name)
				}
			}
			for _, name := range []string{"result.json", "messages.html"} {
				if _, err := os.Stat(filepath.Join(root, tc.directory, name)); err != nil {
					t.Fatal(err)
				}
			}
			data, err := os.ReadFile(filepath.Join(root, tc.directory, "archive-metadata.json"))
			if err != nil {
				t.Fatal(err)
			}
			var metadata struct {
				Chat Chat `json:"chat"`
			}
			if err := json.Unmarshal(data, &metadata); err != nil {
				t.Fatal(err)
			}
			got := metadata.Chat
			if got.Name != chat.Name || got.Username != chat.Username {
				t.Fatalf("folder naming changed Telegram metadata: %+v", got)
			}
		})
	}
}

func TestBatchResumesAfterExportLabelChanges(t *testing.T) {
	for _, tc := range []struct {
		name, oldUsername, username, oldCustom, custom string
	}{
		{"legacy ID only", "", "new_username", "", ""},
		{"changed username", "old_username", "new_username", "", ""},
		{"removed username", "old_username", "", "", ""},
		{"changed custom label", "username", "username", "Before", "After"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			chat := Chat{ID: 42, Key: "channel_42", Name: "Title", Username: tc.oldUsername}
			opts := Options{Concurrency: 1, Resume: true, Media: []string{"photos"}, ExportName: tc.oldCustom}
			source := &fakeSource{items: fakeItems(3, 2, 1), pageSize: 2}
			first, err := RunBatch(context.Background(), source, root, []Chat{chat}, opts, nil)
			if err != nil {
				t.Fatal(err)
			}
			chat.Username, opts.ExportName = tc.username, tc.custom
			source = &fakeSource{items: fakeItems(4, 3, 2, 1), pageSize: 2}
			second, err := RunBatch(context.Background(), source, root, []Chat{chat}, opts, nil)
			if err != nil {
				t.Fatal(err)
			}
			if first.Chats[0].Paths[0] != second.Chats[0].Paths[0] {
				t.Fatalf("backup moved: %v -> %v", first.Chats[0].Paths, second.Chats[0].Paths)
			}
			if second.Chats[0].Stats.Messages != 4 || source.downloads.Load() != 1 {
				t.Fatalf("backup did not resume: messages=%d, downloads=%d", second.Chats[0].Stats.Messages, source.downloads.Load())
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			folders := 0
			for _, entry := range entries {
				if entry.IsDir() {
					folders++
				}
			}
			if folders != 1 {
				t.Fatalf("created duplicate backup folders: %d", folders)
			}
		})
	}
}
