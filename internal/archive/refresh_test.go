package archive

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRefreshUpdatesOldEditsWithoutDroppingDeletedMessages(t *testing.T) {
	dir := t.TempDir()
	s := &fakeSource{items: fakeItems(2, 1), pageSize: 2}
	opts := Options{Concurrency: 1, Resume: true}
	if _, err := Run(context.Background(), s, dir, Chat{ID: 1}, opts); err != nil {
		t.Fatal(err)
	}
	s.items = fakeItems(2)
	s.items[0].Message.Text = "edited"
	opts.Refresh = true
	if _, err := Run(context.Background(), s, dir, Chat{ID: 1}, opts); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "result.json"))
	var data struct{ Messages []Message }
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Messages) != 2 || data.Messages[1].Text != "edited" {
		t.Fatalf("lost history or edit: %+v", data)
	}
}
