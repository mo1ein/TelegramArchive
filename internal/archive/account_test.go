package archive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAccountOutputIsSeparateAndEscapesHTML(t *testing.T) {
	dir := t.TempDir()
	report := AccountReport{AccountID: 42, Sections: []AccountSection{{Name: "personal_info", Data: map[string]any{"first_name": "<script>alert(1)</script>", "user_id": int64(42)}}, {Name: "contacts", Data: map[string]any{"list": []map[string]any{{"first_name": "Alice", "phone_number": "123"}}}}, {Name: "saved_messages", Files: []string{"saved_messages/messages.html"}}, {Name: "stories", Error: "temporary failure"}}}
	if err := WriteAccount(dir, "both", report); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "account.json"))
	var data map[string]any
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatal(err)
	}
	if data["personal_information"] == nil || data["chats"] == nil {
		t.Fatal(data)
	}
	h, _ := os.ReadFile(filepath.Join(dir, "account.html"))
	contacts, _ := os.ReadFile(filepath.Join(dir, "lists", "contacts.html"))
	if strings.Contains(string(h), "<script>") || !strings.Contains(string(h), "saved_messages/messages.html") || !strings.Contains(string(contacts), "Alice") {
		t.Fatal("unsafe/incomplete account HTML")
	}
}
