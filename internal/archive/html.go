package archive

import (
	"bufio"
	"context"
	"fmt"
	"html"
	"html/template"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type htmlPage struct {
	AssetBase, Back string
	Chat            Chat
	Number          int
	Previous, Next  string
	Empty           bool
}
type htmlMessage struct {
	PollView *htmlPoll
	Message
	Body                                                            template.HTML
	ReplyLink                                                       string
	ReplySamePage, Joined                                           bool
	Time, DateTitle, Day, Initials, ServiceText                     string
	Color                                                           int
	PreviewWidth                                                    int
	MediaTitle, MediaStatus, MediaClass, MediaDescription, Duration string
	ReactionViews                                                   []struct {
		Type  string `json:"type"`
		Emoji string `json:"emoji"`
		Count int    `json:"count"`
	}
}
type pageRange struct{ First, Last, Page int }

func htmlPageName(n int) string {
	if n == 1 {
		return "messages.html"
	}
	return fmt.Sprintf("messages%d.html", n)
}

// WriteHTML renders a bounded number of messages per page. No external scripts,
// fonts or images are required. Temporary pages are completed before publishing.
func WriteHTML(ctx context.Context, dir string, chat Chat, pageSize int, each func(func(Message) error) error) (paths []string, stats Stats, rerr error) {
	if pageSize == 0 {
		pageSize = 1000
	}
	if pageSize < 1 || pageSize > 10000 {
		return nil, stats, fmt.Errorf("HTML page size must be between 1 and 10000")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, stats, err
	}
	temps := map[int]string{}
	defer removeHTMLTemps(temps)
	var ranges []pageRange
	var messages []Message
	flush := func(next bool) error {
		number := len(ranges) + 1
		tmp, err := stageHTMLPage(ctx, dir, chat, number, next, ranges, messages)
		if err != nil {
			return err
		}
		temps[number] = tmp
		if len(messages) > 0 {
			ranges = append(ranges, pageRange{messages[0].ID, messages[len(messages)-1].ID, number})
		}
		messages = messages[:0]
		return nil
	}
	err := each(func(m Message) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(messages) == pageSize {
			if err := flush(true); err != nil {
				return err
			}
		}
		messages = append(messages, m)
		stats.Observe(m)
		return nil
	})
	if err != nil {
		return nil, stats, err
	}
	if err := flush(false); err != nil {
		return nil, stats, err
	}
	paths, err = publishHTMLPages(ctx, dir, max(1, len(ranges)), temps)
	return paths, stats, err
}

// Date separator IDs are local to each document, so an edit on one page does
// not change generated IDs on every later page.
func stageHTMLPage(ctx context.Context, dir string, chat Chat, number int, next bool, ranges []pageRange, messages []Message) (name string, rerr error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, ".html-*")
	if err != nil {
		return "", err
	}
	defer func() {
		f.Close()
		if rerr != nil {
			os.Remove(f.Name())
		}
	}()
	w := bufio.NewWriterSize(f, 64*1024)
	page := htmlPage{Chat: chat, Number: number, Empty: len(messages) == 0}
	if desktopAssetDirectory(dir) != dir {
		page.AssetBase = "../"
	}
	base := filepath.Base(dir)
	if strings.HasPrefix(base, "ChatExport_") {
		page.Back = "../lists/chats.html"
	} else if base == "saved_messages" {
		page.Back = "../export_results.html"
	}
	if number > 1 {
		page.Previous = htmlPageName(number - 1)
	}
	if next {
		page.Next = htmlPageName(number + 1)
	}
	if err := htmlHeader.Execute(w, page); err != nil {
		return "", err
	}
	var previous *Message
	dateMessageID := 0
	for i := range messages {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		m := &messages[i]
		view := prepareHTMLMessage(*m, previous)
		if previous == nil || view.Day != messageTime(*previous).Format("2 January 2006") {
			dateMessageID--
			if _, err := fmt.Fprintf(w, "<div class=\"message service\" id=\"message%d\"><div class=\"body details\">%s</div></div>\n", dateMessageID, html.EscapeString(view.Day)); err != nil {
				return "", err
			}
		}
		if m.ReplyTo > 0 {
			target := number
			i, ok := slices.BinarySearchFunc(ranges, m.ReplyTo, comparePageRange)
			if ok && ranges[i].Page < number {
				target = ranges[i].Page
			}
			view.ReplyLink = htmlPageName(target) + "#go_to_message" + strconv.Itoa(m.ReplyTo)
			view.ReplySamePage = target == number
		}
		if err := htmlMessageTemplate.Execute(w, view); err != nil {
			return "", err
		}
		previous = m
	}
	if err := htmlFooter.Execute(w, page); err != nil {
		return "", err
	}
	if err := w.Flush(); err != nil {
		return "", err
	}
	if err := f.Sync(); err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return f.Name(), nil
}

func comparePageRange(p pageRange, id int) int {
	if p.Last < id {
		return -1
	}
	if p.First > id {
		return 1
	}
	return 0
}

func removeHTMLTemps(temps map[int]string) {
	for _, name := range temps {
		os.Remove(name)
	}
}

func publishHTMLPages(ctx context.Context, dir string, count int, temps map[int]string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := writeDesktopAssets(desktopAssetDirectory(dir)); err != nil {
		return nil, err
	}
	var paths []string
	keep := map[string]bool{}
	for number := 1; number <= count; number++ {
		dest := filepath.Join(dir, htmlPageName(number))
		if tmp := temps[number]; tmp != "" {
			if err := os.Rename(tmp, dest); err != nil {
				return nil, err
			}
		}
		paths = append(paths, dest)
		keep[dest] = true
	}
	old, err := filepath.Glob(filepath.Join(dir, "messages*.html"))
	if err != nil {
		return nil, err
	}
	for _, p := range old {
		base := filepath.Base(p)
		suffix := strings.TrimSuffix(strings.TrimPrefix(base, "messages"), ".html")
		_, err := strconv.Atoi(suffix)
		if !keep[p] && (suffix == "" || err == nil) {
			if err := os.Remove(p); err != nil {
				return nil, err
			}
		}
	}
	return paths, nil
}

type htmlPoll struct {
	Question any  `json:"question"`
	Total    int  `json:"total_voters"`
	Closed   bool `json:"closed"`
	Answers  []struct {
		Text   any  `json:"text"`
		Voters int  `json:"voters"`
		Chosen bool `json:"chosen"`
	} `json:"answers"`
}

func localMedia(value string) string {
	if value == "" || strings.HasPrefix(value, "(") || strings.ContainsAny(value, "\\\r\n\x00") || !filepath.IsLocal(value) {
		return ""
	}
	return (&url.URL{Path: filepath.ToSlash(value)}).String()
}

func safeLink(value string) string {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host != "" {
			return u.String()
		}
	case "mailto", "tg", "tel":
		return u.String()
	}
	return ""
}

func renderText(value any) template.HTML {
	var b strings.Builder
	var part func(any)
	part = func(v any) {
		switch v := v.(type) {
		case []any:
			for _, item := range v {
				part(item)
			}
			return
		case string:
			b.WriteString(html.EscapeString(v))
			return
		case map[string]any:
			entity := TextEntity{}
			entity.Type, _ = v["type"].(string)
			entity.Text, _ = v["text"].(string)
			entity.Href, _ = v["href"].(string)
			part(entity)
			return
		case TextEntity:
			b.WriteString(formattedText(v, html.EscapeString(v.Text)))
			return
		}
	}
	part(value)
	// Only fixed tags are introduced above; all Telegram text and attributes escape.
	return template.HTML(b.String())
}
