// Package telegram adapts gotd messages and pooled transfers to the archive engine.
package telegram

import (
	"bytes"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/tg"
	"github.com/iyear/tdl/core/tmedia"

	"github.com/mo1ein/TelegramArchive/internal/archive"
)

func date(ts int) string { return time.Unix(int64(ts), 0).UTC().Format("2006-01-02T15:04:05") }

func sender(p tg.PeerClass, e peer.Entities) (string, string) {
	switch p := p.(type) {
	case *tg.PeerUser:
		id := "user" + strconv.FormatInt(p.UserID, 10)
		if u, ok := e.User(p.UserID); ok {
			return strings.TrimSpace(u.FirstName + " " + u.LastName), id
		}
		return id, id
	case *tg.PeerChat:
		id := "chat" + strconv.FormatInt(p.ChatID, 10)
		if c, ok := e.Chat(p.ChatID); ok {
			return c.Title, id
		}
		return id, id
	case *tg.PeerChannel:
		id := "channel" + strconv.FormatInt(p.ChannelID, 10)
		if c, ok := e.Channel(p.ChannelID); ok {
			return c.Title, id
		}
		return id, id
	}
	return "", ""
}

func Convert(raw tg.NotEmptyMessage, e peer.Entities, selfID int64) archive.Item {
	m := archive.Message{ID: raw.GetID(), Type: "message", Date: date(raw.GetDate()), DateUnix: strconv.Itoa(raw.GetDate()), Text: "", TextEntities: []archive.TextEntity{}}
	item := archive.Item{}
	switch v := raw.(type) {
	case *tg.MessageService:
		m.Type = "service"
		from := v.FromID
		if from == nil && v.Out && selfID > 0 {
			from = &tg.PeerUser{UserID: selfID}
		}
		m.Actor, m.ActorID = sender(from, e)
		if v.Action != nil {
			m.Action = snake(strings.TrimPrefix(v.Action.TypeName(), "messageAction"))
			m.ActionData = v.Action
		}
		if _, ok := v.Action.(*tg.MessageActionPinMessage); ok {
			m.Action = "pin_message"
		}
		item.Files = serviceAction(v, e, selfID, &m)
		if r, ok := v.ReplyTo.(*tg.MessageReplyHeader); ok {
			m.ReplyTo = r.ReplyToMsgID
			m.TopicID = r.ReplyToTopID
			_, m.ReplyToPeer = sender(r.ReplyToPeerID, e)
		}
	case *tg.Message:
		from := v.FromID
		if from == nil && v.Out && selfID > 0 {
			from = &tg.PeerUser{UserID: selfID}
		}
		if from == nil {
			from = v.PeerID
		}
		m.From, m.FromID = sender(from, e)
		m.Author = v.PostAuthor
		if bot, ok := e.User(v.ViaBotID); ok && bot.Username != "" {
			m.ViaBot = "@" + bot.Username
		}
		m.Text, m.TextEntities = convertText(v.Message, v.Entities)
		if v.EditDate > 0 {
			m.Edited = date(v.EditDate)
			m.EditedUnix = strconv.Itoa(v.EditDate)
		}
		if r, ok := v.ReplyTo.(*tg.MessageReplyHeader); ok {
			m.ReplyTo = r.ReplyToMsgID
			m.TopicID = r.ReplyToTopID
			_, m.ReplyToPeer = sender(r.ReplyToPeerID, e)
		}
		if v.FwdFrom.FromID != nil {
			m.ForwardedFrom, m.ForwardedFromID = sender(v.FwdFrom.FromID, e)
		}
		if v.FwdFrom.FromName != "" {
			m.ForwardedFrom = v.FwdFrom.FromName
		}
		if v.FwdFrom.SavedFromPeer != nil {
			m.SavedFrom, _ = sender(v.FwdFrom.SavedFromPeer, e)
		}
		if len(v.Reactions.Results) > 0 {
			reactions := make([]map[string]any, 0, len(v.Reactions.Results))
			for _, r := range v.Reactions.Results {
				value := map[string]any{"count": r.Count}
				switch x := r.Reaction.(type) {
				case *tg.ReactionEmoji:
					value["type"] = "emoji"
					value["emoji"] = x.Emoticon
				case *tg.ReactionCustomEmoji:
					value["type"] = "custom_emoji"
					value["document_id"] = strconv.FormatInt(x.DocumentID, 10)
				case *tg.ReactionPaid:
					value["type"] = "paid"
				default:
					value["type"] = r.Reaction.TypeName()
				}
				reactions = append(reactions, value)
			}
			m.Reactions = reactions
		}
		switch md := v.Media.(type) {
		case *tg.MessageMediaPhoto:
			m.MediaSpoiler, m.SelfDestruct = md.Spoiler, md.TTLSeconds
			m.Photo = archive.Excluded
			m.PhotoSkipReason = "unavailable"
			if p, ok := md.Photo.(*tg.Photo); ok && len(p.Sizes) > 0 {
				// The transfer core assumes progressive sizes contain at least one byte size.
				if last, ok := p.Sizes[len(p.Sizes)-1].(*tg.PhotoSizeProgressive); ok && len(last.Sizes) == 0 {
					break
				}
				if media, ok := tmedia.GetPhotoInfo(md); ok {
					m.PhotoSize = media.Size
					m.PhotoSkipReason = ""
					item.Files = append(item.Files, remoteFile("photos", "photo", v.ID, media))
					for _, size := range p.Sizes {
						switch s := size.(type) {
						case *tg.PhotoSize:
							m.Width = s.W
							m.Height = s.H
						case *tg.PhotoSizeProgressive:
							m.Width = s.W
							m.Height = s.H
						}
					}
				}
			}
		case *tg.MessageMediaDocument:
			m.MediaSpoiler, m.SelfDestruct = md.Spoiler, md.TTLSeconds
			if doc, ok := md.Document.(*tg.Document); ok {
				kind := documentKind(doc, &m)
				m.FileSize = doc.Size
				m.FileName = tmedia.GetDocumentName(doc)
				m.MIME = doc.MimeType
				if media, ok := tmedia.GetDocumentInfo(md); ok {
					item.Files = append(item.Files, remoteFile(kind, "file", v.ID, media))
				}
				for _, thumb := range doc.Thumbs {
					if size, ok := thumb.(*tg.PhotoSize); ok {
						m.ThumbnailSize = int64(size.Size)
						media := &tmedia.Media{Name: "thumb.jpg", Size: int64(size.Size), DC: doc.DCID, InputFileLoc: &tg.InputDocumentFileLocation{ID: doc.ID, AccessHash: doc.AccessHash, FileReference: doc.FileReference, ThumbSize: size.Type}}
						item.Files = append(item.Files, remoteFile(kind, "thumbnail", v.ID, media))
						break
					}
				}
			}
		case *tg.MessageMediaContact:
			m.Contact = &archive.Contact{FirstName: md.FirstName, LastName: md.LastName, Phone: md.PhoneNumber}
			vcard := md.Vcard
			if vcard == "" {
				esc := func(s string) string {
					return strings.NewReplacer("\\", "\\\\", "\r", "", "\n", "\\n", ";", "\\;", ",", "\\,").Replace(s)
				}
				vcard = fmt.Sprintf("BEGIN:VCARD\r\nVERSION:3.0\r\nFN:%s %s\r\nN:%s;%s;;;\r\nTEL:%s\r\nEND:VCARD\r\n", esc(md.FirstName), esc(md.LastName), esc(md.LastName), esc(md.FirstName), esc(md.PhoneNumber))
			}
			item.Files = append(item.Files, archive.File{Kind: "contacts", Field: "contact_vcard", Path: fmt.Sprintf("contacts/%d.vcf", v.ID), Data: []byte(vcard), Size: int64(len(vcard))})
		case *tg.MessageMediaGeo:
			m.Location = location(md.Geo)
		case *tg.MessageMediaGeoLive:
			m.Location = location(md.Geo)
			m.LiveLocationPeriod = md.Period
		case *tg.MessageMediaVenue:
			m.PlaceName, m.Address, m.Location = md.Title, md.Address, location(md.Geo)
		case *tg.MessageMediaGame:
			m.GameTitle, m.GameDescription = md.Game.Title, md.Game.Description
			if bot, ok := e.User(v.ViaBotID); ok && bot.Username != "" {
				m.GameLink = "https://t.me/" + bot.Username + "?game=" + md.Game.ShortName
			}
		case *tg.MessageMediaInvoice:
			m.Invoice = map[string]any{"title": md.Title, "description": md.Description, "amount": md.TotalAmount, "currency": md.Currency}
		case *tg.MessageMediaPoll:
			answers := make([]map[string]any, 0, len(md.Poll.Answers))
			for _, answer := range md.Poll.Answers {
				answerText := answer.GetText()
				text, _ := convertText(answerText.Text, answerText.Entities)
				a := map[string]any{"text": text, "voters": 0, "chosen": false}
				if choice, ok := answer.(*tg.PollAnswer); ok {
					for _, r := range md.Results.Results {
						if bytes.Equal(choice.Option, r.Option) {
							a["voters"] = r.Voters
							a["chosen"] = r.Chosen
							a["correct"] = r.Correct
						}
					}
				}
				answers = append(answers, a)
			}
			question, _ := convertText(md.Poll.Question.Text, md.Poll.Question.Entities)
			m.Poll = map[string]any{"question": question, "closed": md.Poll.Closed, "total_voters": md.Results.TotalVoters, "answers": answers}
		case nil:
		default:
			m.ExtraMedia = md // Preserve metadata for media without a Desktop mapping.
		}
	}
	item.Message = m
	return item
}

func location(raw tg.GeoPointClass) *archive.Location {
	if p, ok := raw.(*tg.GeoPoint); ok {
		return &archive.Location{Latitude: p.Lat, Longitude: p.Long}
	}
	return nil
}

func documentKind(doc *tg.Document, m *archive.Message) string {
	sticker, animated, video, round, audio, voice := false, false, false, false, false, false
	for _, attr := range doc.Attributes {
		switch a := attr.(type) {
		case *tg.DocumentAttributeSticker:
			sticker = true
			m.Emoji = a.Alt
		case *tg.DocumentAttributeAnimated:
			animated = true
		case *tg.DocumentAttributeVideo:
			video = true
			round = a.RoundMessage
			m.Width = a.W
			m.Height = a.H
			m.Duration = a.Duration
		case *tg.DocumentAttributeAudio:
			audio = true
			voice = a.Voice
			m.Duration = float64(a.Duration)
			m.Performer = a.Performer
			m.Title = a.Title
		case *tg.DocumentAttributeImageSize:
			m.Width = a.W
			m.Height = a.H
		}
	}
	switch {
	case sticker:
		m.MediaType = "sticker"
		return "stickers"
	case round:
		m.MediaType = "video_message"
		return "video_messages"
	case voice:
		m.MediaType = "voice_message"
		return "voice_messages"
	case animated:
		m.MediaType = "animation"
		return "animations"
	case video:
		m.MediaType = "video_file"
		return "videos"
	case audio:
		m.MediaType = "audio_file"
		return "audios"
	}
	return "documents"
}

func remoteFile(kind, field string, id int, media *tmedia.Media) archive.File {
	dir := map[string]string{"videos": "video_files", "audios": "audio_files", "video_messages": "round_video_messages", "documents": "files"}[kind]
	if dir == "" {
		dir = kind
	}
	name := safeName(media.Name)
	if loc, ok := media.InputFileLoc.(*tg.InputDocumentFileLocation); ok {
		name = strconv.FormatInt(loc.ID, 10) + "_" + name
	}
	return archive.File{Kind: kind, Field: field, Path: fmt.Sprintf("%s/%d_%s", dir, id, name), Size: media.Size, Remote: media}
}

func safeName(name string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune("<>:\"/\\|?*", r) {
			return '_'
		}
		return r
	}, name)
	name = strings.Trim(name, " .")
	if name == "" {
		name = "file"
	}
	if len(name) > 180 {
		name = name[:180]
		for !utf8.ValidString(name) {
			name = name[:len(name)-1]
		}
	}
	return name
}

func snake(name string) string {
	var b strings.Builder
	for i, r := range name {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}
