package archive

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// Export views are separate from checkpoints, which retain ta's richer metadata.
// Reference: Telegram Desktop v7.4.1, export_output_json.cpp.
func desktopChatJSON(chat Chat) ([]byte, error) {
	var name any
	if chat.Type != "saved_messages" && chat.Type != "replies" && chat.Type != "verification_codes" {
		name = chat.Name
		if chat.Name == "" {
			name = json.RawMessage("null")
		}
	}
	return json.MarshalIndent(struct {
		Name any    `json:"name,omitempty"`
		Type string `json:"type"`
		ID   int64  `json:"id"`
	}{name, chat.Type, chat.ID}, "", " ")
}

func desktopMessageJSON(m Message) ([]byte, error) {
	if m.Type == "unsupported" {
		return json.MarshalIndent(struct {
			ID   int    `json:"id"`
			Type string `json:"type"`
		}{m.ID, m.Type}, "  ", " ")
	}
	if m.Type == "" {
		m.Type = "message"
	}
	m.Date = desktopDate(m.Date, m.DateUnix)
	m.Edited = desktopDate(m.Edited, m.EditedUnix)
	value := m.Text
	if plainText(value) == "" && m.Caption != nil {
		value = m.Caption
	}
	m.Text, m.TextEntities = desktopText(value, m.CustomEmojiFiles)
	m.CustomEmojiFiles, m.ActionData, m.ExtraMedia = nil, nil, nil
	m.ExportScope, m.Caption, m.TopicID, m.Pinned, m.Expires = "", nil, 0, false, ""
	m.Photo = desktopFile(m.Photo, m.PhotoSkipReason)
	m.File = desktopFile(m.File, m.FileSkipReason)
	m.Thumbnail = desktopFile(m.Thumbnail, m.ThumbnailSkipReason)
	m.PhotoSkipReason, m.FileSkipReason, m.ThumbnailSkipReason = "", "", ""
	if m.MediaType == "file" {
		m.MediaType = ""
	}
	if m.Location != nil && m.PlaceName == "" && m.Title != "" {
		m.PlaceName, m.Title = m.Title, ""
	}
	if m.Type == "service" {
		m.From, m.FromID = "", ""
	}
	if m.Poll != nil {
		if poll, ok := jsonValue(m.Poll).(map[string]any); ok {
			if answers, ok := poll["answers"].([]any); ok {
				for _, answer := range answers {
					if row, ok := answer.(map[string]any); ok {
						delete(row, "correct")
						if row["chosen"] == nil {
							row["chosen"] = false
						}
					}
				}
			}
			m.Poll = poll
		}
	}
	view := struct {
		Message
		From          any    `json:"from,omitempty"`
		Actor         any    `json:"actor,omitempty"`
		PhotoSize     *int64 `json:"photo_file_size,omitempty"`
		FileSize      *int64 `json:"file_size,omitempty"`
		ThumbnailSize *int64 `json:"thumbnail_file_size,omitempty"`
	}{Message: m}
	if m.From != "" {
		view.From = m.From
	} else if m.FromID != "" {
		view.From = json.RawMessage("null")
	}
	if m.Actor != "" {
		view.Actor = m.Actor
	} else if m.ActorID != "" {
		view.Actor = json.RawMessage("null")
	}
	if m.Photo != "" {
		view.PhotoSize = &m.PhotoSize
	}
	if m.File != "" {
		view.FileSize = &m.FileSize
	}
	if m.Thumbnail != "" {
		view.ThumbnailSize = &m.ThumbnailSize
	}
	return json.MarshalIndent(view, "  ", " ")
}

func desktopDate(value, unix string) string {
	if unix != "" {
		if ts, err := strconv.ParseInt(unix, 10, 64); err == nil {
			return time.Unix(ts, 0).Local().Format("2006-01-02T15:04:05")
		}
	}
	return value
}

func desktopFile(path, reason string) string {
	if reason == "unavailable" {
		return "(File unavailable, please try again later)"
	}
	if path == TooSmall {
		return Excluded
	}
	return path
}

func desktopText(value any, emojiFiles map[string]string) (any, []TextEntity) {
	entities := []TextEntity{}
	var parts []any
	appendPart := func(part any) {
		var e TextEntity
		switch p := part.(type) {
		case string:
			e = TextEntity{Type: "plain", Text: p}
		case TextEntity:
			e = p
		case map[string]any:
			b, _ := json.Marshal(p)
			json.Unmarshal(b, &e)
		default:
			return
		}
		e.Offset, e.Length = 0, 0
		if e.Type == "custom_emoji" {
			if path := emojiFiles[e.DocumentID]; path != "" {
				e.DocumentID = path
			}
		}
		if e.Type == "blockquote" && e.Collapsed == nil {
			collapsed := false
			e.Collapsed = &collapsed
		}
		if e.Type == "plain" {
			parts = append(parts, e.Text)
		} else {
			parts = append(parts, e)
		}
		if e.Text != "" {
			entities = append(entities, e)
		}
	}
	switch v := value.(type) {
	case []any:
		for _, part := range v {
			appendPart(part)
		}
	case []TextEntity:
		for _, part := range v {
			appendPart(part)
		}
	case nil:
		return "", entities
	default:
		appendPart(v)
	}
	if len(parts) == 1 {
		if text, ok := parts[0].(string); ok {
			return text, entities
		}
	}
	if len(parts) == 0 {
		return "", entities
	}
	return parts, entities
}

func desktopInitials(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return "?"
	}
	initials := string([]rune(parts[0])[0])
	if len(parts) > 1 {
		initials += string([]rune(parts[len(parts)-1])[0])
	}
	return strings.ToUpper(initials)
}
