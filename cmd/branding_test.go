package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatih/color"
	sessionkv "github.com/iyear/tdl/pkg/kv"
	"github.com/klauspost/compress/zstd"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"ta/consts"
)

func TestTACommandBranding(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	root := New()
	var check func(*cobra.Command)
	check = func(command *cobra.Command) {
		text := command.Short + command.Long + command.Example
		command.Flags().VisitAll(func(flag *pflag.Flag) { text += flag.Usage + flag.DefValue })
		if strings.Contains(strings.ToLower(text), "tdl") {
			t.Fatalf("old product name in %s: %s", command.CommandPath(), text)
		}
		for _, child := range command.Commands() {
			check(child)
		}
	}
	check(root)
	for _, tc := range []struct{ command, output string }{{"export", "ta-export.json"}, {"users", "ta-users.json"}} {
		command, _, err := root.Find([]string{"chat", tc.command})
		if err != nil {
			t.Fatal(err)
		}
		value, err := command.Flags().GetString("output")
		if err != nil || value != tc.output {
			t.Fatalf("%s output=%q, %v", command.CommandPath(), value, err)
		}
	}
	var version bytes.Buffer
	root.SetOut(&version)
	root.SetArgs([]string{"version"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(version.String(), "ta ") {
		t.Fatalf("unexpected version: %q", version.String())
	}
}

func TestTAEnvironmentPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name, primary, legacy, flag, want string
	}{
		{"primary", "socks5://primary:1080", "socks5://legacy:1080", "", "socks5://primary:1080"},
		{"legacy compatibility", "", "socks5://legacy:1080", "", "socks5://legacy:1080"},
		{"explicit flag", "socks5://primary:1080", "socks5://legacy:1080", "socks5://flag:1080", "socks5://flag:1080"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			viper.Reset()
			defer viper.Reset()
			t.Setenv("TA_PROXY", tc.primary)
			t.Setenv("TDL_PROXY", tc.legacy)
			root := New()
			if tc.flag != "" {
				if err := root.ParseFlags([]string{"--proxy", tc.flag}); err != nil {
					t.Fatal(err)
				}
			}
			if got := viper.GetString("proxy"); got != tc.want {
				t.Fatalf("proxy=%q want %q", got, tc.want)
			}
		})
	}
}

func TestTASessionStorageCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name, expected, driver string
		directories            []string
		legacyFile             bool
	}{
		{"fresh install", ".ta/data", "bolt", nil, false},
		{"existing login", ".tdl/data", "bolt", []string{".tdl/data"}, false},
		{"primary store", ".ta/data", "bolt", []string{".ta/data", ".tdl/data"}, false},
		{"old file store", ".tdl/data.kv", "legacy", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			for _, directory := range tc.directories {
				if err := os.MkdirAll(filepath.Join(home, directory), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if tc.legacyFile {
				if err := os.MkdirAll(filepath.Join(home, ".tdl"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(home, ".tdl", "data.kv"), []byte("existing login"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			got := defaultProductStorage(home)
			if got["path"] != filepath.Join(home, tc.expected) || got["type"] != tc.driver {
				t.Fatalf("unexpected session storage: %v", got)
			}
		})
	}
}

func TestTABackupFilename(t *testing.T) {
	for _, destination := range []string{"", "my-session.backup"} {
		t.Run(destination, func(t *testing.T) {
			root := &cobra.Command{Use: "ta"}
			backup := &cobra.Command{Use: "backup"}
			var got string
			backup.Flags().StringVar(&got, "dst", destination, "destination")
			backup.RunE = func(*cobra.Command, []string) error { return nil }
			root.AddCommand(backup)
			brandCommands(root)
			if err := backup.RunE(backup, nil); err != nil {
				t.Fatal(err)
			}
			if destination == "" && !strings.HasSuffix(got, ".backup.ta") || destination != "" && got != destination {
				t.Fatalf("unexpected backup filename %q", got)
			}
		})
	}
}

func TestTABackupReusesExistingSession(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	home := t.TempDir()
	oldHome, oldData, oldLog := consts.HomeDir, consts.DataDir, consts.LogPath
	oldOutput := color.Output
	t.Cleanup(func() {
		consts.HomeDir, consts.DataDir, consts.LogPath = oldHome, oldData, oldLog
		color.Output = oldOutput
	})
	consts.HomeDir = home
	consts.DataDir = filepath.Join(home, ".ta")
	consts.LogPath = filepath.Join(consts.DataDir, "log")
	color.Output = &bytes.Buffer{}
	store, err := sessionkv.NewWithMap(map[string]string{"type": "bolt", "path": filepath.Join(home, ".tdl", "data")})
	if err != nil {
		t.Fatal(err)
	}
	namespace, err := store.Open("personal")
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	if err := namespace.Set(context.Background(), "proof", []byte("existing-session")); err != nil {
		store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(home, "session.backup.ta")
	root := New()
	root.SetArgs([]string{"backup", "--dst", backup})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.Close()
	data, err = decoder.DecodeAll(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	var metadata sessionkv.Meta
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatal(err)
	}
	if string(metadata["personal"]["proof"]) != "existing-session" {
		t.Fatal("ta did not reuse the existing session store")
	}
}

func TestTAProductMessagesPreserveDataAndErrors(t *testing.T) {
	var output bytes.Buffer
	writer := productOutput{&output}
	message := []byte("WARN: Export only generates minimal JSON for tdl download, not for backup.\n@tdl_channel file=tdl.mp4\n")
	if n, err := writer.Write(message); err != nil || n != len(message) {
		t.Fatalf("write returned %d, %v", n, err)
	}
	if !strings.Contains(output.String(), "for ta download") || !strings.Contains(output.String(), "@tdl_channel file=tdl.mp4") {
		t.Fatalf("product text was not replaced safely: %q", output.String())
	}
	if !errors.Is(productError{io.EOF}, io.EOF) {
		t.Fatal("product errors lost their cause")
	}
}
