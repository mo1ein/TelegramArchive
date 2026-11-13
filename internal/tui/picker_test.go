package tui

import (
	"context"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
)

func TestCustomListFieldsUseAnAddableMultiSelect(t *testing.T) {
	root, action := fixture()
	action.Command.Flags().StringSlice("extensions", nil, "extensions")
	m := New(Catalog(root), nil, "personal", context.Background())
	m.open(action)
	m.advanced = true
	focusField(&m, "extensions")
	m = update(m, key(tea.KeyEnter, ""))
	if m.picker == nil || !m.picker.multi || m.editing {
		t.Fatal("a list with custom values must open a multi-select with an Add control")
	}
	for _, value := range []string{"pdf", "mp4", "zip"} {
		m = update(m, tea.PasteMsg{Content: value})
		m = update(m, key(tea.KeyEnter, ""))
		if m.picker == nil || !m.picker.checked(value) {
			t.Fatalf("Enter did not add and select %q", value)
		}
	}
	m = clickChoiceText(t, m, "[Done]")
	if m.picker != nil || m.form.field("extensions").Value != "pdf,mp4,zip" {
		t.Fatal("the custom multi-select did not save every added item")
	}
	m = update(m, key(tea.KeyEnter, ""))
	m = update(m, key('d', "", tea.ModCtrl))
	m = clickChoiceText(t, m, "[Done]")
	if m.picker != nil || m.form.field("extensions").Value != "" {
		t.Fatal("an optional custom list should be clearable")
	}
}

func TestCustomListItemsCanBeAddedAndSavedByMouse(t *testing.T) {
	root, action := fixture()
	action.Command.Flags().StringSlice("extensions", []string{"mp4"}, "extensions")
	m := New(Catalog(root), nil, "personal", context.Background())
	m.open(action)
	m.advanced = true
	focusField(&m, "extensions")
	m = clickChoiceText(t, m, "extensions")
	// A partial match must still be addable as a distinct literal item.
	for _, item := range []string{"mp", "pdf"} {
		m = update(m, tea.PasteMsg{Content: item})
		m = clickChoiceText(t, m, "[Add]")
	}
	m = clickChoiceText(t, m, "[Done]")
	if got := m.form.field("extensions").Value; got != "mp4,mp,pdf" || m.picker != nil {
		t.Fatalf("the mouse-only Add and Done actions lost items: %q", got)
	}
}

func TestEnterTogglesAFilteredItemAndLeftSavesAllSelections(t *testing.T) {
	m := inlineModel(t, "personal", "work")
	m = update(m, key(tea.KeyEnter, ""))
	m = update(m, tea.PasteMsg{Content: "work"})
	m = update(m, key(tea.KeyEnter, ""))
	if m.picker == nil || !m.picker.checked("work") || !m.picker.checked("personal") {
		t.Fatal("Enter should toggle the filtered item without losing other choices")
	}
	m = update(m, key(tea.KeyEnter, ""))
	if m.picker == nil || m.picker.checked("work") {
		t.Fatal("Enter should uncheck the same highlighted item and stay open")
	}
	m = update(m, key(tea.KeyEnter, ""))
	m = update(m, key(tea.KeyLeft, ""))
	if m.picker != nil || m.form.field("ns").Value != "personal,work" || m.screen != configure {
		t.Fatal("Left must save selections beyond the filter and return to the form")
	}
}

func TestCustomStringArrayKeepsLiteralItemsAndSavesPendingInput(t *testing.T) {
	root, action := fixture()
	m := New(Catalog(root), nil, "personal", context.Background())
	m.open(action)
	m.advanced = true
	focusField(&m, "from")
	m = update(m, key(tea.KeyEnter, ""))
	m = update(m, tea.PasteMsg{Content: "Book Club, Friends"})
	m = update(m, key(tea.KeyEnter, ""))
	m = update(m, tea.PasteMsg{Content: "https://t.me/example/12"})
	m = update(m, key('r', "", tea.ModCtrl))
	if m.screen != review || m.picker != nil || !slices.Contains(m.plan.Args, "--from=Book Club, Friends") || !slices.Contains(m.plan.Args, "--from=https://t.me/example/12") {
		t.Fatal("the custom list split a literal comma or discarded the pending item")
	}
}

func TestRequiredCustomListCannotBeClearedAndSecretListsStayMasked(t *testing.T) {
	root, action := fixture()
	_ = action.Command.MarkFlagRequired("from")
	action.Command.Flags().StringSlice("tokens", nil, "private values")
	m := New(Catalog(root), nil, "personal", context.Background())
	m.open(action)
	m.advanced = true
	focusField(&m, "from")
	m = update(m, key(tea.KeyEnter, ""))
	m = clickChoiceText(t, m, "[Done]")
	if m.picker == nil || m.picker.err == "" {
		t.Fatal("a required custom list must explain why an empty selection cannot be applied")
	}
	m = update(m, key(tea.KeyEscape, ""))
	focusField(&m, "tokens")
	m = update(m, key(tea.KeyEnter, ""))
	m = update(m, tea.PasteMsg{Content: "synthetic-private-value"})
	if !m.editing || m.picker != nil || strings.Contains(m.View().Content, "synthetic-private-value") {
		t.Fatal("a secret list must use a masked editor")
	}
}

func TestCustomValuesAllAndNoneRemainLiteralItems(t *testing.T) {
	root, action := fixture()
	action.Command.Flags().StringSlice("include", nil, "custom extensions")
	m := New(Catalog(root), nil, "personal", context.Background())
	m.open(action)
	m.advanced = true
	focusField(&m, "include")
	m = update(m, key(tea.KeyEnter, ""))
	for _, value := range []string{"all", "none", "mp4"} {
		m = update(m, tea.PasteMsg{Content: value})
		m = update(m, key(tea.KeyEnter, ""))
	}
	m = update(m, key(tea.KeyHome, ""))
	m = update(m, key(' ', " ")) // Deselect the literal item "all".
	m = clickChoiceText(t, m, "[Done]")
	if got := m.form.field("include").Value; got != "none,mp4" {
		t.Fatalf("custom values were mistaken for reserved selectors: %q", got)
	}
}

func TestAccountPickerKeepsSelectionsAcrossFilteringAndCancelsWithoutSaving(t *testing.T) {
	root, action := fixture()
	m := New(Catalog(root), nil, "personal", context.Background(), WithSavedAccounts([]string{"personal", "work"}))
	m.open(action)
	field := m.form.field("ns")
	if !field.Multi || field.Advanced || field.Type != "accounts" {
		t.Fatal("backup accounts must be a visible multi-select")
	}
	m.openPicker(*field)
	m = update(m, key(tea.KeyDown, ""))
	m = update(m, key(' ', " "))
	m = update(m, key('p', "p"))
	if len(m.picker.filtered()) != 1 || !m.picker.selected["work"] {
		t.Fatal("filtering lost a checked account")
	}
	m = update(m, key(tea.KeyDown, "")) // Return from search to the first result.
	m = update(m, key(tea.KeyLeft, ""))
	selected, _ := readCSV(m.form.field("ns").Value)
	if !slices.Equal(selected, []string{"personal", "work"}) {
		t.Fatalf("incorrect account selection: %v", selected)
	}
	m.openPicker(*m.form.field("ns"))
	m = update(m, key(' ', " "))
	m = update(m, key(tea.KeyEscape, ""))
	if m.picker != nil || m.form.field("ns").Value != "personal,work" {
		t.Fatal("cancelling the picker changed the applied account selection")
	}
}

func TestMediaPickerExpandsAllAndKeepsNoneExclusive(t *testing.T) {
	root, action := fixture()
	action.Command.Flags().StringSlice("media", []string{"all"}, "media categories")
	action.Command.Flags().Lookup("media").Annotations = map[string][]string{"ta:choices": {"all", "none", "photos", "videos", "documents"}}
	m := New(Catalog(root), nil, "personal", context.Background())
	m.open(action)
	m.openPicker(*m.form.field("media"))
	if !m.picker.checked("photos") || !m.picker.checked("videos") {
		t.Fatal("All media should show individual categories checked")
	}
	m.picker.cursor = 3
	m = update(m, key(' ', " "))
	m = update(m, key(tea.KeyLeft, ""))
	if got := m.form.field("media").Value; got != "photos,documents" {
		t.Fatalf("unchecking one category should expand All: %s", got)
	}
	m.openPicker(*m.form.field("media"))
	m.picker.cursor = 1
	m = update(m, key(' ', " "))
	m = update(m, key(tea.KeyLeft, ""))
	if m.form.field("media").Value != "none" {
		t.Fatal("No media must exclude individual categories")
	}
	m.openPicker(*m.form.field("media"))
	m.picker.cursor = 2
	m = update(m, key(' ', " "))
	m = update(m, key(tea.KeyLeft, ""))
	if m.form.field("media").Value != "photos" {
		t.Fatal("choosing a category must clear No media")
	}
}

func TestSingleChoiceDropdownAndEmptyMultiSelection(t *testing.T) {
	root, action := fixture()
	m := New(Catalog(root), nil, "personal", context.Background())
	m.open(action)
	focusField(&m, "format")
	m = update(m, key(tea.KeyEnter, ""))
	if m.picker == nil || m.editing {
		t.Fatal("a choice should open a dropdown instead of a text editor")
	}
	m = update(m, key(tea.KeyDown, ""))
	m = update(m, key(tea.KeyEnter, ""))
	if m.form.field("format").Value != "html" {
		t.Fatal("dropdown did not apply the selected value")
	}
	m.openPicker(*m.form.field("ns"))
	m.activate("picker-clear")
	m = update(m, key(tea.KeyLeft, ""))
	if m.picker == nil || m.picker.err == "" {
		t.Fatal("empty account selection should remain open with an explanation")
	}
}

func TestChooseButtonAppliesHighlightedDropdownOption(t *testing.T) {
	root, action := fixture()
	m := New(Catalog(root), nil, "personal", context.Background())
	m.open(action)
	m.openPicker(*m.form.field("format"))
	m = update(m, key(tea.KeyDown, ""))
	m.activate("picker-apply")
	if m.form.field("format").Value != "html" {
		t.Fatal("Choose applied the old value instead of the highlighted option")
	}
}

func TestAccountNamesAllAndNoneAreOrdinaryAccountChoices(t *testing.T) {
	root, action := fixture()
	m := New(Catalog(root), nil, "all", context.Background(), WithSavedAccounts([]string{"all", "none", "work"}))
	m.open(action)
	m.openPicker(*m.form.field("ns"))
	m.selectAllChoices()
	m.applyPicker()
	plan, err := m.form.Plan(nil)
	if err != nil || len(plan.Tasks) != 3 {
		t.Fatalf("account names were mistaken for special category choices: %v", err)
	}
}

func TestAccountRefreshDoesNotChangeAnAlreadyReviewedPlan(t *testing.T) {
	m := batchModel(t)
	before := m.form.field("ns").Value
	m = update(m, accountsLoadedMsg{names: []string{"another-account"}})
	if m.form.field("ns").Value != before || len(m.plan.Tasks) != 3 {
		t.Fatal("background discovery changed the targets already shown in review")
	}
}

func TestPickerFitsTerminalsAndItsVisibleRowsAreClickable(t *testing.T) {
	root, action := fixture()
	for _, size := range [][2]int{{55, 20}, {80, 24}, {120, 36}} {
		m := New(Catalog(root), nil, "personal", context.Background(), WithSavedAccounts([]string{"personal", "work"}))
		m.width, m.height = size[0], size[1]
		m.open(action)
		m.openPicker(*m.form.field("ns"))
		m.picker.err = "Choose at least one option"
		view := ansi.Strip(m.View().Content)
		if len(strings.Split(view, "\n")) > size[1] || !strings.Contains(view, m.picker.err) {
			t.Fatalf("picker clipped its error at %v", size)
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("picker overflow at %v", size)
			}
		}
	}
	m := New(Catalog(root), nil, "personal", context.Background(), WithSavedAccounts([]string{"personal", "work"}))
	m.open(action)
	m.openPicker(*m.form.field("ns"))
	for y, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
		if offset := strings.Index(line, "[ ] work"); offset >= 0 {
			m = update(m, tea.MouseClickMsg{X: ansi.StringWidth(line[:offset]), Y: y, Button: tea.MouseLeft})
			if !m.picker.selected["work"] {
				t.Fatal("clicking a visible checkbox selected the wrong row")
			}
			return
		}
	}
	t.Fatal("work account was not visible")
}

func TestCheckedChatsBecomeNamedChoicesAndKeepTheirOriginalAccount(t *testing.T) {
	root, _ := fixture()
	m := New(Catalog(root), nil, "personal", context.Background(), WithSavedAccounts([]string{"personal", "work"}))
	m.accountSelection = []string{"personal", "work"}
	m.screen = result
	m.plan = Plan{Path: []string{"chat", "ls"}, Args: []string{"--ns=work", "chat", "ls"}}
	m.data = resultData{Kind: "chat ls", Items: []item{{ID: "42", Title: "First chat"}, {ID: "84", Title: "Second chat"}}}
	m = update(m, key(' ', " "))
	m = update(m, key(tea.KeyDown, ""))
	m = update(m, key(' ', " "))
	m = update(m, key('/', "/"))
	m = update(m, tea.PasteMsg{Content: "Second"})
	m = update(m, key(tea.KeyEnter, ""))
	m.activate("archive-selected")
	if m.screen != configure || m.form.field("chat").Value != "42,84" || m.form.field("ns").Value != "work" {
		t.Fatal("checked chats were lost or applied to another account")
	}
	m.openPicker(*m.form.field("chat"))
	if m.picker.label("42") != "First chat" || !m.picker.checked("42") || !m.picker.checked("84") {
		t.Fatal("loaded chats should be named checkbox options, not raw IDs")
	}
	m.picker = nil
	m.restoreList()
	if len(m.checkedResults()) != 2 || m.resultSearch != "Second" {
		t.Fatal("returning to the list discarded checks outside the filter")
	}
}

func TestAccountsBrowserCanPrepareSeveralBackupsWithoutRunningThem(t *testing.T) {
	root, _ := fixture()
	sync := &cobra.Command{Use: "sync", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error { panic("must not run") }}
	sync.Flags().Bool("everything", false, "entire account")
	sync.Flags().String("format", "json", "format")
	sync.Flags().String("output", "data", "destination")
	root.AddCommand(sync)
	m := New(Catalog(root), nil, "personal", context.Background(), WithSavedAccounts([]string{"personal", "work", "busy"}))
	m.screen = result
	m.data = resultData{Kind: "accounts", Items: []item{{ID: "personal", Status: "ready"}, {ID: "work", Status: "ready"}, {ID: "busy", Status: "busy"}}}
	m = update(m, key(' ', " "))
	m = update(m, key(tea.KeyDown, ""))
	m = update(m, key(' ', " "))
	m = update(m, key(tea.KeyDown, ""))
	m = update(m, key(' ', " "))
	if m.selectedResults["busy"] {
		t.Fatal("a busy account was selectable in the account browser")
	}
	m.activate("backup-selected")
	plan, err := m.form.Plan(nil)
	if err != nil || m.screen != configure || len(plan.Tasks) != 2 {
		t.Fatalf("account selection did not prepare two separate backups: %v", err)
	}
}
