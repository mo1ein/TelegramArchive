package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mo1ein/TelegramArchive/consts"
	"github.com/spf13/viper"
)

func TestInstalledVersionWorksWithoutSessionStorage(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	oldVersion := consts.Version
	consts.Version = "v1.2.3-install-test"
	t.Cleanup(func() { consts.Version = oldVersion })

	for _, arg := range []string{"version", "--version"} {
		t.Run(arg, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			root := New()
			var output bytes.Buffer
			root.SetOut(&output)
			root.SetArgs([]string{"--storage", "type=bolt,path=" + filepath.Join(blocked, "data"), arg})
			if err := root.Execute(); err != nil {
				t.Fatalf("version must work without session storage: %v", err)
			}
			if !strings.Contains(output.String(), consts.Version) {
				t.Fatalf("version output did not use the build version: %q", output.String())
			}
		})
	}
}
