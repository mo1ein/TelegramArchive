package archive

import (
	"strings"
	"testing"
)

func TestNestedUTF16FormattingRendersWithoutDuplicatingText(t *testing.T) {
	m := Message{Text: "Hi 😀 world", TextEntities: []TextEntity{{Type: "bold", Offset: 3, Length: 8}, {Type: "italic", Offset: 6, Length: 5}}}
	h := string(renderMessageText(m))
	if !strings.Contains(h, "<strong><em>world</em></strong>") {
		t.Fatal(h)
	}
	if strings.Count(h, "world") != 1 || !strings.Contains(h, "😀") {
		t.Fatal(h)
	}
}
