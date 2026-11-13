package cmd

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mo1ein/TelegramArchive/internal/tui"
	"github.com/spf13/viper"
	"go.etcd.io/bbolt"
)

func TestBackupTimeFiltersAreEssentialFields(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	for _, action := range tui.Catalog(New()) {
		if action.Group == "Quick start" || !slices.Contains([]string{"archive", "account", "sync", "cron add"}, strings.Join(action.Path, " ")) {
			continue
		}
		found := map[string]bool{}
		for _, field := range tui.NewForm(action, nil).Fields {
			if field.Name == "since" || field.Name == "until" {
				found[field.Name] = true
				if field.Advanced {
					t.Errorf("%s / %s is hidden under More options", action.Title, field.Name)
				}
			}
		}
		if len(found) != 2 {
			t.Errorf("%s is missing a since or until field", action.Title)
		}
	}
}

func TestChatCategoriesEnterTogglesAndLeftAccepts(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	var action tui.Action
	for _, candidate := range tui.Catalog(New()) {
		if strings.Join(candidate.Path, " ") == "archive" && candidate.Group != "Quick start" {
			action = candidate
			break
		}
	}
	action.Group = "Quick start"
	m := tui.New([]tui.Action{action}, nil, "personal", context.Background(), tui.WithSavedAccounts([]string{"personal"}))
	update := func(msg tea.Msg) { next, _ := m.Update(msg); m = next.(tui.Model) }
	update(tea.KeyPressMsg{Code: tea.KeyEnter})
	for range 3 {
		update(tea.KeyPressMsg{Code: tea.KeyTab})
	}
	update(tea.KeyPressMsg{Code: tea.KeyEnter}) // Open Chat categories.
	update(tea.KeyPressMsg{Code: tea.KeyDown})
	update(tea.KeyPressMsg{Code: tea.KeyEnter}) // Uncheck Bots; keep choosing.
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "[ ] Bots") || !strings.Contains(view, "[Done]") {
		t.Fatalf("Enter should toggle the category and keep the list open:\n%s", view)
	}
	update(tea.KeyPressMsg{Code: tea.KeyDown})
	update(tea.KeyPressMsg{Code: tea.KeyEnter}) // Uncheck Groups.
	update(tea.KeyPressMsg{Code: tea.KeyEnter}) // Check Groups again.
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "[✓] Groups") || !strings.Contains(view, "[ ] Bots") {
		t.Fatalf("repeated Enter presses should toggle only the highlighted item:\n%s", view)
	}
	update(tea.KeyPressMsg{Code: tea.KeyLeft})
	view = ansi.Strip(m.View().Content)
	if strings.Contains(view, "[Done]") || !strings.Contains(view, "Essentials") || !strings.Contains(view, "Personal, Groups, Channels") {
		t.Fatalf("Left should close the list and accept its choices in the form:\n%s", view)
	}
	update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if strings.Contains(ansi.Strip(m.View().Content), "[Done]") {
		t.Fatal("repeating Left after accepting must not reopen the selector")
	}
	update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "--types=personal,groups,channels") {
		t.Fatalf("the accepted categories did not reach the actual archive command:\n%s", view)
	}
}

func TestArchiveMediaCanBeSelectedWithOnlyTheMouse(t *testing.T) {
	for _, size := range [][2]int{{55, 20}, {80, 24}, {110, 32}} {
		t.Run(strings.Join([]string{strconv.Itoa(size[0]), strconv.Itoa(size[1])}, "x"), func(t *testing.T) {
			viper.Reset()
			defer viper.Reset()
			var action tui.Action
			for _, candidate := range tui.Catalog(New()) {
				if strings.Join(candidate.Path, " ") == "archive" && candidate.Group != "Quick start" {
					action = candidate
					break
				}
			}
			action.Group = "Quick start"
			m := tui.New([]tui.Action{action}, nil, "personal", context.Background(), tui.WithSavedAccounts([]string{"personal"}))
			update := func(msg tea.Msg) { next, _ := m.Update(msg); m = next.(tui.Model) }
			update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			locate := func(text string) (int, int, bool) {
				for y, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
					if index := strings.Index(line, text); index >= 0 {
						return ansi.StringWidth(line[:index]), y, true
					}
				}
				return 0, 0, false
			}
			click := func(text string) {
				t.Helper()
				x, y, ok := locate(text)
				if !ok {
					t.Fatalf("mouse target %q is missing:\n%s", text, ansi.Strip(m.View().Content))
				}
				update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
				update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
			}
			click("[ Open ]")
			click("Media to")
			click("[Clear]")
			for _, name := range []string{"Photos", "Videos", "Documents"} {
				found := false
				for range 12 {
					if _, _, ok := locate("[ ] " + name); ok {
						click("[ ] " + name)
						found = true
						break
					}
					x, y, ok := locate("⌕ ")
					if !ok {
						t.Fatal("the selector's scroll area is missing")
					}
					update(tea.MouseWheelMsg{X: x, Y: y, Button: tea.MouseWheelDown})
				}
				if !found {
					t.Fatalf("mouse scrolling never made %q selectable:\n%s", name, ansi.Strip(m.View().Content))
				}
			}
			click("[Done]")
			click("[ Review ]")
			view := ansi.Strip(m.View().Content)
			for _, name := range []string{"Photos", "Videos", "Documents"} {
				if !strings.Contains(view, name) {
					t.Fatalf("the mouse-only selection lost %q in review:\n%s", name, view)
				}
			}
		})
	}
}

func TestArchiveMediaSelectionsRemainVisible(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	var action tui.Action
	for _, candidate := range tui.Catalog(New()) {
		if strings.Join(candidate.Path, " ") == "archive" && candidate.Group != "Quick start" {
			action = candidate
			break
		}
	}
	if action.Command == nil {
		t.Fatal("archive action is missing")
	}
	// Exercise the public UI through the actual command metadata, with no
	// Telegram calls or flag mutations. A one-item dashboard opens that action.
	action.Group = "Quick start"
	m := tui.New([]tui.Action{action}, nil, "personal", context.Background(), tui.WithSavedAccounts([]string{"personal"}))
	update := func(msg tea.Msg) { next, _ := m.Update(msg); m = next.(tui.Model) }
	update(tea.KeyPressMsg{Code: tea.KeyEnter})
	for y, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
		if strings.Contains(line, "Media to download") {
			update(tea.MouseClickMsg{X: 40, Y: y, Button: tea.MouseLeft})
			break
		}
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "[All]") || !strings.Contains(view, "[Clear]") {
		t.Fatalf("archive media did not open a multi-select:\n%s", view)
	}
	update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	choices := action.Command.Flags().Lookup("media").Annotations["ta:choices"]
	for _, value := range []string{"photos", "videos", "documents"} {
		index := slices.Index(choices, value)
		if index < 0 {
			t.Fatalf("missing supported media category %q", value)
		}
		update(tea.KeyPressMsg{Code: tea.KeyHome})
		for range index {
			update(tea.KeyPressMsg{Code: tea.KeyDown})
		}
		update(tea.KeyPressMsg{Code: ' ', Text: " "})
	}
	update(tea.KeyPressMsg{Code: tea.KeyLeft})
	view = ansi.Strip(m.View().Content)
	for _, name := range []string{"Photos", "Videos", "Documents"} {
		if !strings.Contains(view, name) {
			t.Fatalf("selected media category %q disappeared after applying:\n%s", name, view)
		}
	}
	update(tea.KeyPressMsg{Code: tea.KeyEnter})
	update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	view = ansi.Strip(m.View().Content)
	for _, name := range []string{"[✓] Photos", "[✓] Videos", "[✓] Documents"} {
		if !strings.Contains(view, name) {
			t.Fatalf("the selected-only list lost %q:\n%s", name, view)
		}
	}
	update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "--media=photos,videos,documents") {
		t.Fatalf("review did not preserve all selected media in the actual command:\n%s", view)
	}
}

func TestAllBackupFormsExposeEveryKnownMultiSelect(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	counts := map[string]int{}
	for _, action := range tui.Catalog(New()) {
		path := strings.Join(action.Path, " ")
		for _, field := range tui.NewForm(action, nil).Fields {
			if field.Name != "media" && field.Name != "types" && field.Name != "include" && field.Name != "account-include" {
				continue
			}
			if !slices.Contains([]string{"archive", "account", "sync", "cron add"}, path) {
				continue
			}
			if !field.Multi || len(field.Choices) == 0 {
				t.Fatalf("%s (%s) / %s is missing its multi-select options", path, action.Title, field.Name)
			}
			counts[path]++
		}
	}
	for _, path := range []string{"archive", "account", "sync", "cron add"} {
		if counts[path] == 0 {
			t.Fatalf("the multi-select audit did not reach %s", path)
		}
	}
}

func TestTUIBackupListFieldsExposeCheckboxChoices(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	for _, action := range tui.Catalog(New()) {
		if len(action.Path) != 1 || action.Path[0] != "archive" {
			continue
		}
		for _, field := range tui.NewForm(action, nil).Fields {
			if field.Name == "media" && (!field.Multi || !slices.Contains(field.Choices, "photos") || !slices.Contains(field.Choices, "videos")) {
				t.Fatal("media choices remain a free-text list")
			}
			if field.Name == "types" && (!field.Multi || !slices.Contains(field.Choices, "private_channels")) {
				t.Fatal("chat categories do not expose every supported choice")
			}
		}
	}
}

func TestTUIPathFieldsUseTheRightDialogWithoutTreatingPeersAsPaths(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	want := map[string]string{
		"archive/output": "directory", "account/output": "directory", "sync/output": "directory", "cron add/output": "directory",
		"login/desktop": "directory", "render/arguments": "directory", "stats/arguments": "open",
		"chat export/output": "save", "chat users/output": "save", "backup/dst": "save", "recover/file": "open",
		"download/dir": "directory", "download/file": "open", "upload/path": "any",
		"download/url": "", "upload/to": "", "migrate/to": "",
	}
	for _, action := range tui.Catalog(New()) {
		if action.Group == "Quick start" {
			continue
		}
		for _, field := range tui.NewForm(action, nil).Fields {
			key := strings.Join(action.Path, " ") + "/" + field.Name
			if kind, ok := want[key]; ok {
				if field.PathKind != kind {
					t.Errorf("%s: expected %q dialog, got %q", key, kind, field.PathKind)
				}
				if (key == "stats/arguments" || key == "download/file" || key == "upload/path") && !field.PathMultiple {
					t.Errorf("%s must accept several paths", key)
				}
				delete(want, key)
			}
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing expected path fields: %v", want)
	}
}

func TestTUIAccountDiscoveryReadsMetadataWithoutCreatingOrOpeningSessions(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"work", "personal", ".ignored"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("synthetic session placeholder"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	names, err := tuiSavedAccounts(map[string]string{"type": "bolt", "path": dir})
	if err != nil || !slices.Equal(names, []string{"personal", "work"}) {
		t.Fatalf("account names require valid session databases: %v %v", names, err)
	}
	missing := filepath.Join(dir, "missing")
	if _, err := tuiSavedAccounts(map[string]string{"type": "bolt", "path": missing}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("opening the picker created a new session directory")
	}
	file := filepath.Join(dir, "sessions.json")
	if err := os.WriteFile(file, []byte(`{"work":{},"personal":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	names, err = tuiSavedAccounts(map[string]string{"type": "file", "path": file})
	if err != nil || !slices.Equal(names, []string{"personal", "work"}) {
		t.Fatalf("file storage discovery: %v %v", names, err)
	}
	legacy := filepath.Join(dir, "legacy.db")
	db, err := bbolt.Open(legacy, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Update(func(tx *bbolt.Tx) error { _, err := tx.CreateBucket([]byte("work")); return err })
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	names, err = tuiSavedAccounts(map[string]string{"type": "legacy", "path": legacy})
	if err != nil || !slices.Equal(names, []string{"work"}) {
		t.Fatalf("legacy account discovery: %v %v", names, err)
	}
}

func TestMultiAccountPlansParseThroughTheRealBackupCommands(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	for _, action := range tui.Catalog(New()) {
		path := strings.Join(action.Path, " ")
		if action.Group == "Quick start" || !slices.Contains([]string{"archive", "account", "sync"}, path) {
			continue
		}
		form := tui.NewForm(action, nil)
		for i := range form.Fields {
			field := &form.Fields[i]
			if field.Name == "ns" {
				field.Type, field.Multi, field.Dirty, field.Value = "accounts", true, true, "personal,work"
				field.Choices = []string{"personal", "work"}
			}
		}
		plan, err := form.Plan(nil)
		if err != nil || len(plan.Tasks) != 2 {
			t.Fatalf("%s could not plan both accounts: %v", path, err)
		}
		for _, task := range plan.Tasks {
			root := New()
			leaf, args, err := root.Find(task.Args)
			if err != nil {
				t.Fatal(err)
			}
			if err := leaf.ParseFlags(args); err != nil {
				t.Fatalf("%s: %v", path, err)
			}
			namespace, _ := leaf.Flags().GetString("ns")
			output, _ := leaf.Flags().GetString("output")
			if namespace != task.Account || output != task.Directory {
				t.Fatalf("%s runs with the wrong account or folder: %v", path, task.Args)
			}
		}
	}
}
