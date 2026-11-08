package telegram

import (
	"context"
	"errors"
	"fmt"
	"github.com/gotd/td/tg"
	"github.com/mo1ein/TelegramArchive/internal/archive"
	"slices"
)

func (s *Source) LeftDialogs(ctx context.Context, types []string) ([]archive.Chat, error) {
	if s.takeoutID == 0 {
		return nil, fmt.Errorf("left-channel discovery requires --takeout")
	}
	var chats []archive.Chat
	for offset := 0; ; {
		raw, err := s.takeoutAPI().ChannelsGetLeftChannels(ctx, offset)
		if err != nil {
			return chats, err
		}
		var entries []tg.ChatClass
		done := false
		switch r := raw.(type) {
		case *tg.MessagesChats:
			entries = r.Chats
			done = true
		case *tg.MessagesChatsSlice:
			entries = r.Chats
		default:
			return chats, fmt.Errorf("unexpected left-channel response %T", raw)
		}
		if len(entries) == 0 {
			break
		}
		for _, raw := range entries {
			if c, ok := raw.(*tg.Channel); ok {
				chat := s.addChat(s.manager.Channel(c))
				chat.OwnMessagesOnly = true
				if matches(chat, types) {
					chats = append(chats, chat)
				}
			}
		}
		offset += len(entries)
		if done {
			break
		}
	}
	return chats, nil
}

// Old and new group message IDs can overlap; keep migrated histories in linked,
// separate folders under their original peer IDs so neither can overwrite data.
func (s *Source) MigratedGroups(ctx context.Context, chats []archive.Chat) ([]archive.Chat, error) {
	result := slices.Clone(chats)
	seen := map[string]bool{}
	for _, chat := range chats {
		seen[chat.Key] = true
	}
	var failures []error
	for _, chat := range chats {
		if chat.Type != "public_supergroup" && chat.Type != "private_supergroup" {
			continue
		}
		p, ok := s.chats[chat.Key].(*tg.InputPeerChannel)
		if !ok {
			continue
		}
		r, err := s.api.ChannelsGetFullChannel(ctx, &tg.InputChannel{ChannelID: p.ChannelID, AccessHash: p.AccessHash})
		if err != nil {
			failures = append(failures, fmt.Errorf("migration metadata for %s: %w", chat.Name, err))
			continue
		}
		full, ok := r.FullChat.(*tg.ChannelFull)
		if !ok || full.MigratedFromChatID == 0 {
			continue
		}
		old, err := s.manager.GetChat(ctx, full.MigratedFromChatID)
		if err != nil {
			failures = append(failures, fmt.Errorf("original group for %s: %w", chat.Name, err))
			continue
		}
		legacy := s.addChat(old)
		legacy.MigratedToID = chat.ID
		if !seen[legacy.Key] {
			result = append(result, legacy)
			seen[legacy.Key] = true
		}
	}
	return result, errors.Join(failures...)
}
