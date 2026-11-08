package archive

import (
	"html"
	"html/template"
	"slices"
	"strings"
	"unicode/utf16"
)

func renderMessageText(m Message) template.HTML {
	value := m.Text
	if plainText(value) == "" && m.Caption != nil {
		value = m.Caption
	}
	var ranges []TextEntity
	for _, e := range m.TextEntities {
		if e.Type != "plain" && e.Length > 0 {
			ranges = append(ranges, e)
		}
	}
	if len(ranges) == 0 {
		if len(m.CustomEmojiFiles) > 0 {
			value, _ = desktopText(value, m.CustomEmojiFiles)
		}
		return renderText(value)
	}
	units := utf16.Encode([]rune(plainText(value)))
	bounds := []int{0, len(units)}
	valid := ranges[:0]
	for _, e := range ranges {
		end := e.Offset + e.Length
		if e.Offset < 0 || end > len(units) || end <= e.Offset {
			continue
		}
		valid = append(valid, e)
		bounds = append(bounds, e.Offset, end)
	}
	ranges = valid
	slices.Sort(bounds)
	bounds = slices.Compact(bounds)
	slices.SortStableFunc(ranges, func(a, b TextEntity) int {
		if a.Offset != b.Offset {
			return a.Offset - b.Offset
		}
		return b.Length - a.Length
	})
	var out strings.Builder
	for i := 1; i < len(bounds); i++ {
		start, end := bounds[i-1], bounds[i]
		text := html.EscapeString(string(utf16.Decode(units[start:end])))
		for j := len(ranges) - 1; j >= 0; j-- {
			e := ranges[j]
			if path := m.CustomEmojiFiles[e.DocumentID]; e.Type == "custom_emoji" && path != "" {
				e.DocumentID = path
			}
			if e.Offset <= start && e.Offset+e.Length >= end {
				text = formattedText(e, text)
			}
		}
		out.WriteString(text)
	}
	return template.HTML(out.String())
}

func formattedText(e TextEntity, escaped string) string {
	tag := ""
	switch e.Type {
	case "bold":
		tag = "strong"
	case "italic":
		tag = "em"
	case "underline":
		tag = "u"
	case "strikethrough":
		tag = "s"
	case "code":
		tag = "code"
	case "pre":
		tag = "pre"
	case "blockquote":
		tag = "blockquote"
	case "spoiler":
		return `<span class="spoiler hidden" tabindex="0" role="button" onclick="ShowSpoiler(this)" onkeydown="if(event.key==='Enter'||event.key===' '){event.preventDefault();ShowSpoiler(this)}"><span aria-hidden="true">` + escaped + `</span></span>`
	case "mention":
		return `<a href="https://t.me/` + html.EscapeString(strings.TrimPrefix(e.Text, "@")) + `">` + escaped + `</a>`
	case "mention_name":
		return `<a href="" onclick="return ShowMentionName()">` + escaped + `</a>`
	case "hashtag":
		return `<a href="" data-tag="` + html.EscapeString(strings.TrimPrefix(e.Text, "#")) + `" onclick="return ShowHashtag(this.dataset.tag)">` + escaped + `</a>`
	case "cashtag":
		return `<a href="" data-tag="` + html.EscapeString(strings.TrimPrefix(e.Text, "$")) + `" onclick="return ShowCashtag(this.dataset.tag)">` + escaped + `</a>`
	case "bot_command":
		return `<a href="" data-command="` + html.EscapeString(strings.TrimPrefix(e.Text, "/")) + `" onclick="return ShowBotCommand(this.dataset.command)">` + escaped + `</a>`
	case "custom_emoji":
		if path := localMedia(e.DocumentID); path != "" && strings.Contains(e.DocumentID, "/") {
			return `<a href="` + html.EscapeString(path) + `">` + escaped + `</a>`
		}
		return `<a href="" onclick="return ShowNotLoadedEmoji()">` + escaped + `</a>`
	case "link", "text_link", "email", "phone":
		link := e.Href
		if link == "" {
			link = e.Text
			if e.Type == "email" {
				link = "mailto:" + link
			}
			if e.Type == "phone" {
				link = "tel:" + link
			}
		}
		if href := safeLink(link); href != "" {
			return `<a rel="noreferrer noopener" href="` + html.EscapeString(href) + `">` + escaped + `</a>`
		}
	}
	if tag != "" {
		return "<" + tag + ">" + escaped + "</" + tag + ">"
	}
	return escaped
}
