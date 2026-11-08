package telegram

import (
	"context"
	"fmt"

	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"
	"ta/internal/archive"
)

func peerKey(p tg.PeerClass) string {
	switch p := p.(type) {
	case *tg.PeerUser:
		return fmt.Sprintf("user_%d", p.UserID)
	case *tg.PeerChat:
		return fmt.Sprintf("chat_%d", p.ChatID)
	case *tg.PeerChannel:
		return fmt.Sprintf("channel_%d", p.ChannelID)
	}
	return ""
}

// Explicitly visit both Telegram folders. Inaccessible peers can still supply a
// pagination cursor, so one removed channel cannot truncate the remaining list.
func (s *Source) folderDialogs(ctx context.Context, api *tg.Client, folder int, types []string) ([]archive.Chat, error) {
	var chats []archive.Chat
	offsetID, offsetDate := 0, 0
	var offsetPeer tg.InputPeerClass = &tg.InputPeerEmpty{}
	previous := ""
	for {
		req := &tg.MessagesGetDialogsRequest{OffsetID: offsetID, OffsetDate: offsetDate, OffsetPeer: offsetPeer, Limit: 100, ExcludePinned: offsetID != 0}
		req.SetFolderID(folder)
		raw, err := api.MessagesGetDialogs(ctx, req)
		if err != nil {
			return chats, fmt.Errorf("list folder %d: %w", folder, err)
		}
		var dialogs []tg.DialogClass
		var messages []tg.MessageClass
		var entities peer.Entities
		var rawChats []tg.ChatClass
		done := false
		switch r := raw.(type) {
		case *tg.MessagesDialogs:
			dialogs = r.Dialogs
			messages = r.Messages
			rawChats = r.Chats
			entities = peer.EntitiesFromResult(r)
			done = true
		case *tg.MessagesDialogsSlice:
			dialogs = r.Dialogs
			messages = r.Messages
			rawChats = r.Chats
			entities = peer.EntitiesFromResult(r)
		default:
			return chats, fmt.Errorf("unexpected dialogs response %T", raw)
		}
		if len(dialogs) == 0 {
			return chats, nil
		}
		lastMessages := map[string]tg.NotEmptyMessage{}
		for _, m := range messages {
			if m, ok := m.AsNotEmpty(); ok {
				lastMessages[peerKey(m.GetPeerID())] = m
			}
		}
		cursorKey := ""
		for _, d := range dialogs {
			if _, ok := d.(*tg.DialogFolder); ok {
				continue
			}
			key := peerKey(d.GetPeer())
			input, extractErr := entities.ExtractPeer(d.GetPeer())
			if extractErr != nil {
				for _, c := range rawChats {
					if c, ok := c.(*tg.ChannelForbidden); ok && key == fmt.Sprintf("channel_%d", c.ID) {
						input = &tg.InputPeerChannel{ChannelID: c.ID, AccessHash: c.AccessHash}
						break
					}
				}
				s.Warnings = append(s.Warnings, fmt.Sprintf("folder %d: %s is unavailable", folder, key))
			}
			if input == nil {
				continue
			}
			cursorKey = key
			offsetPeer = input
			if m := lastMessages[key]; m != nil {
				offsetID = m.GetID()
				offsetDate = m.GetDate()
			} else if dlg, ok := d.(*tg.Dialog); ok {
				offsetID = dlg.TopMessage
				offsetDate = 0
			}
			if extractErr != nil {
				continue
			}
			var p peers.Peer
			switch raw := input.(type) {
			case *tg.InputPeerUser:
				if u, ok := entities.User(raw.UserID); ok {
					p = s.manager.User(u)
				}
			case *tg.InputPeerChat:
				if c, ok := entities.Chat(raw.ChatID); ok && !c.Left && !c.Deactivated {
					p = s.manager.Chat(c)
				}
			case *tg.InputPeerChannel:
				if c, ok := entities.Channel(raw.ChannelID); ok && !c.Left {
					p = s.manager.Channel(c)
				}
			}
			if p != nil {
				chat := s.addChat(p)
				if matches(chat, types) {
					chats = append(chats, chat)
				}
			}
		}
		if done {
			return chats, nil
		}
		next := fmt.Sprintf("%s/%d/%d", cursorKey, offsetID, offsetDate)
		if cursorKey == "" || next == previous {
			return chats, fmt.Errorf("folder %d pagination cannot advance past unavailable peers", folder)
		}
		previous = next
	}
}
