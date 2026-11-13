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
	for _, path := range []string{"archive", "account", "sync", "cron add", "cron list", "cron logs", "cron remove", "cron run", "schedule", "login", "accounts", "download", "upload", "forward", "chat ls", "chat export", "chat users", "render", "stats", "backup", "recover", "migrate", "extension install", "extension list", "extension remove", "extension upgrade", "completion bash", "completion zsh", "completion fish", "completion powershell", "version"} {
		if !found[path] {
			t.Errorf("missing %s", path)
		}
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
