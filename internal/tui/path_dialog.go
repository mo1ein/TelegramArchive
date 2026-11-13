package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/kballard/go-shellquote"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func flagPathKind(path []string, flag *pflag.Flag) string {
	if kind := flag.Annotations["ta:path"]; len(kind) > 0 {
		return kind[0]
	}
	command := strings.Join(path, " ")
	if flag.Name == "output" {
		switch command {
		case "archive", "account", "sync", "cron add":
			return "directory"
		case "chat export", "chat users":
			return "save"
		}
	}
	if command == "backup" && flag.Name == "dst" {
		return "save"
	}
	if command == "upload" && flag.Name == "path" {
		return "any"
	}
	if command == "download" && flag.Name == "file" {
		return "open"
	}
	if _, ok := flag.Annotations[cobra.BashCompSubdirsInDir]; ok {
		return "directory"
	}
	if _, ok := flag.Annotations[cobra.BashCompFilenameExt]; ok {
		return "open"
	}
	if flag.Name == "dir" || (command == "login" && flag.Name == "desktop") {
		return "directory"
	}
	return ""
}

type pathRequest struct {
	Title, Kind, Initial string
	Multiple             bool
}

type pathDialog struct {
	field   string
	request pathRequest
	ctx     context.Context
	cancel  context.CancelFunc
	browser *pathBrowser
}

type pathSelectedMsg struct {
	dialog *pathDialog
	paths  []string
	err    error
}

var errPathDialogUnavailable = errors.New("system path dialog unavailable")

func (m *Model) openPathDialog(field Field) tea.Cmd {
	if m.pathDialog != nil || field.PathKind == "" {
		return nil
	}
	value := field.Value
	if m.editing {
		value = m.editor.Value()
	}
	if field.Type == "arguments" {
		if paths, err := shellquote.Split(value); err == nil && len(paths) > 0 {
			value = paths[0]
		}
	} else if field.Multi {
		if paths, err := readCSV(value); err == nil && len(paths) > 0 {
			value = paths[0]
		}
	}
	ctx, cancel := context.WithCancel(m.ctx)
	dialog := &pathDialog{field: field.Name, request: pathRequest{Title: field.Label, Kind: field.PathKind, Initial: value, Multiple: field.PathMultiple}, ctx: ctx, cancel: cancel}
	m.pathDialog, m.err, m.buttonsFocused = dialog, "", false
	choose := m.choosePath
	return func() tea.Msg {
		paths, err := choose(ctx, dialog.request)
		return pathSelectedMsg{dialog, paths, err}
	}
}

func (m *Model) closePathDialog() {
	if m.pathDialog != nil {
		m.pathDialog.cancel()
		m.pathDialog = nil
		m.buttonsFocused = false
	}
}

func (m *Model) pathDialogResult(msg pathSelectedMsg) tea.Cmd {
	if msg.dialog != m.pathDialog {
		return nil
	}
	if errors.Is(msg.err, errPathDialogUnavailable) {
		return m.openPathBrowser()
	}
	if msg.err != nil {
		m.err = safeText(msg.err.Error())
		m.closePathDialog()
		return nil
	}
	m.acceptPaths(msg.paths)
	return nil
}

func (m *Model) pathControlClick(msg tea.MouseClickMsg) (bool, tea.Cmd) {
	fields := m.visibleFields()
	if len(fields) == 0 || m.focus >= len(fields) {
		return false, nil
	}
	field := m.form.Fields[fields[m.focus]]
	if field.PathKind == "" {
		return false, nil
	}
	g := m.formLayout(m.width-4, m.height-9)
	x, y, width := 2+g.controlX, 3+g.top+g.fieldRow(m.focus), g.controlWidth
	if msg.X < x || msg.X >= x+width || msg.Y < y || msg.Y > y+1 {
		return false, nil
	}
	if msg.Y == y+1 && msg.X >= x+12 && msg.X < x+25 {
		if !m.editing {
			m.editField()
		}
		return true, nil
	}
	if msg.Y == y+1 && msg.X >= x+11 {
		return false, nil
	}
	if m.editing && msg.Y != y+1 {
		return false, nil
	}
	return true, m.openPathDialog(field)
}

func (m *Model) acceptPaths(paths []string) {
	dialog := m.pathDialog
	if dialog == nil {
		return
	}
	if len(paths) == 0 {
		m.closePathDialog()
		return
	}
	field := m.form.field(dialog.field)
	if field == nil || m.screen != configure {
		m.closePathDialog()
		return
	}
	err := checkPaths(dialog.request, paths)
	if err == nil {
		copy := *field
		copy.Value, copy.Dirty = paths[0], true
		if copy.Type == "arguments" {
			copy.Value = shellquote.Join(paths...)
		} else if copy.Multi {
			copy.Value = csvString(paths)
		}
		err = validate(copy)
		if err == nil {
			*field = copy
			m.editing, m.err = false, ""
			m.editor.Blur()
		}
	}
	if err != nil {
		m.err = safeText(err.Error())
		if dialog.browser != nil {
			dialog.browser.err = m.err
			return
		}
	}
	m.closePathDialog()
}

func checkPaths(request pathRequest, paths []string) error {
	if !request.Multiple && len(paths) != 1 {
		return fmt.Errorf("choose one path")
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if request.Kind == "save" && os.IsNotExist(err) {
			parent, parentErr := os.Stat(filepath.Dir(path))
			if parentErr != nil || !parent.IsDir() {
				return fmt.Errorf("choose an existing destination folder")
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("read selected path: %w", err)
		}
		switch request.Kind {
		case "directory":
			if !info.IsDir() {
				return fmt.Errorf("choose a folder")
			}
		case "open", "save":
			if !info.Mode().IsRegular() {
				return fmt.Errorf("choose a file")
			}
		case "any":
			if !info.IsDir() && !info.Mode().IsRegular() {
				return fmt.Errorf("choose a file or folder")
			}
		}
	}
	return nil
}

// Start from the closest existing folder, including when a destination is new.
func pathLocation(initial string) (directory, name string, err error) {
	if initial == "~" || strings.HasPrefix(initial, "~/") {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "", "", homeErr
		}
		initial = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(initial, "~"), "/"))
	}
	if initial == "" {
		initial = "."
	}
	path, err := filepath.Abs(initial)
	if err != nil {
		return "", "", err
	}
	name = filepath.Base(path)
	for {
		info, statErr := os.Stat(path)
		if statErr == nil && info.IsDir() {
			return path, name, nil
		}
		if statErr != nil && !os.IsNotExist(statErr) {
			return "", "", statErr
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", "", fmt.Errorf("could not find a starting folder")
		}
		path = parent
	}
}
