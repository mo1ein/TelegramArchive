package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
	"github.com/mo1ein/TelegramArchive/internal/taskui"
)

// A bounded plain-text task surface. Child cursor/erase sequences update this
// surface, never the real terminal. OSC, clipboard and hyperlink escapes drop.
type transcript struct {
	parser               *ansi.Parser
	lines                [][]string
	x, y, savedX, savedY int
	width, height        int
	progress             []string
	respond              func(string)
}

func newTranscript(width, height int) *transcript {
	t := &transcript{lines: [][]string{{}}, width: max(20, min(width, 500)), height: max(5, min(height, 200))}
	t.parser = ansi.NewParser()
	t.parser.SetDataSize(8192)
	t.parser.SetHandler(ansi.Handler{HandleOsc: func(cmd int, data []byte) {
		payload := strings.TrimPrefix(string(data), "777;")
		if cmd == 777 && strings.HasPrefix(payload, taskui.ProgressPrefix) {
			var lines []string
			if json.Unmarshal([]byte(payload[len(taskui.ProgressPrefix):]), &lines) == nil && len(lines) == 3 {
				for i := range lines {
					lines[i] = safeText(lines[i])
				}
				t.progress = lines
			}
		}
	}, Print: t.print, Execute: t.control, HandleCsi: t.csi, HandleEsc: func(cmd ansi.Cmd) {
		switch byte(cmd) {
		case '7':
			t.savedX, t.savedY = t.x, t.y
		case '8':
			t.x, t.y = t.savedX, t.savedY
			t.ensure()
		}
	}})
	return t
}
func (t *transcript) write(p []byte) { t.parser.Parse(p) }
func (t *transcript) ensure() {
	t.y = max(0, min(t.y, len(t.lines)+t.height))
	for len(t.lines) <= t.y {
		t.lines = append(t.lines, []string{})
	}
	if len(t.lines) > 2000 {
		drop := len(t.lines) - 2000
		t.lines = t.lines[drop:]
		t.y -= drop
		t.savedY = max(0, t.savedY-drop)
	}
}
func (t *transcript) print(r rune) {
	w := runewidth.RuneWidth(r)
	if w == 0 {
		if t.x > 0 && t.x <= len(t.lines[t.y]) && len(t.lines[t.y][t.x-1]) < 64 {
			t.lines[t.y][t.x-1] += string(r)
		}
		return
	}
	if t.x+w > t.width {
		t.y++
		t.x = 0
		t.ensure()
	}
	t.ensure()
	line := t.lines[t.y]
	for len(line) < t.x+w {
		line = append(line, " ")
	}
	line[t.x] = string(r)
	for i := 1; i < w; i++ {
		line[t.x+i] = ""
	}
	t.lines[t.y] = line
	t.x += w
}
func (t *transcript) control(b byte) {
	switch b {
	case '\n':
		t.y++
		t.x = 0
		t.ensure()
	case '\r':
		t.x = 0
	case '\b':
		t.x = max(0, t.x-1)
	case '\t':
		for n := 8 - t.x%8; n > 0; n-- {
			t.print(' ')
		}
	}
}
func (t *transcript) csi(cmd ansi.Cmd, params ansi.Params) {
	param := func(i, d int) int { n, _, _ := params.Param(i, d); return n }
	n := max(1, param(0, 1))
	top := max(0, len(t.lines)-t.height)
	switch byte(cmd) {
	case 'A':
		t.y = max(top, t.y-n)
	case 'B':
		t.y = min(top+t.height-1, t.y+n)
		t.ensure()
	case 'C':
		t.x = min(t.width-1, t.x+n)
	case 'D':
		t.x = max(0, t.x-n)
	case 'E':
		t.y = min(top+t.height-1, t.y+n)
		t.x = 0
		t.ensure()
	case 'F':
		t.y = max(top, t.y-n)
		t.x = 0
	case 'G':
		t.x = min(t.width-1, n-1)
	case 'H', 'f':
		t.y = top + min(t.height-1, n-1)
		t.x = max(0, min(t.width-1, max(1, param(1, 1))-1))
		t.ensure()
	case 'K':
		t.ensure()
		line := t.lines[t.y]
		switch param(0, 0) {
		case 0:
			if t.x < len(line) {
				line = line[:t.x]
			}
		case 1:
			for i := 0; i < min(t.x+1, len(line)); i++ {
				line[i] = " "
			}
		case 2:
			line = nil
		}
		t.lines[t.y] = line
	case 'J':
		switch param(0, 0) {
		case 0:
			if t.x < len(t.lines[t.y]) {
				t.lines[t.y] = t.lines[t.y][:t.x]
			}
			t.lines = t.lines[:t.y+1]
		case 2:
			for i := top; i < len(t.lines); i++ {
				t.lines[i] = nil
			}
		}
	case 's':
		t.savedX, t.savedY = t.x, t.y
	case 'u':
		t.x, t.y = t.savedX, t.savedY
		t.ensure()
	case 'n':
		if param(0, 0) == 6 && t.respond != nil {
			t.respond(fmt.Sprintf("\x1b[%d;%dR", t.y-top+1, t.x+1))
		}
	}
}
func (t *transcript) text() string {
	lines := make([]string, len(t.lines))
	for i, line := range t.lines {
		lines[i] = strings.TrimRight(strings.Join(line, ""), " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}
func safeText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, ansi.Strip(s))
}
