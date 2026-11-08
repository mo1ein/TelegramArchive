package telegram

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/gotd/td/tg"
	"ta/internal/archive"
)

// Telegram uses UTF-16 code units for entity offsets, including surrogate pairs.
// Source: https://core.telegram.org/api/entities#message-entities
func convertText(text string, raw []tg.MessageEntityClass) (any, []archive.TextEntity) {
	if len(raw) == 0 {
		return text, []archive.TextEntity{{Type: "plain", Text: text}}
	}
	units := utf16.Encode([]rune(text))
	entities := slices.Clone(raw)
	slices.SortStableFunc(entities, func(a, b tg.MessageEntityClass) int { return cmp.Compare(a.GetOffset(), b.GetOffset()) })
	var parts []any
	var result []archive.TextEntity
	pos := 0
	plain := func(start, end int) {
		if end > start {
			s := string(utf16.Decode(units[start:end]))
			parts = append(parts, s)
			result = append(result, archive.TextEntity{Type: "plain", Text: s, Offset: start, Length: end - start})
		}
	}
	for _, e := range entities {
		start, end := e.GetOffset(), e.GetOffset()+e.GetLength()
		if start < 0 || end > len(units) || end <= start {
			continue
		}
		kind := snake(strings.TrimPrefix(e.TypeName(), "messageEntity"))
		switch kind {
		case "u_r_l", "url":
			kind = "link"
		case "text_u_r_l", "text_url":
			kind = "text_link"
		case "mention_name":
			kind = "mention_name"
		case "strike":
			kind = "strikethrough"
		}
		entity := archive.TextEntity{Type: kind, Text: string(utf16.Decode(units[start:end])), Offset: start, Length: end - start}
		switch e := e.(type) {
		case *tg.MessageEntityTextURL:
			entity.Href = e.URL
		case *tg.MessageEntityPre:
			entity.Language = e.Language
		case *tg.MessageEntityMentionName:
			entity.UserID = e.UserID
		case *tg.MessageEntityCustomEmoji:
			entity.DocumentID = strconv.FormatInt(e.DocumentID, 10)
		case *tg.MessageEntityBlockquote:
			collapsed := e.Collapsed
			entity.Collapsed = &collapsed
		}
		if start < pos {
			result = append(result, entity)
			continue
		} // Preserve nested styles without duplicating message text.
		plain(pos, start)
		parts = append(parts, entity)
		result = append(result, entity)
		pos = end
	}
	plain(pos, len(units))
	return parts, result
}
