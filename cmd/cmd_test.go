package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mo1ein/TelegramArchive/internal/archive"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestArchiveIgnoresConfigurationFiles(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	t.Chdir(t.TempDir())
	if err := os.WriteFile(".env", []byte("CHAT_IDS=invalid JSON\nDOWNLOAD_PATH=/unexpected\nENABLE_RESUME=false\nMEDIA_EXPORT_PHOTOS=invalid boolean\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CHAT_IDS", "invalid JSON")
	t.Setenv("DOWNLOAD_PATH", "/unexpected")
	t.Setenv("ENABLE_RESUME", "false")
	f := &archiveFlags{}
	c := newArchive(f)
	if err := c.ParseFlags([]string{"--since", "bad"}); err != nil {
		t.Fatal(err)
	}
	c.SetContext(context.Background())
	if err := c.RunE(c, nil); err == nil || !strings.Contains(err.Error(), "since:") {
		t.Fatalf("unexpected configuration-file handling: %v", err)
	}
	if f.output != "data" || !f.resume || !reflect.DeepEqual(f.media, []string{"photos"}) {
		t.Fatalf("removed configuration changed command defaults: %+v", f)
	}
}

func TestCommandsHaveNoConfigurationFileFlag(t *testing.T) {
	for _, command := range []*cobra.Command{NewArchive(), NewAccount(), NewSync()} {
		if command.Flags().Lookup("env-file") != nil {
			t.Fatalf("%s still exposes a configuration-file flag", command.Name())
		}
	}
}

func TestOfflineStatsCommand(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	file := filepath.Join(t.TempDir(), "chat.json")
	if err := os.WriteFile(file, []byte(`{"name":"test","messages":[{"id":1,"type":"message","text":"hello world"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	c := New()
	var output bytes.Buffer
	c.SetOut(&output)
	c.SetArgs([]string{"stats", file})
	if err := c.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	var got struct{ Messages, Words int64 }
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Messages != 1 || got.Words != 2 {
		t.Fatal(got)
	}
}

func TestInvalidArchiveFlagsFailBeforeNetwork(t *testing.T) {
	for _, args := range [][]string{{"--since", "bad"}, {"--part-size", "-1"}, {"--media", "typo"}, {"--all", "--chat", "me"}} {
		t.Run(args[0], func(t *testing.T) {
			viper.Reset()
			defer viper.Reset()
			viper.Set("limit", 2)
			viper.Set("threads", 4)
			viper.Set("pool", 8)
			c := NewArchive()
			if err := c.ParseFlags(args); err != nil {
				t.Fatal(err)
			}
			c.SetContext(context.Background())
			if err := c.RunE(c, nil); err == nil {
				t.Fatal("accepted invalid options")
			}
		})
	}
}

func TestAccountSelectionDoesNotReadChatIDs(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	t.Setenv("CHAT_IDS", "invalid JSON")
	c := NewAccount()
	if err := c.ParseFlags([]string{"--include", "invalid-section"}); err != nil {
		t.Fatal(err)
	}
	c.SetContext(context.Background())
	if err := c.RunE(c, nil); err == nil || !strings.Contains(err.Error(), "unknown account section") {
		t.Fatalf("account selection depended on chats: %v", err)
	}
}

func TestFileFiltersSelectMediaUnlessExplicitlyRestricted(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{[]string{"--extensions", "pdf"}, archive.MediaKinds},
		{[]string{"--mime", "video/*"}, archive.MediaKinds},
		{[]string{"--extensions", "pdf", "--media", "none"}, []string{"none"}},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			viper.Reset()
			defer viper.Reset()
			t.Setenv("MEDIA_EXPORT_PHOTOS", "true")
			c := NewArchive()
			if err := c.ParseFlags(tc.args); err != nil {
				t.Fatal(err)
			}
			f := archiveFlags{media: []string{"photos"}}
			if c.Flags().Changed("media") {
				f.media, _ = c.Flags().GetStringSlice("media")
			}
			f.selectMedia(c)
			if !reflect.DeepEqual(f.media, tc.want) {
				t.Fatalf("media=%v want %v", f.media, tc.want)
			}
		})
	}
}

func TestSyncKeepsSessionBackupCommand(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	root := New()
	sync, _, err := root.Find([]string{"sync"})
	if err != nil || sync.Name() != "sync" || sync.Flags().Lookup("every") == nil {
		t.Fatal("recurring sync command is not reachable", err)
	}
	backup, _, err := root.Find([]string{"backup"})
	if err != nil || backup.Flags().Lookup("dst") == nil || backup.Flags().Lookup("everything") != nil {
		t.Fatal("session backup command was replaced", err)
	}
}
