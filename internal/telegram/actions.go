package telegram

import (
	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/tg"
	"github.com/mo1ein/TelegramArchive/internal/archive"
	"strings"
)

func serviceAction(v *tg.MessageService, e peer.Entities, selfID int64, m *archive.Message) []archive.File {
	users := func(ids []int64) []string {
		var names []string
		for _, id := range ids {
			name, _ := sender(&tg.PeerUser{UserID: id}, e)
			names = append(names, name)
		}
		return names
	}
	switch a := v.Action.(type) {
	case *tg.MessageActionChatCreate:
		m.Action = "create_group"
		m.Title = a.Title
		m.Members = users(a.Users)
	case *tg.MessageActionChannelCreate:
		m.Action = "create_channel"
		m.Title = a.Title
	case *tg.MessageActionChatEditTitle:
		m.Action = "edit_group_title"
		m.Title = a.Title
	case *tg.MessageActionChatEditPhoto:
		m.Action = "edit_group_photo"
		photo := Convert(&tg.Message{ID: v.ID, Date: v.Date, Media: &tg.MessageMediaPhoto{Photo: a.Photo}}, e, selfID)
		m.Photo = photo.Message.Photo
		m.PhotoSize = photo.Message.PhotoSize
		m.Width = photo.Message.Width
		m.Height = photo.Message.Height
		return photo.Files
	case *tg.MessageActionChatDeletePhoto:
		m.Action = "delete_group_photo"
	case *tg.MessageActionChatAddUser:
		m.Action = "invite_members"
		m.Members = users(a.Users)
	case *tg.MessageActionChatDeleteUser:
		m.Action = "remove_members"
		m.Members = users([]int64{a.UserID})
	case *tg.MessageActionChatJoinedByLink:
		m.Action = "join_group_by_link"
		m.Inviter, _ = sender(&tg.PeerUser{UserID: a.InviterID}, e)
	case *tg.MessageActionChatMigrateTo:
		m.Action = "migrate_to_supergroup"
	case *tg.MessageActionChannelMigrateFrom:
		m.Action = "migrate_from_group"
		m.Title = a.Title
	case *tg.MessageActionPinMessage:
		m.Action = "pin_message"
	case *tg.MessageActionHistoryClear:
		m.Action = "clear_history"
	case *tg.MessageActionGameScore:
		m.Action = "score_in_game"
		m.Score = a.Score
	case *tg.MessageActionPaymentSent:
		m.Action = "send_payment"
		m.Currency = a.Currency
		m.Amount = a.TotalAmount
	case *tg.MessageActionPhoneCall:
		m.Action = "phone_call"
		m.Duration = float64(a.Duration)
		if a.Reason != nil {
			m.DiscardReason = snake(strings.TrimPrefix(a.Reason.TypeName(), "phoneCallDiscardReason"))
		}
	case *tg.MessageActionScreenshotTaken:
		m.Action = "take_screenshot"
	case *tg.MessageActionBotAllowed:
		m.Action = "allow_sending_messages"
	}
	return nil
}
