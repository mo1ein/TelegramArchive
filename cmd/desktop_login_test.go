package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMacDesktopDataDiscovery(t *testing.T) {
	for _, location := range []string{
		"Library/Application Support/Telegram Desktop/tdata",
		"Library/Containers/org.telegram.desktop/Data/Library/Application Support/Telegram Desktop/tdata",
	} {
		t.Run(location, func(t *testing.T) {
			home := t.TempDir()
			want := filepath.Join(home, filepath.FromSlash(location))
			if err := os.MkdirAll(want, 0700); err != nil {
				t.Fatal(err)
			}
			got, err := findMacDesktopData(home, "")
			if err != nil || got != want {
				t.Fatalf("got=%q want=%q err=%v", got, want, err)
			}
		})
	}
}

func TestMacDesktopExplicitPath(t *testing.T) {
	home := t.TempDir()
	parent := filepath.Join(home, "Custom Desktop")
	want := filepath.Join(parent, "tdata")
	if err := os.MkdirAll(want, 0700); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{parent, want} {
		got, err := findMacDesktopData(home, input)
		if err != nil || got != want {
			t.Fatalf("input=%q got=%q err=%v", input, got, err)
		}
	}
	if _, err := findMacDesktopData(home, filepath.Join(home, "Telegram.app")); err == nil || !strings.Contains(err.Error(), "not the application bundle") {
		t.Fatal(err)
	}
}

func TestMacNativeTelegramHasSpecificDiagnostic(t *testing.T) {
	for _, location := range []string{
		"Library/Group Containers/6N38VWS5BX.ru.keepcoder.Telegram",
		"Library/Containers/ru.keepcoder.Telegram",
	} {
		t.Run(location, func(t *testing.T) {
			home := t.TempDir()
			native := filepath.Join(home, filepath.FromSlash(location))
			if err := os.MkdirAll(native, 0700); err != nil {
				t.Fatal(err)
			}
			_, err := findMacDesktopData(home, "")
			if err == nil {
				t.Fatal("native format must not be sent to the tdata decoder")
			}
			for _, want := range []string{"native Telegram for macOS", "no signed-in account metadata"} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("missing %q: %v", want, err)
				}
			}
			entries, err := os.ReadDir(native)
			if err != nil || len(entries) != 0 {
				t.Fatalf("discovery modified native data: %v", err)
			}
		})
	}
}

func TestMacDesktopDataTakesPriorityOverNativeClient(t *testing.T) {
	home := t.TempDir()
	want := filepath.Join(home, "Library", "Application Support", "Telegram Desktop", "tdata")
	for _, path := range []string{want, filepath.Join(home, "Library", "Containers", "ru.keepcoder.Telegram")} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	got, err := findMacDesktopData(home, "")
	if err != nil || got != want {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestMacDesktopMissingData(t *testing.T) {
	_, err := findMacDesktopData(t.TempDir(), "")
	if err == nil || !strings.Contains(err.Error(), "standard or sandboxed macOS") {
		t.Fatal(err)
	}
}

func TestMacNativeClientSessionIsDiscoverable(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "Library", "Group Containers", "6N38VWS5BX.ru.keepcoder.Telegram", "stable")
	if err := os.MkdirAll(filepath.Join(root, "accounts-metadata"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "accounts-metadata", "atomic-state"), []byte(`{"records":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := findMacDesktopData(home, "")
	if err != nil || got != root {
		t.Fatalf("native client session root: got=%q err=%v", got, err)
	}
}
