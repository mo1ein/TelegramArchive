package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mo1ein/TelegramArchive/internal/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func TestTUICoversPublicCommandsAndOptions(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	root := New()
	root.InitDefaultCompletionCmd()
	actions := tui.Catalog(root)
	found := map[string]bool{}
	for _, action := range actions {
		path := strings.Join(action.Path, " ")
		found[path] = true
		form := tui.NewForm(action, nil)
		fields := map[string]bool{}
		for _, f := range form.Fields {
			fields[f.Name] = true
		}
		flags := pflag.NewFlagSet("check", pflag.ContinueOnError)
		flags.AddFlagSet(action.Command.InheritedFlags())
		flags.AddFlagSet(action.Command.Flags())
		flags.VisitAll(func(f *pflag.Flag) {
			if !f.Hidden && f.Deprecated == "" && f.Name != "help" && f.Name != "version" && !fields[f.Name] {
				t.Errorf("%s missing --%s", path, f.Name)
			}
		})
		if action.Group == "Quick start" && path != "cron add" {
			plan, err := form.Plan(nil)
			if err != nil {
				t.Fatalf("preset %s: %v", action.Title, err)
			}
			// Parse presets through the real definitions without running hooks or Telegram.
			clone := New()
			leaf, args, err := clone.Find(plan.Args)
			if err != nil {
				t.Fatal(err)
			}
			if err := leaf.ParseFlags(args); err != nil {
				t.Fatal(action.Title, err)
			}
		}
	}
	for _, path := range []string{"archive", "account", "sync", "cron add", "cron list", "cron logs", "cron remove", "cron run", "schedule", "login", "accounts", "download", "upload", "forward", "chat ls", "chat export", "chat users", "render", "stats", "backup", "recover", "migrate", "extension install", "extension list", "extension remove", "extension upgrade", "completion bash", "completion zsh", "completion fish", "completion powershell"} {
		if !found[path] {
			t.Errorf("missing %s", path)
		}
	}
	if found["version"] {
		t.Fatal("version belongs in the header, not the workflow catalogue")
	}
	if found["tui"] {
		t.Fatal("workspace must not recursively launch itself")
	}
}

func TestTUIAndRootDoNotOpenSessionForHelpOrPipes(t *testing.T) {
	for _, args := range [][]string{nil, {"tui", "--help"}, {"tui"}, {"typo-command"}} {
		viper.Reset()
		root := New()
		root.SetIn(strings.NewReader(""))
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(args)
		root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
			if cmd.Annotations["offline"] != "true" {
				t.Fatal("TUI initialized Telegram storage")
			}
			return nil
		}
		err := root.ExecuteContext(context.Background())
		if len(args) == 1 && args[0] == "tui" {
			if err == nil || !strings.Contains(err.Error(), "needs a terminal") {
				t.Fatal(err)
			}
		} else if len(args) == 1 && args[0] == "typo-command" {
			if err == nil {
				t.Fatal("unknown command opened the UI")
			}
		} else if err != nil || !strings.Contains(out.String(), "tui") {
			t.Fatalf("args=%v err=%v output=%s", args, err, out.String())
		}
	}
	viper.Reset()
}

func TestDownloadFormUsesRealURLFlagWithoutArguments(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	for _, action := range tui.Catalog(New()) {
		if strings.Join(action.Path, " ") != "download" {
			continue
		}
		form := tui.NewForm(action, nil)
		if form.Fields[0].Name != "url" || form.Fields[0].Multi {
			t.Fatal("URL should be the first plain input")
		}
		for i := range form.Fields {
			field := &form.Fields[i]
			if field.Name == "arguments" {
				t.Fatal("download exposes unused positional arguments")
			}
			if field.Name == "url" {
				field.Value, field.Dirty = "https://t.me/channel/123?x=a,b https://t.me/c/123/456", true
			}
		}
		plan, err := form.Plan(nil)
		if err != nil {
			t.Fatal(err)
		}
		leaf, args, err := New().Find(plan.Args)
		if err != nil {
			t.Fatal(err)
		}
		if err := leaf.ParseFlags(args); err != nil {
			t.Fatal(err)
		}
		urls, err := leaf.Flags().GetStringSlice("url")
		if err != nil || len(urls) != 2 || urls[0] != "https://t.me/channel/123?x=a,b" {
			t.Fatal(urls, err)
		}
		return
	}
	t.Fatal("download workflow missing")
}

func TestDownloadFormRoundTripsEveryPublicFlag(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	var download tui.Action
	for _, action := range tui.Catalog(New()) {
		if strings.Join(action.Path, " ") == "download" {
			download = action
			break
		}
	}
	if download.Command == nil {
		t.Fatal("download missing")
	}
	fields := tui.NewForm(download, nil).Fields
	for _, original := range fields {
		t.Run(original.Name, func(t *testing.T) {
			form := tui.NewForm(download, nil)
			value := "sample value"
			switch original.Type {
			case "links":
				value = "https://t.me/example/123"
			case "bool":
				value = "true"
			case "int":
				value = "9"
			case "duration":
				value = "2s"
			case "stringSlice":
				value = "one,two"
			case "stringToString":
				value = "type=bolt,path=/tmp/download-form-test"
			case "string":
			default:
				t.Fatalf("add a representative value for flag type %q", original.Type)
			}
			for i := range form.Fields {
				field := &form.Fields[i]
				if field.Name == "url" {
					field.Value, field.Dirty = "https://t.me/example/123", true
				}
				if field.Name == original.Name {
					field.Value, field.Dirty = value, true
				}
			}
			plan, err := form.Plan(nil)
			if err != nil {
				t.Fatal(err)
			}
			root := New()
			leaf, args, err := root.Find(plan.Args)
			if err != nil {
				t.Fatal(err)
			}
			if err := leaf.ParseFlags(args); err != nil {
				t.Fatal(err)
			}
			flag := leaf.Flags().Lookup(original.Name)
			if flag == nil || !flag.Changed {
				t.Fatalf("--%s was not applied", original.Name)
			}
			actual := flag.Value.String()
			expected, _, err := New().Find([]string{"download"})
			if err != nil {
				t.Fatal(err)
			}
			if err := expected.ParseFlags([]string{"--" + original.Name + "=" + value}); err != nil {
				t.Fatal(err)
			}
			if want := expected.Flags().Lookup(original.Name).Value.String(); actual != want {
				t.Fatalf("--%s: got %q, want %q", original.Name, actual, want)
			}
		})
	}
	t.Logf("Verified %d download and inherited flags", len(fields))
}
