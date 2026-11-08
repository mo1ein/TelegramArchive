package telegram

import (
	"context"
	"fmt"
	"github.com/gotd/td/tg"
	"github.com/iyear/tdl/core/tmedia"
	"github.com/mo1ein/TelegramArchive/internal/archive"
	"strconv"
)

func (s *Source) attachCustomEmojis(ctx context.Context, items []archive.Item) error {
	if !s.DownloadCustomEmoji {
		return nil
	}
	seen := map[int64]bool{}
	var ids []int64
	for _, item := range items {
		for _, entity := range item.Message.TextEntities {
			if entity.Type != "custom_emoji" {
				continue
			}
			id, err := strconv.ParseInt(entity.DocumentID, 10, 64)
			if err == nil && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	documents := map[int64]*tg.Document{}
	for start := 0; start < len(ids); start += 100 {
		end := min(start+100, len(ids))
		raw, err := s.accountAPI().MessagesGetCustomEmojiDocuments(ctx, ids[start:end])
		if err != nil {
			return fmt.Errorf("resolve custom emoji: %w", err)
		}
		for _, raw := range raw {
			if doc, ok := raw.(*tg.Document); ok {
				documents[doc.ID] = doc
			}
		}
	}
	for i := range items {
		added := map[int64]bool{}
		for _, entity := range items[i].Message.TextEntities {
			id, _ := strconv.ParseInt(entity.DocumentID, 10, 64)
			doc := documents[id]
			if doc == nil || added[id] {
				continue
			}
			added[id] = true
			media, ok := tmedia.GetDocumentInfo(&tg.MessageMediaDocument{Document: doc})
			if !ok {
				continue
			}
			file := remoteFile("stickers", "custom_emoji", items[i].Message.ID, media)
			file.Path = fmt.Sprintf("stickers/custom_emoji_%d_%s", doc.ID, safeName(media.Name))
			file.ResourceID = entity.DocumentID
			file.MIME = doc.MimeType
			items[i].Files = append(items[i].Files, file)
		}
	}
	return nil
}
