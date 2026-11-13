package cmd

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	upstreamcmd "github.com/iyear/tdl/cmd"
	"github.com/mo1ein/TelegramArchive/consts"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestLoginDefaultsToDesktop(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	command, _, err := New().Find([]string{"login"})
	if err != nil {
		t.Fatal(err)
	}
	mode := command.Flags().Lookup("type")
	if mode.Value.String() != "desktop" || mode.DefValue != "desktop" {
		t.Fatalf("plain ta login selects %s (documented default %s); want desktop", mode.Value.String(), mode.DefValue)
	}
	if mode.Changed {
		t.Fatal("the default must not count as an explicit login-method choice")
	}
}

func TestLoginMethodSelection(t *testing.T) {
	home := t.TempDir()
	previousHome := consts.HomeDir
	consts.HomeDir = home
	t.Cleanup(func() { consts.HomeDir = previousHome })
	desktopPath := filepath.Join(home, "Library", "Application Support", "Telegram Desktop")
	if err := os.MkdirAll(filepath.Join(desktopPath, "tdata"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"desktop default", nil, "desktop"},
		{"phone login", []string{"-T", "code"}, "code"},
		{"explicit QR", []string{"-T", "qr"}, "qr"},
		{"legacy code bypasses Desktop", []string{"--code"}, "desktop"},
		{"desktop discovery", []string{"-T", "desktop"}, "desktop"},
		{"desktop path", []string{"-d", desktopPath}, "desktop"},
		{"desktop passcode", []string{"-p", "synthetic-passcode"}, "desktop"},
		{"explicit QR wins", []string{"-T", "qr", "-d", "/synthetic/Telegram Desktop"}, "qr"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := &cobra.Command{Use: "ta"}
			root.AddGroup(&cobra.Group{ID: "account", Title: "Accounts"})
			login := upstreamcmd.NewLogin()
			// Stop at the authentication boundary, without reading or overwriting
			// a real session or attempting to connect to Telegram.
			stopped := errors.New("authentication boundary reached")
			called := false
			login.RunE = func(cmd *cobra.Command, _ []string) error {
				called = true
				if got := cmd.Flags().Lookup("type").Value.String(); got != tc.want {
					t.Errorf("selected %s, want %s", got, tc.want)
				}
				legacy, _ := cmd.Flags().GetBool("code")
				if runtime.GOOS == "darwin" && tc.want == "desktop" && !legacy {
					if got, _ := cmd.Flags().GetString("desktop"); got != filepath.Join(desktopPath, "tdata") {
						t.Errorf("Desktop importer received %q", got)
					}
				}
				return stopped
			}
			root.AddCommand(login)
			wrapLoginIdentity(root)
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(append([]string{"login"}, tc.args...))
			if err := root.Execute(); !errors.Is(err, stopped) || !called {
				t.Fatalf("called=%v err=%v", called, err)
			}
		})
	}
}

func TestMacNativeDesktopStopsBeforeImport(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS login adapter")
	}
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "Library", "Containers", "ru.keepcoder.Telegram"), 0700); err != nil {
		t.Fatal(err)
	}
	previousHome := consts.HomeDir
	consts.HomeDir = home
	t.Cleanup(func() { consts.HomeDir = previousHome })
	root := &cobra.Command{Use: "ta"}
	root.AddGroup(&cobra.Group{ID: "account", Title: "Accounts"})
	login := upstreamcmd.NewLogin()
	login.RunE = func(*cobra.Command, []string) error {
		t.Fatal("unsupported native data reached the session importer")
		return nil
	}
	root.AddCommand(login)
	wrapLoginIdentity(root)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"login"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "native Telegram for macOS") {
		t.Fatal(err)
	}
}
