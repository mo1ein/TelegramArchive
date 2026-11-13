package tui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
)

func fixture() (*cobra.Command, Action) {
	root := &cobra.Command{Use: "ta"}
	root.PersistentFlags().String("ns", "default", "saved account")
	root.PersistentFlags().String("proxy", "", "proxy address")
	root.PersistentFlags().StringToString("storage", map[string]string{"type": "bolt", "path": "data"}, "storage options [legacy,bolt]")
	command := &cobra.Command{Use: "archive", Short: "Back up chats", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error { panic("must never execute") }}
	command.Flags().StringSlice("chat", nil, "chats")
	command.Flags().Bool("all", false, "all chats")
	command.Flags().Bool("resume", true, "resume")
	command.Flags().String("format", "json", "format")
	command.Flags().String("output", "data", "destination")
	command.Flags().Duration("every", 0, "interval")
	command.Flags().Int("limit", 2, "concurrency")
	command.Flags().String("since", "", "date")
	command.Flags().StringArray("from", nil, "sources")
	command.Flags().String("private", "", "internal")
	_ = command.Flags().MarkHidden("private")
	root.AddCommand(command)
	return root, Action{Title: "archive", Description: command.Short, Path: []string{"archive"}, Group: "Backups", Command: command}
}

func set(form *Form, name, value string) {
	f := form.field(name)
	if f == nil {
		panic(name)
	}
	f.Value, f.Dirty = value, true
}

func key(code rune, text string, mods ...tea.KeyMod) tea.KeyPressMsg {
	msg := tea.KeyPressMsg{Code: code, Text: text}
	if len(mods) > 0 {
		msg.Mod = mods[0]
	}
	return msg
}

func update(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func TestPlanPreservesArgumentsWithoutMutatingCobra(t *testing.T) {
	root, action := fixture()
	form := NewForm(action, map[string]string{"ns": "work"})
	set(&form, "chat", `"Book Club, Friends",telegram`)
	set(&form, "output", `exports/hello world/$(touch nope);literal`)
	set(&form, "resume", "false")
	set(&form, "from", `"/path/with,a comma.json",second.json`)
	set(&form, "storage", "type=bolt,path=/tmp/sessions")
	plan, err := form.Plan([]string{"--proxy=socks5://user:password@example.invalid:1080", "--ns=old"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(plan.Args, "--output=exports/hello world/$(touch nope);literal") || !slices.Contains(plan.Args, "--resume=false") {
		t.Fatal(plan.Args)
	}
	if !slices.Contains(plan.Args, "--from=/path/with,a comma.json") || !slices.Contains(plan.Args, "--from=second.json") {
		t.Fatal(plan.Args)
	}
	if strings.Contains(plan.Preview(), "password") || strings.Contains(plan.Preview(), "user:") {
		t.Fatal("credentials in preview:", plan.Preview())
	}
	if slices.Contains(plan.Args, "--ns=old") || strings.Count(plan.Preview(), "--ns=") != 1 {
		t.Fatal("global overrides should replace startup arguments", plan.Args)
	}
	if got, _ := action.Command.Flags().GetStringSlice("chat"); len(got) != 0 {
		t.Fatal("form changed actual flags", got)
	}
	if root.PersistentFlags().Lookup("ns").Changed {
		t.Fatal("form changed the running namespace")
	}
	if form.field("private") != nil {
		t.Fatal("hidden flag exposed")
	}

	// Parse the generated arguments through the same Cobra command definitions.
	clone, _ := fixture()
	leaf, args, err := clone.Find(plan.Args)
	if err != nil {
		t.Fatal(err)
	}
	if err := leaf.ParseFlags(args); err != nil {
		t.Fatal(err)
	}
	chats, _ := leaf.Flags().GetStringSlice("chat")
	if !reflect.DeepEqual(chats, []string{"Book Club, Friends", "telegram"}) {
		t.Fatal(chats)
	}
	sources, _ := leaf.Flags().GetStringArray("from")
	if !reflect.DeepEqual(sources, []string{"/path/with,a comma.json", "second.json"}) {
		t.Fatal(sources)
	}
}

func TestPlanValidatesBeforeExecution(t *testing.T) {
	for _, tc := range []struct{ name, value, want string }{
		{"every", "tomorrow", "every"},
		{"limit", "many", "limit"},
		{"format", "pdf", "choose"},
		{"since", "yesterday", "YYYY-MM-DD"},
		{"chat", `"unfinished`, "chat"},
		{"arguments", `"unfinished`, "arguments"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, action := fixture()
			form := NewForm(action, nil)
			set(&form, tc.name, tc.value)
			if _, err := form.Plan(nil); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v", err)
			}
		})
	}
	_, action := fixture()
	form := NewForm(action, nil)
	set(&form, "all", "true")
	set(&form, "chat", "me")
	if _, err := form.Plan(nil); err == nil {
		t.Fatal("accepted conflicting chat selection")
	}
	set(&form, "chat", "")
	if _, err := form.Plan(nil); err != nil {
		t.Fatal(err)
	}
}

func TestRequiredArgumentsAndPassthrough(t *testing.T) {
	for _, tc := range []struct {
		path, use, arguments string
		raw                  bool
		want                 []string
	}{
		{"stats", "stats FILE...", `'a file.json' --leading-dash.json`, false, []string{"stats", "--", "a file.json", "--leading-dash.json"}},
		{"schedule", "schedule", `--all --format both`, false, []string{"schedule", "--", "--all", "--format", "both"}},
		{"my-extension", "my-extension", `--custom 'hello world'`, true, []string{"my-extension", "--custom", "hello world"}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			command := &cobra.Command{Use: tc.use, Args: cobra.MinimumNArgs(1), DisableFlagParsing: tc.raw}
			form := NewForm(Action{Path: []string{tc.path}, Command: command}, nil)
			if _, err := form.Plan(nil); err == nil {
				t.Fatal("missing arguments accepted")
			}
			set(&form, "arguments", tc.arguments)
			plan, err := form.Plan(nil)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(plan.Args, tc.want) {
				t.Fatalf("%v != %v", plan.Args, tc.want)
			}
		})
	}
	_, action := fixture()
	action.Command.MarkFlagRequired("output")
	form := NewForm(action, nil)
	if _, err := form.Plan(nil); err == nil {
		t.Fatal("required flag must be explicitly supplied")
	}
}

func TestModelSearchEditReviewAndReturn(t *testing.T) {
	root, _ := fixture()
	m := New(Catalog(root), nil, "work", context.Background())
	runs := 0
	m.run = func(_ context.Context, plan Plan) tea.Cmd {
		runs++
		if !slices.Contains(plan.Args, "--ns=work") {
			t.Error(plan.Args)
		}
		return func() tea.Msg { return finishedMsg{elapsed: time.Second} }
	}
	m = update(m, key('/', "/"))
	m = update(m, tea.PasteMsg{Content: "BACK UP"})
	if len(m.filteredActions()) != 1 {
		t.Fatal(m.filteredActions())
	}
	m = update(m, key(tea.KeyEnter, "")) // Finish search.
	m = update(m, key(tea.KeyEnter, "")) // Open command.
	if m.screen != configure || runs != 0 {
		t.Fatal("opened command ran unexpectedly")
	}
	m.focus = slices.Index(m.visibleFields(), slices.IndexFunc(m.form.Fields, func(f Field) bool { return f.Name == "output" }))
	m = clickChoiceText(t, m, "[ Type path ]")
	m = update(m, key('u', "", tea.ModCtrl))
	m = update(m, tea.PasteMsg{Content: "آرشیو/Book Club"})
	m = update(m, key(tea.KeyEnter, ""))
	if m.form.field("output").Value != "آرشیو/Book Club" {
		t.Fatal(m.form.field("output"))
	}
	m = update(m, key('r', "", tea.ModCtrl))
	if m.screen != review || runs != 0 {
		t.Fatal("review must not execute the command")
	}
	m = update(m, key(tea.KeyEscape, ""))
	if m.form.field("output").Value != "آرشیو/Book Club" {
		t.Fatal("editing lost values")
	}
	m = update(m, key('r', "", tea.ModCtrl))
	next, command := m.Update(key(tea.KeyEnter, ""))
	m = next.(Model)
	if runs != 1 || command == nil {
		t.Fatal("review did not dispatch")
	}
	m = update(m, command())
	if m.screen != result {
		t.Fatal(m.screen)
	}
	m = update(m, key('e', "e"))
	if m.screen != configure {
		t.Fatal(m.screen)
	}
}

func TestSearchNoMatchesAndAdvancedOptions(t *testing.T) {
	root, action := fixture()
	m := New(Catalog(root), nil, "default", context.Background())
	m = update(m, key('/', "/"))
	m = update(m, tea.PasteMsg{Content: "no such command"})
	m = update(m, key(tea.KeyEnter, ""))
	m = update(m, key(tea.KeyEnter, ""))
	if m.screen != dashboard {
		t.Fatal(m.screen)
	}
	m.open(action)
	m = update(m, key('f', "", tea.ModCtrl))
	m = update(m, tea.PasteMsg{Content: "proxy"})
	if fields := m.visibleFields(); len(fields) != 1 || m.form.Fields[fields[0]].Name != "proxy" {
		t.Fatal(fields)
	}
	m = update(m, key(tea.KeyEscape, ""))
	if m.fieldSearch != "" {
		t.Fatal("escape must clear search")
	}
	m = update(m, key('a', "", tea.ModCtrl))
	if !m.advanced {
		t.Fatal("advanced options not enabled")
	}
}

func TestSpaceTogglesSwitchesAndCyclesChoices(t *testing.T) {
	root, action := fixture()
	m := New(Catalog(root), nil, "default", context.Background())
	m.open(action)
	m.advanced = true // Resume is an advanced setting; its control still works.
	focus := func(name string) {
		m.focus = slices.Index(m.visibleFields(), slices.IndexFunc(m.form.Fields, func(f Field) bool { return f.Name == name }))
	}
	focus("resume")
	m = update(m, key(' ', " "))
	if m.form.field("resume").Value != "false" {
		t.Fatal("space did not toggle the switch")
	}
	focus("format")
	m = update(m, key(' ', " "))
	if m.form.field("format").Value != "html" {
		t.Fatal("space did not cycle choices")
	}
	m = update(m, key('d', "", tea.ModCtrl))
	if m.form.field("format").Value != "json" || m.form.field("format").Dirty {
		t.Fatal("field did not reset")
	}
	focus("output")
	m = clickChoiceText(t, m, "[ Type path ]")
	m = update(m, key(' ', " "))
	if string(m.input) != "data " {
		t.Fatal("space should type normally in an editor")
	}
}

func TestViewsFitTerminalAndKeepValidationVisible(t *testing.T) {
	root, action := fixture()
	for _, size := range [][2]int{{55, 20}, {80, 24}, {100, 32}, {160, 48}} {
		for _, state := range []screen{dashboard, configure, review, result} {
			m := New(Catalog(root), nil, "work", context.Background())
			m.open(action)
			m.width, m.height, m.screen = size[0], size[1], state
			m.err = "Validation needs attention"
			m.plan = Plan{Title: "archive", Args: []string{"archive", "--output=" + strings.Repeat("long/path/", 40)}}
			value := ansi.Strip(m.View().Content)
			lines := strings.Split(value, "\n")
			if len(lines) > size[1] {
				t.Fatalf("%v state %d: too many lines: %d", size, state, len(lines))
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > size[0] {
					t.Fatalf("%v state %d: too wide: %q", size, state, line)
				}
			}
			if state == configure && !strings.Contains(value, "Validation needs attention") {
				t.Fatalf("error clipped at %v:\n%s", size, value)
			}
		}
	}
}

func TestUniquePathCompletion(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "archive folder"), 0700); err != nil {
		t.Fatal(err)
	}
	m := Model{input: []rune(filepath.Join(dir, "arch"))}
	m.completePath()
	if string(m.input) != filepath.Join(dir, "archive folder")+string(filepath.Separator) {
		t.Fatal(string(m.input))
	}
}

func TestLoginPasscodeIsHiddenInFormAndPreview(t *testing.T) {
	command := &cobra.Command{Use: "login", Args: cobra.NoArgs}
	command.Flags().String("passcode", "", "")
	form := NewForm(Action{Path: []string{"login"}, Command: command}, nil)
	set(&form, "passcode", "synthetic private passcode")
	if !form.field("passcode").Secret || strings.Contains(form.field("passcode").Display(), "private") {
		t.Fatal("passcode was displayed")
	}
	plan, err := form.Plan(nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plan.Preview(), "synthetic private") {
		t.Fatal("passcode leaked in preview")
	}
	if !slices.Contains(plan.Args, "--passcode=synthetic private passcode") {
		t.Fatal("masking changed execution arguments")
	}
}

func TestJobKeepsOutputAndReturnsStatus(t *testing.T) {
	for _, failed := range []bool{false, true} {
		child := exec.Command(os.Args[0], "-test.run=^TestTerminalHelper$")
		child.Env = append(os.Environ(), "TA_TUI_HELPER=1")
		if failed {
			child.Env = append(child.Env, "TA_TUI_FAIL=1")
		}
		j, err := startJob(context.Background(), child, true, 80, 24)
		if err != nil {
			t.Fatal(err)
		}
		select {
		case <-j.doneC:
		case <-time.After(10 * time.Second):
			j.cancel()
			t.Fatal("worker did not finish")
		}
		j.mu.Lock()
		text := j.terminal.text()
		gotErr := j.err
		j.mu.Unlock()
		if (gotErr != nil) != failed || !strings.Contains(text, "synthetic command output") {
			t.Fatalf("err=%v output=%s", gotErr, text)
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := startJob(canceled, exec.Command(os.Args[0]), true, 80, 24); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestTerminalHelper(t *testing.T) {
	if os.Getenv("TA_TUI_HELPER") != "1" {
		return
	}
	_, _ = os.Stdout.WriteString("synthetic command output\n")
	if os.Getenv("TA_TUI_FAIL") == "1" {
		os.Exit(2)
	}
	os.Exit(0)
}
