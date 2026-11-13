package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/mo1ein/TelegramArchive/consts"
	"github.com/spf13/cobra"
)

// Resolve macOS data locations before calling the transfer core's importer.
// The core only searches the unsandboxed Desktop location on macOS.
func prepareDesktopLogin(cmd *cobra.Command) (bool, error) {
	if runtime.GOOS != "darwin" {
		return false, nil
	}
	mode := cmd.Flags().Lookup("type")
	if mode == nil || mode.Value.String() != "desktop" {
		return false, nil
	}
	// The deprecated --code flag still bypasses Desktop import upstream.
	if legacy := cmd.Flags().Lookup("code"); legacy != nil && legacy.Value.String() == "true" {
		return false, nil
	}
	desktop := cmd.Flags().Lookup("desktop")
	if desktop == nil {
		return false, nil
	}
	source, err := findMacLoginSource(consts.HomeDir, desktop.Value.String())
	if err != nil {
		return false, err
	}
	// This updates the actual options bound to the inherited login handler.
	// Leave Changed intact: automatic discovery is not a user-supplied flag.
	return source.native, desktop.Value.Set(source.path)
}

type desktopSource struct {
	path   string
	native bool
}

// Only inspect directory metadata here, never session keys or message data.
func findMacDesktopData(home, explicit string) (string, error) {
	source, err := findMacLoginSource(home, explicit)
	return source.path, err
}

func findMacLoginSource(home, explicit string) (desktopSource, error) {
	candidates := []string{
		filepath.Join(home, "Library", "Application Support", "Telegram Desktop"),
		filepath.Join(home, "Library", "Containers", "org.telegram.desktop", "Data", "Library", "Application Support", "Telegram Desktop"),
	}
	if explicit != "" {
		candidates = []string{explicit}
	}
	var inaccessible error
	for _, candidate := range candidates {
		path := candidate
		if filepath.Base(path) != "tdata" {
			path = filepath.Join(path, "tdata")
		}
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			return desktopSource{path: path}, nil
		}
		if err != nil && !os.IsNotExist(err) && inaccessible == nil {
			inaccessible = fmt.Errorf("cannot access Telegram Desktop data at %q: %w", path, err)
		}
	}
	if inaccessible != nil {
		return desktopSource{}, inaccessible
	}
	native := []string{
		filepath.Join(home, "Library", "Group Containers", "6N38VWS5BX.ru.keepcoder.Telegram"),
		filepath.Join(home, "Library", "Containers", "ru.keepcoder.Telegram"),
	}
	if explicit != "" {
		root := explicit
		if filepath.Base(root) == "atomic-state" {
			root = filepath.Dir(filepath.Dir(root))
		} else if filepath.Base(root) == "accounts-metadata" {
			root = filepath.Dir(root)
		}
		native = []string{root}
	}
	for _, root := range native {
		for _, channel := range []string{"", "stable", "appstore", "beta"} {
			path := filepath.Join(root, channel)
			info, err := os.Stat(filepath.Join(path, "accounts-metadata", "atomic-state"))
			if err == nil && info.Mode().IsRegular() {
				return desktopSource{path: path, native: true}, nil
			}
			if err != nil && !os.IsNotExist(err) {
				return desktopSource{}, fmt.Errorf("cannot read native Telegram metadata; give the app running ta Full Disk Access in macOS Privacy & Security, reopen it and retry: %w", err)
			}
		}
	}
	if explicit != "" {
		return desktopSource{}, fmt.Errorf("no Telegram session data found at %q; -d must point to tdata, its parent data directory, or the native client's stable/appstore data directory, not the application bundle", explicit)
	}
	for _, path := range native {
		if _, err := os.Stat(path); err == nil {
			return desktopSource{}, fmt.Errorf("native Telegram for macOS was found but has no signed-in account metadata; open Telegram and sign in, then retry ta login")
		}
	}
	return desktopSource{}, fmt.Errorf("no Telegram session data found in the standard or sandboxed macOS data locations; open Telegram and sign in, or specify its data directory with -d (not the application bundle)")
}
