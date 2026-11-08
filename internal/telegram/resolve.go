package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/gotd/td/telegram/deeplink"
	"github.com/mo1ein/TelegramArchive/internal/archive"
)

func isDisplayNameReference(ref string) bool {
	return !strings.HasPrefix(ref, "@") && !strings.HasPrefix(ref, "+") && !deeplink.IsDeeplinkLike(ref)
}

// Bare references prefer the name visible in the user's chat list. Explicit
// @usernames and Telegram links bypass this search.
func (s *Source) resolveNamedChat(ctx context.Context, ref string) (archive.Chat, bool, error) {
	if !s.namesLoaded {
		chats, err := s.Dialogs(ctx, []string{"personal", "bots", "groups", "channels"})
		if err != nil {
			return archive.Chat{}, false, fmt.Errorf("search chats for display name %q: %w; use @username for an explicit username", ref, err)
		}
		s.nameDialogs, s.namesLoaded = chats, true
	}
	name := strings.Join(strings.Fields(ref), " ")
	var found []archive.Chat
	for _, chat := range s.nameDialogs {
		if strings.EqualFold(strings.Join(strings.Fields(chat.Name), " "), name) {
			found = append(found, chat)
		}
	}
	if len(found) == 1 {
		return found[0], true, nil
	}
	if len(found) > 1 {
		ids := make([]string, 0, len(found))
		for _, chat := range found {
			ids = append(ids, fmt.Sprintf("%d (%s)", chat.ID, chat.Type))
		}
		return archive.Chat{}, false, fmt.Errorf("chat display name %q matches multiple chats: %s; select by ID or @username", ref, strings.Join(ids, ", "))
	}
	return archive.Chat{}, false, nil
}
