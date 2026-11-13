package tui

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func datePickerModel(t *testing.T, name, value string) Model {
	t.Helper()
	root, action := fixture()
	action.Command.Flags().String("until", "", "exclusive date/time")
	m := New(Catalog(root), nil, "personal", t.Context())
	m.open(action)
	set(&m.form, name, value)
	focusField(&m, name)
	return m
}

func TestCalendarSelectsBothBackupDateFilters(t *testing.T) {
	for _, name := range []string{"since", "until"} {
		t.Run(name, func(t *testing.T) {
			m := datePickerModel(t, name, "2026-11-14")
			m = update(m, key(tea.KeyEnter, ""))
			if m.calendar == nil || m.editing || !m.inlineCalendar() {
				t.Fatal("Enter should open an inline calendar")
			}
			m = update(m, key(tea.KeyRight, ""))
			m = update(m, key(tea.KeyDown, ""))
			if m.form.field(name).Value != "2026-11-14" {
				t.Fatal("moving in the calendar changed the saved date")
			}
			m = update(m, key(tea.KeyEnter, ""))
			m = update(m, key('r', "", tea.ModCtrl))
			if m.calendar != nil || m.screen != review || !slices.Contains(m.plan.Args, "--"+name+"=2026-11-22") {
				t.Fatal("choosing a day did not reach the backup plan")
			}
		})
	}
}

func TestCalendarPagesMonthsWithoutSkippingShortMonths(t *testing.T) {
	for _, example := range []struct{ from, to string }{
		{"2028-01-31", "2028-02-29"},
		{"2026-01-31", "2026-02-28"},
		{"2026-12-31", "2027-01-31"},
	} {
		m := datePickerModel(t, "since", example.from)
		m = update(m, key(tea.KeyEnter, ""))
		m = update(m, key(tea.KeyPgDown, ""))
		m = update(m, key(tea.KeyEnter, ""))
		if got := m.form.field("since").Value; got != example.to {
			t.Fatalf("paging from %s: got %s, want %s", example.from, got, example.to)
		}
	}
	m := datePickerModel(t, "since", "2026-01-01")
	m = update(m, key(tea.KeyEnter, ""))
	m = update(m, key(tea.KeyLeft, ""))
	m = update(m, key(tea.KeyEscape, ""))
	if m.calendar != nil || m.form.field("since").Value != "2026-01-01" {
		t.Fatal("Esc should discard calendar navigation across a year")
	}
}

func TestCalendarMouseSelectsDaysAndControlsAtEverySupportedSize(t *testing.T) {
	for _, size := range [][2]int{{55, 20}, {80, 24}, {55, 25}, {110, 32}} {
		m := datePickerModel(t, "since", "2026-11-14")
		m.width, m.height = size[0], size[1]
		m = clickChoiceText(t, m, "[ Calendar ]")
		if m.calendar == nil {
			t.Fatalf("calendar control did not open at %v", size)
		}
		view := ansi.Strip(m.View().Content)
		if !strings.Contains(view, "Mo  Tu  We  Th  Fr  Sa  Su") || !strings.Contains(view, "November 2026") {
			t.Fatalf("calendar grid is missing at %v: %s", size, view)
		}
		for _, row := range strings.Split(view, "\n") {
			if ansi.StringWidth(row) > m.width {
				t.Fatalf("calendar overflow at %v: %q", size, row)
			}
		}
		if len(strings.Split(view, "\n")) > m.height {
			t.Fatalf("calendar exceeds terminal height at %v", size)
		}
		m = clickChoiceText(t, m, "›")
		if m.calendar.day.Format("2006-01-02") != "2026-12-14" {
			t.Fatal("mouse next-month control did not page the month")
		}
		b := m.calendarBounds()
		// December 2026 starts on Tuesday: the first Thursday is December 3.
		m = update(m, tea.MouseClickMsg{X: b.x + 1 + 3*4, Y: b.y + 2, Button: tea.MouseLeft})
		if m.calendar != nil || m.form.field("since").Value != "2026-12-03" {
			t.Fatalf("mouse day selection disagrees with the grid at %v", size)
		}
		m = update(m, key(tea.KeyEnter, ""))
		m = clickChoiceText(t, m, "Clear")
		if m.calendar != nil || m.form.field("since").Value != "" {
			t.Fatalf("Clear did not remove the date limit at %v", size)
		}
	}
}

func TestCalendarKeepsManualTimestampsAndSupportsToday(t *testing.T) {
	for _, value := range []string{"2026-11-14T18:30:00+03:30", "1794672000"} {
		m := datePickerModel(t, "since", value)
		m = update(m, key(tea.KeyEnter, ""))
		if m.calendar == nil || m.calendar.day.Format("2006-01-02") != "2026-11-14" {
			t.Fatalf("calendar did not start at the existing timestamp %s", value)
		}
		m = clickChoiceText(t, m, "Type date/time")
		if m.calendar != nil || !m.editing || m.editor.Value() != value {
			t.Fatal("switching to manual entry lost the timestamp")
		}
		m = update(m, key('u', "", tea.ModCtrl))
		m = update(m, tea.PasteMsg{Content: "2026-11-01T12:45:00Z"})
		m = update(m, key('r', "", tea.ModCtrl))
		if m.screen != review || !slices.Contains(m.plan.Args, "--since=2026-11-01T12:45:00Z") {
			t.Fatal("manual timestamp did not reach the backup plan")
		}
	}
	m := datePickerModel(t, "until", "2020-01-01")
	m = update(m, key(tea.KeyEnter, ""))
	m = clickChoiceText(t, m, "Today")
	m = update(m, key(tea.KeyEnter, ""))
	if m.form.field("until").Value != time.Now().Format("2006-01-02") {
		t.Fatal("Today did not select today's local date")
	}
}

func TestCalendarTabAndReviewSaveTheHighlightedDay(t *testing.T) {
	for _, reviewWithMouse := range []bool{false, true} {
		m := datePickerModel(t, "since", "2026-11-14")
		m = update(m, key(tea.KeyEnter, ""))
		m = update(m, key(tea.KeyLeft, ""))
		if reviewWithMouse {
			m = clickChoiceText(t, m, "[ Review ]")
		} else {
			m = update(m, key(tea.KeyTab, ""))
			if m.form.Fields[m.visibleFields()[m.focus]].Name != "until" {
				t.Fatal("Tab did not move to Until")
			}
			m = update(m, key('r', "", tea.ModCtrl))
		}
		if m.screen != review || m.calendar != nil || !slices.Contains(m.plan.Args, "--since=2026-11-13") {
			t.Fatal("leaving the calendar lost the highlighted day")
		}
	}
}

func TestCalendarKeepsMouseTargetsWhenAnotherFieldIsClicked(t *testing.T) {
	for _, height := range []int{25, 40} {
		m := datePickerModel(t, "since", "2026-11-14")
		m.height = height
		m = update(m, key(tea.KeyEnter, ""))
		m = update(m, key(tea.KeyRight, ""))
		if height == 25 {
			m = clickChoiceText(t, m, "▸ Since")
			if m.calendar == nil || m.calendar.field != "since" {
				t.Fatal("clicking the active field opened a different field after the form scrolled")
			}
		} else {
			m = clickChoiceText(t, m, "Until (date/time)")
			if m.calendar == nil || m.calendar.field != "until" {
				t.Fatal("clicking Until below an expanded calendar opened a different field")
			}
		}
		if m.form.field("since").Value != "2026-11-14" {
			t.Fatal("clicking outside the calendar should discard unchosen navigation")
		}
	}
}

func TestSmallCalendarButtonsReturnSafelyToTheForm(t *testing.T) {
	for _, label := range []string{"Type date/time", "Cancel"} {
		m := datePickerModel(t, "since", "2026-11-14")
		m.width, m.height = 55, 20
		m = update(m, key(tea.KeyEnter, ""))
		m = update(m, key(tea.KeyF6, ""))
		for m.buttons()[m.buttonFocus].Label != label {
			m = update(m, key(tea.KeyTab, ""))
		}
		// Check the scrolled button bar and activate its last actions by keyboard.
		if !strings.Contains(ansi.Strip(m.View().Content), label) {
			t.Fatal("the focused calendar action scrolled out of view")
		}
		m = update(m, key(tea.KeyEnter, ""))
		if m.calendar != nil || !strings.Contains(ansi.Strip(m.View().Content), "[ Review ]") {
			t.Fatal("calendar action did not safely restore the form")
		}
	}
}
