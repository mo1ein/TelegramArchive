// Package tui provides the interactive command workspace for ta.
package tui

import (
	"encoding/csv"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type Action struct {
	Title, Description, Group string
	Path                      []string
	Command                   *cobra.Command
	Preset                    map[string]string
}

var Groups = []string{"Quick start", "All commands", "Backups", "Automation", "Transfers", "Accounts", "Chat tools", "Tools"}

// Catalog uses Cobra as the source of truth, including installed extensions.
// Hidden commands and implementation-only flags stay outside the interface.
func Catalog(root *cobra.Command) []Action {
	var actions []Action
	var walk func(*cobra.Command, []string)
	walk = func(cmd *cobra.Command, path []string) {
		if cmd.Hidden || cmd.Deprecated != "" || cmd.Name() == "tui" || cmd.Name() == "help" {
			return
		}
		if len(path) > 0 && cmd.Runnable() {
			actions = append(actions, Action{Title: strings.Join(path, " "), Description: cmd.Short, Group: group(path), Path: slices.Clone(path), Command: cmd})
		}
		for _, child := range cmd.Commands() {
			walk(child, append(slices.Clone(path), child.Name()))
		}
	}
	walk(root, nil)
	slices.SortFunc(actions, func(a, b Action) int { return strings.Compare(a.Title, b.Title) })
	var quick []Action
	for _, preset := range []struct {
		path, title, description string
		values                   map[string]string
	}{
		{"chat ls", "Browse your chats", "Choose a chat, inspect its details, and archive it without typing an ID.", nil},
		{"accounts", "Your Telegram accounts", "Select a saved account for your next backup.", nil},
		{"cron list", "Scheduled backups", "Review schedules, run a backup, or inspect its logs.", nil},
		{"login", "Connect a Telegram account", "Sign in with a QR code to get started.", map[string]string{"type": "qr"}},
		{"archive", "Archive a chat", "Save a chat with media, JSON and an offline HTML reader.", map[string]string{"chat": "me", "media": "all", "format": "both"}},
		{"archive", "Back up all channels", "Archive your joined channels, including private channels.", map[string]string{"all": "true", "types": "channels", "media": "all", "format": "both"}},
		{"sync", "Back up everything", "Incrementally sync chats, account data and Saved Messages.", map[string]string{"everything": "true", "format": "both"}},
		{"account", "Save account data", "Choose account sections and back up Saved Messages.", map[string]string{"format": "both"}},
		{"cron add", "Schedule a backup", "Install a recurring backup every six hours.", map[string]string{"everything": "true", "every": "6h", "format": "both"}},
	} {
		for _, action := range actions {
			if strings.Join(action.Path, " ") == preset.path {
				action.Title, action.Description, action.Group, action.Preset = preset.title, preset.description, "Quick start", preset.values
				quick = append(quick, action)
				break
			}
		}
	}
	return append(quick, actions...)
}

func group(path []string) string {
	switch path[0] {
	case "archive", "account", "sync", "render", "stats":
		return "Backups"
	case "cron", "schedule":
		return "Automation"
	case "download", "upload", "forward":
		return "Transfers"
	case "login", "accounts", "backup", "recover", "migrate":
		return "Accounts"
	case "chat":
		return "Chat tools"
	default:
		return "Tools"
	}
}

type Field struct {
	Name, Label, Help, Type, Value, Default string
	Choices                                 []string
	Required, Advanced, Global, Dirty       bool
	Secret                                  bool
}

type Form struct {
	Action Action
	Fields []Field
}

var common = []string{"ns", "arguments", "type", "chat", "all", "everything", "include", "account", "types", "media", "format", "output", "dir", "path", "url", "file", "from", "to", "every", "target", "since", "until", "extensions", "resume", "run-now"}

// NewForm reads metadata without changing the flags bound to the running CLI.
func NewForm(action Action, globals map[string]string) Form {
	form := Form{Action: action}
	cmd := action.Command
	flags := pflag.NewFlagSet("form", pflag.ContinueOnError)
	flags.AddFlagSet(cmd.InheritedFlags())
	flags.AddFlagSet(cmd.Flags())
	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden || flag.Deprecated != "" || flag.Name == "help" || flag.Name == "version" {
			return
		}
		value := flag.DefValue
		if flag.Changed {
			value = flag.Value.String()
		}
		if slice, ok := flag.Value.(pflag.SliceValue); ok {
			value = csvString(slice.GetSlice())
		}
		if flag.Value.Type() == "stringToString" {
			value = strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
		}
		_, required := flag.Annotations[cobra.BashCompOneRequiredFlag]
		global := cmd.Root().PersistentFlags().Lookup(flag.Name) == flag
		field := Field{Name: flag.Name, Label: strings.ReplaceAll(flag.Name, "-", " "), Help: flag.Usage, Type: flag.Value.Type(), Value: value, Default: value, Required: required, Global: global, Advanced: !slices.Contains(common, flag.Name), Secret: flag.Name == "proxy" || strings.Contains(flag.Name, "password") || strings.Contains(flag.Name, "passcode") || strings.Contains(flag.Name, "token")}
		if flag.Name == "ns" {
			field.Label = "account namespace"
		}
		if flag.Name == "type" && strings.Join(action.Path, " ") == "login" {
			field.Label = "login method"
		}
		field.Choices = choices(action.Path, flag.Name, flag.Usage)
		if value, ok := globals[flag.Name]; ok && global {
			field.Value, field.Dirty = value, true
		}
		if value, ok := action.Preset[flag.Name]; ok {
			field.Value, field.Dirty = value, true
		}
		field.Advanced = field.Advanced && !required && !field.Dirty
		form.Fields = append(form.Fields, field)
	})
	// Some inherited commands accept arguments without declaring them in Use.
	// Keep an argument field for every runnable command; Cobra validates it.
	argumentHelp := "Separate arguments with spaces; quote paths or names containing spaces. Usage: " + cmd.UseLine()
	if strings.Join(action.Path, " ") == "schedule" {
		argumentHelp = "Backup options, for example: --all --media all --format both. These follow the -- separator."
	}
	argumentLabel := "arguments"
	switch strings.Join(action.Path, " ") {
	case "render":
		argumentLabel = "archive directory"
	case "stats":
		argumentLabel = "JSON exports"
	case "cron add", "cron remove", "cron run", "cron logs":
		argumentLabel = "job name"
	case "extension install":
		argumentLabel = "extension sources"
	case "extension remove", "extension upgrade":
		argumentLabel = "extension names"
	case "schedule":
		argumentLabel = "backup options"
	}
	required := cmd.ValidateArgs(nil) != nil
	noArgs := !required && cmd.ValidateArgs([]string{"example"}) != nil
	form.Fields = append(form.Fields, Field{Name: "arguments", Label: argumentLabel, Help: argumentHelp, Type: "arguments", Required: required, Advanced: noArgs})
	slices.SortStableFunc(form.Fields, func(a, b Field) int {
		rank := func(f Field) int {
			if f.Required {
				return -1
			}
			if i := slices.Index(common, f.Name); i >= 0 {
				return i
			}
			return len(common)
		}
		return rank(a) - rank(b)
	})
	return form
}

func choices(path []string, name, usage string) []string {
	key := strings.Join(path, " ")
	if name == "format" && slices.Contains([]string{"archive", "account", "sync", "render", "cron add"}, key) {
		return []string{"json", "html", "both"}
	}
	if name == "type" && key == "login" {
		return []string{"desktop", "code", "qr"}
	}
	if name == "target" && key == "schedule" {
		return []string{"archive", "account", "sync"}
	}
	if !slices.Contains([]string{"type", "mode", "output", "target"}, name) {
		return nil
	}
	if start := strings.Index(usage, "["); start >= 0 {
		if end := strings.Index(usage[start:], "]"); end > 0 {
			values := strings.Split(usage[start+1:start+end], ",")
			if len(values) > 1 {
				for i := range values {
					values[i] = strings.TrimSpace(values[i])
				}
				return values
			}
		}
	}
	return nil
}

func csvString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	var out strings.Builder
	w := csv.NewWriter(&out)
	_ = w.Write(values)
	w.Flush()
	return strings.TrimSuffix(out.String(), "\n")
}

func (f Field) Display() string {
	if f.Secret && f.Value != "" {
		return "••••••••"
	}
	if f.Value == "" {
		return "—"
	}
	if f.Type == "bool" {
		if f.Value == "true" {
			return "● on"
		}
		return "○ off"
	}
	return f.Value
}

func (f Form) field(name string) *Field {
	for i := range f.Fields {
		if f.Fields[i].Name == name {
			return &f.Fields[i]
		}
	}
	return nil
}

func (f Field) Error(err error) error { return fmt.Errorf("%s: %w", f.Label, err) }
