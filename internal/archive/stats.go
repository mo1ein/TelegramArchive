package archive

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Stats keeps aggregates, never message history. Add another metric in Observe;
// live exports and offline analysis use the same code.
type Stats struct {
	Messages        int64            `json:"messages"`
	ServiceMessages int64            `json:"service_messages"`
	Words           int64            `json:"words"`
	Characters      int64            `json:"characters"`
	Replies         int64            `json:"replies"`
	Forwards        int64            `json:"forwards"`
	Edited          int64            `json:"edited"`
	MediaBytes      int64            `json:"media_bytes"`
	BySender        map[string]int64 `json:"by_sender"`
	ByDay           map[string]int64 `json:"by_day"`
	ByHour          [24]int64        `json:"by_hour"`
	ByMedia         map[string]int64 `json:"by_media"`
	FirstDate       string           `json:"first_date,omitempty"`
	LastDate        string           `json:"last_date,omitempty"`
}

func (s *Stats) Observe(m Message) {
	if s.BySender == nil {
		s.BySender = map[string]int64{}
		s.ByDay = map[string]int64{}
		s.ByMedia = map[string]int64{}
	}
	if m.Type == "service" {
		s.ServiceMessages++
	} else {
		s.Messages++
	}
	if m.FromID != "" {
		s.BySender[m.FromID]++
	}
	if len(m.Date) >= 10 {
		s.ByDay[m.Date[:10]]++
		if s.FirstDate == "" || m.Date < s.FirstDate {
			s.FirstDate = m.Date
		}
		if m.Date > s.LastDate {
			s.LastDate = m.Date
		}
	}
	if len(m.Date) >= 13 {
		if h, err := strconv.Atoi(m.Date[11:13]); err == nil && h >= 0 && h < 24 {
			s.ByHour[h]++
		}
	}
	if m.ReplyTo != 0 {
		s.Replies++
	}
	if m.ForwardedFrom != "" || m.ForwardedFromID != "" {
		s.Forwards++
	}
	if m.Edited != "" {
		s.Edited++
	}
	text := plainText(m.Text)
	if text == "" {
		text = plainText(m.Caption)
	}
	s.Words += int64(len(strings.Fields(text)))
	s.Characters += int64(utf8.RuneCountInString(text))
	kind := m.MediaType
	if m.Photo != "" {
		kind = "photo"
	} else if kind == "" && m.File != "" {
		kind = "document"
	}
	if m.Contact != nil {
		kind = "contact"
	}
	if m.Location != nil {
		kind = "location"
	}
	if m.Poll != nil {
		kind = "poll"
	}
	if kind != "" {
		s.ByMedia[kind]++
	}
	s.MediaBytes += m.PhotoSize + m.FileSize
}

func plainText(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case TextEntity:
		return v.Text
	case map[string]any:
		s, _ := v["text"].(string)
		return s
	case []any:
		var b strings.Builder
		for _, part := range v {
			b.WriteString(plainText(part))
		}
		return b.String()
	}
	return ""
}

// ReadStats streams one Desktop chat export, including one split part.
func ReadStats(ctx context.Context, r io.Reader, stats *Stats) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d := json.NewDecoder(r)
	d.UseNumber()
	t, err := d.Token()
	if err != nil {
		return err
	}
	if t != json.Delim('{') {
		return fmt.Errorf("expected a chat JSON object")
	}
	found := false
	for d.More() {
		if err := ctx.Err(); err != nil {
			return err
		}
		key, err := d.Token()
		if err != nil {
			return err
		}
		if key != "messages" {
			if err := skipJSON(d); err != nil {
				return err
			}
			continue
		}
		if found {
			return fmt.Errorf("duplicate messages array")
		}
		found = true
		t, err := d.Token()
		if err != nil {
			return err
		}
		if t != json.Delim('[') {
			return fmt.Errorf("messages must be an array")
		}
		for d.More() {
			if err := ctx.Err(); err != nil {
				return err
			}
			var m Message
			if err := d.Decode(&m); err != nil {
				return fmt.Errorf("decode message: %w", err)
			}
			stats.Observe(m)
		}
		if _, err := d.Token(); err != nil {
			return err
		}
	}
	if _, err := d.Token(); err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("no messages array; provide a per-chat result.json or result_part*.json")
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("unexpected data after chat object")
	}
	return nil
}

func skipJSON(d *json.Decoder) error {
	t, err := d.Token()
	if err != nil {
		return err
	}
	if delim, ok := t.(json.Delim); ok && (delim == '{' || delim == '[') {
		for d.More() {
			if err := skipJSON(d); err != nil {
				return err
			}
		}
		_, err = d.Token()
	}
	return err
}
