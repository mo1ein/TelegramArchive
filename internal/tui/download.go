package tui

import (
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The downloader consumes --url, not positional command-line arguments.
func (f *Form) configureDownload() {
	f.Action.Title = "Download from a link"
	f.Fields = slices.DeleteFunc(f.Fields, func(field Field) bool { return field.Name == "arguments" })
	labels := map[string]string{
		"url": "Telegram message links", "dir": "Save to folder", "group": "Download entire album",
		"skip-same": "Skip existing files", "include": "Only extensions", "exclude": "Exclude extensions",
		"file": "Telegram export files", "continue": "Continue previous download", "restart": "Restart previous download",
		"desc": "Newest files first", "template": "File name template", "rewrite-ext": "Detect file extensions",
		"serve": "Serve files over HTTP", "port": "HTTP port", "takeout": "Use takeout session",
	}
	for i := range f.Fields {
		field := &f.Fields[i]
		if label, ok := labels[field.Name]; ok {
			field.Label = label
		}
		if field.Name == "url" {
			values, _ := readCSV(field.Value)
			field.Type, field.Multi, field.Advanced = "links", false, false
			field.Value = strings.Join(values, " ")
			field.Default = field.Value
			field.Help = "Copy a message link in Telegram and paste it here. Separate multiple links with spaces or newlines."
		}
	}
	order := []string{"url", "dir", "ns", "group", "skip-same"}
	slices.SortStableFunc(f.Fields, func(a, b Field) int {
		rank := func(name string) int {
			if n := slices.Index(order, name); n >= 0 {
				return n
			}
			return len(order)
		}
		return rank(a.Name) - rank(b.Name)
	})
}

func (m Model) downloadForm() bool { return strings.Join(m.form.Action.Path, " ") == "download" }

func (m Model) editingDownloadURL() bool {
	if m.screen != configure || m.searching || !m.downloadForm() {
		return false
	}
	fields := m.visibleFields()
	return m.focus >= 0 && m.focus < len(fields) && m.form.Fields[fields[m.focus]].Type == "links"
}

func (m *Model) startDownload() tea.Cmd {
	m.prepareReview()
	if m.screen != review {
		return nil
	}
	return m.begin()
}

func downloadURLs(value string) ([]string, error) {
	links := strings.Fields(value)
	for i, link := range links {
		if !strings.Contains(link, "://") {
			link = "https://" + link
		}
		parsed, err := url.Parse(link)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || !slices.Contains([]string{"t.me", "telegram.me", "telegram.dog"}, strings.ToLower(parsed.Hostname())) {
			return nil, fmt.Errorf("paste a Telegram message link, such as https://t.me/channel/123")
		}
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		messageID, err := strconv.Atoi(parts[len(parts)-1])
		if len(parts) < 2 || len(parts) > 4 || messageID <= 0 || err != nil {
			return nil, fmt.Errorf("copy the link to a specific Telegram message, not a channel or invite")
		}
		links[i] = link
	}
	return links, nil
}

func (m Model) downloadURLBar(field Field, width int, focused bool) string {
	border := lipgloss.NewStyle().Foreground(line)
	if focused {
		border = border.Foreground(mint)
	}
	value := field.Display()
	if field.Value == "" {
		value = placeholder(field)
	}
	content := dim.Width(width-2).Padding(0, 1).Render(clip(value, width-4))
	if focused {
		content = m.fieldControl(field, width-2)
	}
	return border.Render("╭─ URL "+strings.Repeat("─", max(0, width-8))+"╮") + "\n" +
		border.Render("│") + content + border.Render("│") + "\n" +
		border.Render("╰"+strings.Repeat("─", max(0, width-2))+"╯")
}
