package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (p *datePicker) openYears() {
	p.years, p.yearCursor = true, max(1, min(9999, p.day.Year()))
	p.yearText, p.yearError = "", ""
}

func (p datePicker) firstYear() int { return max(1, p.yearCursor/20*20) }

func (p *datePicker) moveYear(delta int) {
	p.yearCursor = max(1, min(9999, p.yearCursor+delta))
	p.yearText, p.yearError = "", ""
}

func (p *datePicker) chooseYear() bool {
	year := p.yearCursor
	if p.yearText != "" {
		var err error
		year, err = strconv.Atoi(p.yearText)
		if err != nil || year < 1 || year > 9999 {
			p.yearError = "Enter a year from 1 to 9999"
			return false
		}
	}
	p.moveMonth((year - p.day.Year()) * 12)
	p.years = false
	p.yearText, p.yearError = "", ""
	return true
}

func (m Model) yearPickerView() string {
	p := m.calendar
	frame := lipgloss.NewStyle().Foreground(violet)
	first := p.firstYear()
	heading := fmt.Sprintf("%04d–%04d", first, min(9999, first+19))
	space := calendarWidth - lipgloss.Width(heading) - 6
	lines := []string{frame.Render("╭ ") + selectedStyle.Render("‹") + strings.Repeat(" ", space/2) + titleStyle.Render(heading) + strings.Repeat(" ", space-space/2) + selectedStyle.Render("›") + frame.Render(" ╮")}
	input := "Type a year: " + p.yearText + "▏"
	lines = append(lines, frame.Render("│")+fit(accent.Render(input), calendarWidth-2, 1)+frame.Render("│"))
	for row := range 5 {
		var cells strings.Builder
		for column := range 4 {
			year := first + row*4 + column
			label := "       "
			style := textStyle
			if year <= 9999 {
				label = fmt.Sprintf(" %04d  ", year)
				if year == p.yearCursor {
					label = fmt.Sprintf("▸%04d  ", year)
					style = selectedStyle.Bold(true)
				}
			}
			cells.WriteString(style.Render(label))
		}
		lines = append(lines, frame.Render("│")+cells.String()+frame.Render("│"))
	}
	hint := dim.Render("Arrows choose · Enter opens")
	if p.yearError != "" {
		hint = warning.Render(p.yearError)
	}
	lines = append(lines, frame.Render("│")+fit(hint, calendarWidth-2, 1)+frame.Render("│"), frame.Render("╰"+strings.Repeat("─", calendarWidth-2)+"╯"))
	return strings.Join(lines, "\n")
}

func (m Model) updateYearPicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := m.calendar
	switch msg.String() {
	case "esc", "y":
		p.years = false
	case "left":
		p.moveYear(-1)
	case "right":
		p.moveYear(1)
	case "up":
		p.moveYear(-4)
	case "down":
		p.moveYear(4)
	case "pgup", "ctrl+pgup":
		p.moveYear(-20)
	case "pgdown", "ctrl+pgdown":
		p.moveYear(20)
	case "home":
		p.yearCursor = p.firstYear()
		p.yearText, p.yearError = "", ""
	case "end":
		p.yearCursor = min(9999, p.firstYear()+19)
		p.yearText, p.yearError = "", ""
	case "backspace":
		runes := []rune(p.yearText)
		if len(runes) > 0 {
			p.yearText = string(runes[:len(runes)-1])
		}
		p.yearError = ""
	case "ctrl+u", "delete":
		p.yearText, p.yearError = "", ""
	case "enter", "tab":
		p.chooseYear()
	case "ctrl+r", "ctrl+enter":
		if p.chooseYear() {
			m.prepareReview()
		}
	case "i":
		m.typeDate()
	default:
		if msg.Text != "" && len(p.yearText) < 16 {
			p.yearText += msg.Text
			p.yearError = ""
		}
	}
	return m, nil
}

func (p *datePicker) yearClick(x, y int) {
	switch {
	case y == 0 && x >= 1 && x <= 3:
		p.moveYear(-20)
	case y == 0 && x >= calendarWidth-4 && x <= calendarWidth-2:
		p.moveYear(20)
	case y >= 2 && y < 7 && x >= 1 && x < calendarWidth-1:
		year := p.firstYear() + (y-2)*4 + (x-1)/7
		if year <= 9999 {
			p.yearCursor, p.yearText = year, ""
			p.chooseYear()
		}
	}
}
