package tui

import (
	"slices"
	"strings"
)

func specialChoice(field, value string) bool {
	return (value == "all" && slices.Contains([]string{"media", "include", "account-include"}, field)) || (value == "none" && field == "media")
}

// The full CLI remains available under More options. Everyday forms expose
// only the decisions relevant to the selected task.
func essential(path []string, name string) bool {
	var names []string
	switch strings.Join(path, " ") {
	case "archive":
		names = []string{"chat", "all", "types", "media", "format", "output", "since", "until"}
	case "sync":
		names = []string{"everything", "chat", "all", "media", "format", "output", "since", "until"}
	case "account":
		names = []string{"include", "media", "format", "output", "since", "until"}
	case "login":
		names = []string{"type", "desktop", "passcode", "ns"}
	case "cron add":
		names = []string{"everything", "chat", "every", "at-startup", "format", "output", "since", "until"}
	default:
		return slices.Contains(common, name)
	}
	return slices.Contains(names, name)
}

func fieldLabel(name string) string {
	if label, ok := map[string]string{
		"chat": "Chats to back up", "all": "All joined chats", "everything": "Entire account",
		"media": "Media to download", "format": "Export format", "output": "Save to folder",
		"every": "Repeat backup every", "at-startup": "Catch up at startup", "ns": "Saved account",
		"desktop": "Telegram data folder", "passcode": "Local app passcode", "since": "Since (date/time)",
		"until": "Until (date/time)", "resume": "Resume interrupted backup", "types": "Chat categories",
		"include": "Account sections", "proxy": "Connection proxy",
	}[name]; ok {
		return label
	}
	return strings.ReplaceAll(name, "-", " ")
}

func choiceLabel(name, value string) string {
	if name == "ns" {
		return safeText(value)
	}
	if name == "media" && value == "all" {
		return "All media"
	}
	if name == "media" && value == "none" {
		return "No media"
	}
	if name == "type" {
		switch value {
		case "desktop":
			return "Installed app"
		case "qr":
			return "QR code"
		case "code":
			return "Phone code"
		}
	}
	if name == "format" {
		switch value {
		case "json":
			return "JSON"
		case "html":
			return "HTML"
		case "both":
			return "Both"
		}
	}
	label := strings.ReplaceAll(value, "_", " ")
	if label != "" {
		runes := []rune(label)
		label = strings.ToUpper(string(runes[0])) + string(runes[1:])
	}
	return label
}

func (f *Form) reconcileSelection(changed string) {
	clear := func(name, value string) {
		if field := f.field(name); field != nil && field.Value != value {
			field.Value, field.Dirty = value, true
		}
	}
	field := f.field(changed)
	if field == nil {
		return
	}
	switch {
	case changed == "all" && field.Value == "true":
		clear("chat", "")
		clear("everything", "false")
	case changed == "everything" && field.Value == "true":
		clear("chat", "")
		clear("all", "false")
		if media := f.field("media"); media != nil && !media.Dirty {
			media.Value, media.Dirty = "all", true
		}
	case changed == "chat" && field.Value != "":
		clear("all", "false")
		clear("everything", "false")
	}
}
