package archive

import (
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func messageTime(m Message) time.Time {
	if unix, err := strconv.ParseInt(m.DateUnix, 10, 64); err == nil {
		return time.Unix(unix, 0).Local()
	}
	value, _ := time.ParseInLocation("2006-01-02T15:04:05", m.Date, time.Local)
	return value
}

func avatarColor(id string) int {
	if value, err := strconv.ParseInt(strings.TrimLeft(id, "abcdefghijklmnopqrstuvwxyz"), 10, 64); err == nil {
		return []int{1, 8, 5, 2, 7, 4, 6}[uint64(value)%7]
	}
	value := uint8(0xff)
	for _, ch := range []byte(id) {
		value = value*239 + ch
	}
	return []int{1, 8, 5, 2, 7, 4, 6}[value%7]
}

func fileSize(size int64) string {
	for _, unit := range []struct {
		size int64
		name string
	}{{1 << 20, "MB"}, {1 << 10, "KB"}} {
		if size >= unit.size {
			tenths := size * 10 / unit.size
			return fmt.Sprintf("%d.%d %s", tenths/10, tenths%10, unit.name)
		}
	}
	return fmt.Sprintf("%d B", size)
}

func serviceText(m Message) string {
	actor := m.Actor
	if actor == "" {
		actor = "Someone"
	}
	switch m.Action {
	case "create_group":
		return actor + " created group «" + m.Title + "» with members " + strings.Join(m.Members, ", ")
	case "create_channel":
		return actor + " created channel «" + m.Title + "»"
	case "edit_group_title":
		return actor + " changed group title to «" + m.Title + "»"
	case "edit_group_photo":
		return actor + " changed group photo"
	case "delete_group_photo":
		return actor + " removed group photo"
	case "invite_members":
		return actor + " invited " + strings.Join(m.Members, ", ")
	case "remove_members":
		return actor + " removed " + strings.Join(m.Members, ", ")
	case "join_group_by_link":
		return actor + " joined group by link from " + m.Inviter
	case "pin_message":
		return actor + " pinned this message"
	case "clear_history":
		return actor + " cleared history"
	case "migrate_to_supergroup":
		return "This group was migrated to a supergroup"
	case "migrate_from_group":
		return "This supergroup was migrated from a group"
	case "phone_call":
		return actor + " made a phone call"
	case "take_screenshot":
		return actor + " took a screenshot"
	}
	return actor + " " + strings.ReplaceAll(m.Action, "_", " ")
}

func prepareHTMLMessage(m Message, previous *Message) htmlMessage {
	stamp := messageTime(m)
	key := m.FromID
	if key == "" {
		key = m.From
	}
	view := htmlMessage{Message: m, Body: renderMessageText(m), Time: stamp.Format("15:04"), DateTitle: stamp.Format("02.01.2006 15:04:05 UTC-07:00"), Day: stamp.Format("2 January 2006"), Initials: desktopInitials(m.From), Color: avatarColor(key), ServiceText: serviceText(m)}
	// Desktop writes previews at twice their displayed size (520/2 for photos
	// and videos, 384/2 for stickers). Match that geometry without another codec.
	maximum := 520
	if m.MediaType == "sticker" {
		maximum = 384
	}
	view.PreviewWidth = maximum / 2
	if m.Width > 0 && m.Height > 0 {
		scale := min(1, float64(maximum)/float64(max(m.Width, m.Height)))
		view.PreviewWidth = max(1, (int(float64(m.Width)*scale)&^1)/2)
	}
	view.Body = template.HTML(strings.ReplaceAll(string(view.Body), "\n", "<br>"))
	if previous != nil && previous.Type != "service" && m.Type != "service" && m.FromID == previous.FromID && m.From == previous.From && m.ForwardedFrom == previous.ForwardedFrom && stamp.Format("2006-01-02") == messageTime(*previous).Format("2006-01-02") {
		delta := stamp.Sub(messageTime(*previous))
		limit := 15 * time.Minute
		if m.ForwardedFrom != "" {
			limit = time.Second
		}
		view.Joined = delta >= 0 && delta <= limit
	}
	view.MediaTitle = m.FileName
	view.MediaClass = "media_file"
	view.MediaStatus = fileSize(m.FileSize)
	switch m.MediaType {
	case "video_file", "video_message", "animation":
		view.MediaClass = "media_video"
		if view.MediaTitle == "" {
			view.MediaTitle = "Video file"
		}
	case "voice_message":
		view.MediaClass = "media_voice_message"
		view.MediaTitle = "Voice message"
	case "audio_file":
		view.MediaClass = "media_audio_file"
		if m.Title != "" {
			view.MediaTitle = m.Title
		}
		if m.Performer != "" {
			view.MediaTitle = m.Performer + " – " + view.MediaTitle
		}
	case "sticker":
		view.MediaClass = "media_photo"
		view.MediaTitle = "Sticker"
		view.MediaStatus = m.Emoji
	}
	if view.MediaTitle == "" {
		view.MediaTitle = "File"
	}
	if m.Duration > 0 {
		view.Duration = fmt.Sprintf("%02d:%02d", int(m.Duration)/60, int(m.Duration)%60)
		view.MediaStatus = view.Duration + ", " + view.MediaStatus
	}
	if m.Photo != "" && localMedia(m.Photo) == "" {
		view.MediaTitle = "Photo"
		view.MediaClass = "media_photo"
		view.MediaStatus = fileSize(m.PhotoSize)
	}
	if (m.Photo != "" && localMedia(m.Photo) == "") || (m.File != "" && localMedia(m.File) == "") {
		view.MediaDescription = "Not included, change data exporting settings to download."
	}
	if m.PhotoSkipReason == "unavailable" || m.FileSkipReason == "unavailable" {
		view.MediaDescription = "Unavailable, please try again later."
	}
	if m.Photo == TooLarge || m.File == TooLarge {
		view.MediaDescription = "Exceeds maximum size, change data exporting settings to download."
	}
	if m.Poll != nil {
		b, _ := json.Marshal(m.Poll)
		var poll htmlPoll
		if json.Unmarshal(b, &poll) == nil {
			view.PollView = &poll
		}
	}
	if m.Reactions != nil {
		b, _ := json.Marshal(m.Reactions)
		json.Unmarshal(b, &view.ReactionViews)
		for i := range view.ReactionViews {
			if view.ReactionViews[i].Emoji == "" {
				view.ReactionViews[i].Emoji = "👋"
				if view.ReactionViews[i].Type == "paid" {
					view.ReactionViews[i].Emoji = "⭐"
				}
			}
		}
	}
	return view
}

var htmlFunctions = template.FuncMap{
	"text": renderText,
	"lines": func(value any) template.HTML {
		return template.HTML(strings.ReplaceAll(html.EscapeString(plainValue(value)), "\n", "<br>"))
	},
	"votes": func(count int) string {
		if count == 0 {
			return "No votes"
		}
		if count == 1 {
			return "1 vote"
		}
		return fmt.Sprintf("%d votes", count)
	},
	"local": localMedia,
	"image": func(path string) bool {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".svg", ".avif":
			return true
		}
		return false
	},
	"initials": desktopInitials,
}

const desktopHead = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>Exported Data</title>
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src 'self' data:; media-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; base-uri 'none'; form-action 'none'">
<link href="{{.AssetBase}}css/style.css" rel="stylesheet"><script src="{{.AssetBase}}js/script.js" type="text/javascript"></script>
</head><body onload="CheckLocation();"><div class="page_wrap">
<div class="page_header">{{if .Back}}<a class="content block_link" href="{{.Back}}" onclick="return GoBack(this)">{{else}}<div class="content">{{end}}<div class="text bold">{{.Chat.Name}}</div>{{if .Back}}</a>{{else}}</div>{{end}}</div>
`

var htmlHeader = template.Must(template.New("header").Parse(desktopHead + `
<div class="page_body chat_page"><div class="history">
{{if .Previous}}<a class="pagination block_link" href="{{.Previous}}">Previous messages</a>{{end}}
`))

var htmlMessageTemplate = template.Must(template.New("message").Funcs(htmlFunctions).Parse(`{{if eq .Type "service"}}<div class="message service" id="message{{.ID}}"><div class="body details">{{.ServiceText}}{{if .ReplyTo}} <a href="{{.ReplyLink}}">Go to message</a>{{end}}</div></div>
{{else}}<div class="message default clearfix{{if .Joined}} joined{{end}}" id="message{{.ID}}">
{{if not .Joined}}<div class="pull_left userpic_wrap"><div class="userpic userpic{{.Color}}" style="width: 42px; height: 42px"><div class="initials" style="line-height: 42px">{{.Initials}}</div></div></div>{{end}}
<div class="body"><div class="pull_right date details" title="{{.DateTitle}}">{{.Time}}</div>
{{if not .Joined}}<div class="from_name">{{if .From}}{{.From}}{{else}}Deleted Account{{end}}{{if .ViaBot}} via {{.ViaBot}}{{end}}</div>{{end}}
{{if .ForwardedFrom}}<div class="pull_left forwarded userpic_wrap"><div class="userpic userpic{{.Color}}" style="width: 42px; height: 42px"><div class="initials" style="line-height: 42px">{{initials .ForwardedFrom}}</div></div></div><div class="forwarded body"><div class="from_name">{{.ForwardedFrom}}</div>{{end}}
{{if .ReplyTo}}<div class="reply_to details">In reply to {{if .ReplyToPeer}}a message in another chat{{else}}<a href="{{.ReplyLink}}"{{if .ReplySamePage}} onclick="return GoToMessage({{.ReplyTo}})"{{end}}>this message</a>{{end}}</div>{{end}}
{{if local .Photo}}<div class="media_wrap clearfix"><a class="photo_wrap clearfix pull_left" href="{{local .Photo}}"><img class="photo" src="{{local .Photo}}" alt="Photo" style="width: {{.PreviewWidth}}px; max-width: 100%; height: auto"{{if .Width}} width="{{.Width}}"{{end}}{{if .Height}} height="{{.Height}}"{{end}}></a></div>
{{else if and (eq .MediaType "sticker") (local .File) (image .File)}}<div class="media_wrap clearfix"><a class="sticker_wrap clearfix pull_left" href="{{local .File}}"><img class="sticker" src="{{local .File}}" alt="{{.Emoji}}" style="width: {{.PreviewWidth}}px; max-width: 100%; height: auto"></a></div>
{{else if and (eq .MediaClass "media_video") (local .File) (local .Thumbnail)}}<div class="media_wrap clearfix"><a class="video_file_wrap clearfix pull_left" href="{{local .File}}"><div class="video_play_bg"><div class="video_play"></div></div>{{if .Duration}}<div class="video_duration">{{.Duration}}</div>{{end}}<img class="video_file" src="{{local .Thumbnail}}" alt="Video file" style="width: {{.PreviewWidth}}px; max-width: 100%; height: auto"></a></div>
{{else if or .File .Photo}}<div class="media_wrap clearfix">{{if local .File}}<a class="media clearfix pull_left block_link {{.MediaClass}}" href="{{local .File}}">{{else}}<div class="media clearfix pull_left {{.MediaClass}}">{{end}}<div class="fill pull_left"></div><div class="body"><div class="title bold">{{.MediaTitle}}</div>{{if .MediaDescription}}<div class="description">{{.MediaDescription}}</div>{{end}}<div class="status details">{{.MediaStatus}}</div></div>{{if local .File}}</a>{{else}}</div>{{end}}</div>{{end}}
{{if .Contact}}<div class="media_wrap clearfix"><div class="media clearfix pull_left media_contact"><div class="fill pull_left"></div><div class="body"><div class="title bold">{{.Contact.FirstName}} {{.Contact.LastName}}</div><div class="status details">{{.Contact.Phone}}</div></div></div></div>{{end}}
{{if .GameTitle}}<div class="media_wrap clearfix"><div class="media clearfix pull_left media_game"><div class="fill pull_left"></div><div class="body"><div class="title bold">{{if .GameLink}}<a href="{{.GameLink}}">{{.GameTitle}}</a>{{else}}{{.GameTitle}}{{end}}</div><div class="description">{{.GameDescription}}</div></div></div></div>{{end}}
{{if .Invoice}}<div class="media_wrap clearfix"><div class="media clearfix pull_left media_invoice"><div class="fill pull_left"></div><div class="body"><div class="title bold">{{.Invoice.title}}</div><div class="description">{{.Invoice.description}}</div><div class="status details">{{.Invoice.currency}} {{.Invoice.amount}}</div></div></div></div>{{end}}
{{if .Location}}<div class="media_wrap clearfix"><div class="media clearfix pull_left media_location"><div class="fill pull_left"></div><div class="body"><div class="title bold">{{if .PlaceName}}{{.PlaceName}}{{else}}Location{{end}}</div>{{if .Address}}<div class="description">{{.Address}}</div>{{end}}<div class="status details">{{.Location.Latitude}}, {{.Location.Longitude}}</div></div></div></div>{{end}}
{{if .PollView}}<div class="media_wrap clearfix"><div class="media_poll"><div class="question bold">{{text .PollView.Question}}</div><div class="details">{{if .PollView.Closed}}Final results{{else}}Anonymous poll{{end}}</div>{{range .PollView.Answers}}<div class="answer">- {{text .Text}}{{if .Voters}} <span class="details">{{votes .Voters}}{{if .Chosen}}, chosen vote{{end}}</span>{{end}}</div>{{end}}<div class="total details">{{votes .PollView.Total}}</div></div></div>{{end}}
{{if .Body}}<div class="text">{{.Body}}</div>{{end}}
{{if .ReactionViews}}<span class="reactions">{{range .ReactionViews}}<span class="reaction"><span class="emoji">{{.Emoji}}</span><span class="count">{{.Count}}</span></span>{{end}}</span>{{end}}
{{if .ForwardedFrom}}</div>{{end}}</div></div>{{end}}
`))

var htmlFooter = template.Must(template.New("footer").Parse(`{{if .Empty}}<div class="message service" id="message-1"><div class="body details">No exported messages</div></div>{{end}}
{{if .Next}}<a class="pagination block_link" href="{{.Next}}">Next messages</a>{{end}}</div></div></div></body></html>`))
