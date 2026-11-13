package tui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/kballard/go-shellquote"
	"golang.org/x/text/unicode/norm"
)

func pathModel(t *testing.T, kind string, multiple bool, initial string) Model {
	t.Helper()
	root, action := fixture()
	m := New(Catalog(root), nil, "personal", context.Background())
	m.open(action)
	field := m.form.field("output")
	field.PathKind, field.PathMultiple, field.Multi, field.Value = kind, multiple, multiple, initial
	if multiple {
		field.Type = "stringSlice"
	}
	focusField(&m, "output")
	return m
}

func openFallback(t *testing.T, m Model) Model {
	t.Helper()
	m.choosePath = func(context.Context, pathRequest) ([]string, error) { return nil, errPathDialogUnavailable }
	next, command := m.Update(key(tea.KeyEnter, ""))
	if command == nil {
		t.Fatal("Enter must open the path chooser")
	}
	next, command = next.(Model).Update(command())
	if command == nil {
		t.Fatal("an unavailable system dialog should open the terminal browser")
	}
	return update(next.(Model), command())
}

func TestPathDialogAppliesTheSelectedFolderWithoutRunningACommand(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "آرشیو, Book Club $(literal)")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	m := pathModel(t, "directory", false, "data")
	m.choosePath = func(_ context.Context, request pathRequest) ([]string, error) {
		if request.Kind != "directory" || request.Title != "Save to folder" || request.Initial != "data" {
			t.Errorf("wrong dialog request: %+v", request)
		}
		return []string{dir}, nil
	}
	next, command := m.Update(key(tea.KeyEnter, ""))
	m = update(next.(Model), command())
	set(&m.form, "chat", "me")
	m.prepareReview()
	if m.pathDialog != nil || m.editing || m.screen != review || !slices.Contains(m.plan.Args, "--output="+dir) {
		t.Fatal("the chosen folder did not reach review as a literal argument")
	}
	if m.form.Action.Command.Flags().Lookup("output").Changed {
		t.Fatal("choosing a path mutated the running CLI flags")
	}
}

func TestCancellingPathDialogPreservesTheActiveEditorAndIgnoresLateResults(t *testing.T) {
	m := pathModel(t, "directory", false, "data")
	m = clickChoiceText(t, m, "[ Type path ]")
	m = update(m, key('u', "", tea.ModCtrl))
	m = update(m, tea.PasteMsg{Content: "unsaved folder"})
	m = update(m, key('o', "", tea.ModAlt))
	dialog := m.pathDialog
	if dialog == nil || dialog.request.Initial != "unsaved folder" {
		t.Fatal("Browse should start from the path being edited")
	}
	m = update(m, key(tea.KeyEscape, ""))
	if dialog.ctx.Err() == nil || !m.editing || m.editor.Value() != "unsaved folder" || m.form.field("output").Value != "data" {
		t.Fatal("cancelling changed the path or lost the active text edit")
	}
	m = update(m, pathSelectedMsg{dialog: dialog, paths: []string{t.TempDir()}})
	if m.form.field("output").Value != "data" {
		t.Fatal("a dismissed chooser changed a later form")
	}
	m = update(m, key('o', "", tea.ModAlt))
	m = update(m, pathSelectedMsg{dialog: m.pathDialog}) // Cancel in macOS.
	if !m.editing || m.editor.Value() != "unsaved folder" || m.err != "" {
		t.Fatal("system cancellation should preserve the unsaved text without an error")
	}
}

func TestPathDialogFailureIsVisibleAndLeavesTheOldPath(t *testing.T) {
	m := pathModel(t, "directory", false, "data")
	m = update(m, key(tea.KeyEnter, ""))
	m = update(m, pathSelectedMsg{dialog: m.pathDialog, err: errors.New("chooser could not open")})
	if m.pathDialog != nil || m.form.field("output").Value != "data" || !strings.Contains(ansi.Strip(m.View().Content), "chooser could not open") {
		t.Fatal("dialog failures must remain visible without replacing the saved path")
	}
}

func TestMultiplePathsPreserveCommasSpacesAndShellCharacters(t *testing.T) {
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "a, b.json"), filepath.Join(dir, "آرشیو ' $(literal)\n.json")}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := decodeDialogPaths(strings.Join(paths, "\x00") + "\x00\n"); !slices.Equal(got, paths) {
		t.Fatalf("system dialog corrupted filenames: %q", got)
	}
	for _, positional := range []bool{false, true} {
		m := pathModel(t, "open", true, "")
		field := m.form.field("output")
		if positional {
			field.Type = "arguments"
		}
		m = update(m, key(tea.KeyEnter, ""))
		m = update(m, pathSelectedMsg{dialog: m.pathDialog, paths: paths})
		var got []string
		if positional {
			got, _ = shellquote.Split(m.form.field("output").Value)
		} else {
			got, _ = readCSV(m.form.field("output").Value)
		}
		if !slices.Equal(got, paths) {
			t.Fatalf("selected filenames changed: %q", got)
		}
	}
}

func TestPathValidationAndStartingLocationDoNotCreateDestinations(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "input.json")
	if err := os.WriteFile(file, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		kind, path string
		valid      bool
	}{{"directory", dir, true}, {"directory", file, false}, {"open", file, true}, {"open", dir, false}, {"save", filepath.Join(dir, "new.json"), true}, {"save", filepath.Join(dir, "missing", "new.json"), false}, {"save", dir, false}, {"any", dir, true}} {
		if err := checkPaths(pathRequest{Kind: test.kind}, []string{test.path}); (err == nil) != test.valid {
			t.Fatalf("%s %s: %v", test.kind, test.path, err)
		}
	}
	location, name, err := pathLocation(filepath.Join(dir, "missing", "new.json"))
	if err != nil || location != dir || name != "new.json" {
		t.Fatalf("new destinations should start in the nearest existing folder: %q %q %v", location, name, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "missing")); !os.IsNotExist(err) {
		t.Fatal("browsing created the destination before review")
	}
}

func TestTerminalPathBrowserNavigatesSelectsAndKeepsChecksOutsideSearch(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "folder")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"first.json", "second.json", ".hidden.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	m := openFallback(t, pathModel(t, "open", true, csvString([]string{dir})))
	if got := len(m.pathDialog.browser.filtered()); got != 3 {
		t.Fatalf("wrong visible entries: %d", got)
	}
	next, command := m.Update(key(tea.KeyEnter, "")) // First row is a folder.
	m = update(next.(Model), command())
	if m.pathDialog.browser.directory != child {
		t.Fatal("Enter did not open the selected folder")
	}
	next, command = m.Update(key(tea.KeyLeft, ""))
	m = update(next.(Model), command())
	m = update(m, key(tea.KeyDown, ""))
	m = update(m, key(' ', " "))
	m = update(m, tea.PasteMsg{Content: "second"})
	m = update(m, key(tea.KeyDown, "")) // Leave search.
	m = update(m, key(' ', " "))
	m.pathAction("path-choose")
	got, _ := readCSV(m.form.field("output").Value)
	if !slices.Equal(got, []string{filepath.Join(dir, "first.json"), filepath.Join(dir, "second.json")}) || m.pathDialog != nil {
		t.Fatalf("filtering lost checked files: %q", got)
	}
}

func TestPathControlsAndBrowserFitSmallTerminalsAndSupportMouse(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "chosen"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, size := range [][2]int{{55, 20}, {80, 24}, {110, 32}, {120, 20}} {
		m := pathModel(t, "directory", false, dir)
		m.width, m.height = size[0], size[1]
		m.err = "A path validation error"
		view := ansi.Strip(m.View().Content)
		if !strings.Contains(view, m.err) || !strings.Contains(view, "[ Browse… ]") {
			t.Fatalf("path controls hid actions or errors at %v:\n%s", size, view)
		}
		m = clickChoiceText(t, m, "[ Browse… ]")
		if m.pathDialog == nil {
			t.Fatal("Browse did not open the chooser")
		}
		m = update(m, key(tea.KeyEscape, ""))
		m = openFallback(t, m)
		view = ansi.Strip(m.View().Content)
		if len(strings.Split(view, "\n")) > size[1] {
			t.Fatalf("browser height overflow at %v", size)
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("browser width overflow at %v", size)
			}
		}
		// Open with the keyboard so the async read can be delivered explicitly.
		next, command := m.Update(key(tea.KeyEnter, ""))
		m = update(next.(Model), command())
		m = clickChoiceText(t, m, "[ Use this folder ]")
		if m.pathDialog != nil || m.form.field("output").Value != filepath.Join(dir, "chosen") {
			t.Fatal("the mouse confirmation chose the wrong folder")
		}
	}
}

func TestMacPathDialogScriptCompilesWithoutOpeningAWindow(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("The system dialog script is available on macOS")
	}
	command := exec.Command("/usr/bin/osacompile", "-l", "JavaScript", "-o", filepath.Join(t.TempDir(), "chooser.scpt"), "-")
	command.Stdin = strings.NewReader(macPathDialogScript)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("invalid system chooser script: %v\n%s", err, output)
	}
}

func TestMacDialogBridgePreservesAPathWithoutOpeningAWindow(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("The Cocoa bridge is available on macOS")
	}
	path := filepath.Join(t.TempDir(), "آرشیو, $(literal)\n")
	script := `ObjC.import('AppKit');
function run(argv) {
    if (!$.NSOpenPanel.instancesRespondToSelector('setCanChooseDirectories:') ||
        !$.NSOpenPanel.instancesRespondToSelector('setAllowsMultipleSelection:') ||
        !$.NSSavePanel.instancesRespondToSelector('setNameFieldStringValue:')) throw Error('Missing chooser API');
    return ObjC.unwrap($.NSURL.fileURLWithPath(argv[0]).path) + '\u0000';
}`
	command := exec.Command("/usr/bin/osascript", "-l", "JavaScript", "-", path)
	command.Stdin = strings.NewReader(script)
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	// Cocoa returns decomposed Unicode names on macOS; compare canonical forms.
	if paths := decodeDialogPaths(string(output)); len(paths) != 1 || norm.NFC.String(paths[0]) != norm.NFC.String(path) {
		t.Fatalf("Cocoa bridge changed the selected path: %q", paths)
	}
}

func TestTerminalSaveBrowserChoosesANewFilenameWithoutWritingIt(t *testing.T) {
	dir := t.TempDir()
	m := openFallback(t, pathModel(t, "save", false, filepath.Join(dir, "original.json")))
	m = update(m, key(tea.KeyF2, ""))
	m = update(m, key('u', "", tea.ModCtrl))
	m = update(m, tea.PasteMsg{Content: "new, آرشیو.json"})
	m = update(m, key(tea.KeyEnter, ""))
	m = clickChoiceText(t, m, "[ Save here ]")
	path := filepath.Join(dir, "new, آرشیو.json")
	if m.pathDialog != nil || m.form.field("output").Value != path {
		t.Fatal("the Save browser did not use the entered filename")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("selecting a save destination wrote the output before review")
	}
}

func TestPathBrowserDiscardsStaleDirectoryReadsAndShowsReadErrors(t *testing.T) {
	m := openFallback(t, pathModel(t, "directory", false, t.TempDir()))
	dialog := m.pathDialog
	command := m.readPathDirectory(filepath.Join(t.TempDir(), "missing"))
	current := dialog.browser.revision
	m = update(m, pathDirectoryMsg{dialog: dialog, revision: current - 1, entries: []pathEntry{{name: "stale"}}})
	if !dialog.browser.loading || len(dialog.browser.entries) != 0 {
		t.Fatal("a stale folder scan replaced the current browser")
	}
	m = update(m, command())
	if dialog.browser.loading || dialog.browser.err == "" || !strings.Contains(ansi.Strip(m.View().Content), clip(dialog.browser.err, m.width-4)) {
		t.Fatal("a failed folder read must display its cause")
	}
}
