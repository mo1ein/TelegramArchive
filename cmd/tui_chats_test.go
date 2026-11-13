package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/iyear/tdl/core/storage"
	"github.com/mo1ein/TelegramArchive/internal/taskui"
	"github.com/spf13/viper"
)

func TestChatBrowserConnectionTimeoutExplainsFailure(t *testing.T) {
	var output bytes.Buffer
	started := time.Now()
	err := browseTUIChats(context.Background(), &output, "true", 25*time.Millisecond, func(ctx context.Context, _ func(context.Context, *telegram.Client, storage.Storage) error) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if err == nil || !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), "ALL_PROXY") {
		t.Fatalf("unhelpful timeout: %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("connection timeout not enforced")
	}
	var event taskui.ChatEvent
	if json.Unmarshal(bytes.Split(output.Bytes(), []byte{'\n'})[0], &event) != nil || !strings.Contains(event.Stage, "Connecting") {
		t.Fatal("connection stage was not published before waiting")
	}
}
func TestTUIChatStreamIsHiddenAndOrdinaryCLIUnchanged(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	root := New()
	command, _, err := root.Find([]string{"chat", "ls"})
	if err != nil {
		t.Fatal(err)
	}
	flag := command.Flags().Lookup("tui-stream")
	if flag == nil || !flag.Hidden || flag.DefValue != "false" {
		t.Fatal("stream flag exposed or default CLI altered")
	}
}
