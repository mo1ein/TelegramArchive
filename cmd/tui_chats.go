package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/expr-lang/expr"
	"github.com/gotd/td/telegram"
	"github.com/iyear/tdl/app/chat"
	"github.com/iyear/tdl/core/storage"
	"github.com/iyear/tdl/pkg/texpr"
	"github.com/mo1ein/TelegramArchive/internal/archive"
	"github.com/mo1ein/TelegramArchive/internal/taskui"
	transport "github.com/mo1ein/TelegramArchive/internal/telegram"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func wrapTUIChatBrowser(root *cobra.Command) {
	command, _, err := root.Find([]string{"chat", "ls"})
	if err != nil || command.Name() != "ls" {
		return
	}
	original := command.RunE
	var stream bool
	command.Flags().BoolVar(&stream, "tui-stream", false, "stream chat pages to the interactive workspace")
	_ = command.Flags().MarkHidden("tui-stream")
	command.RunE = func(cmd *cobra.Command, args []string) error {
		if !stream {
			return original(cmd, args)
		}
		cmd.SilenceUsage = true
		filter, _ := cmd.Flags().GetString("filter")
		return browseTUIChats(cmd.Context(), cmd.OutOrStdout(), filter, 90*time.Second, tRun)
	}
}

type chatBrowserRun func(context.Context, func(context.Context, *telegram.Client, storage.Storage) error) error

func browseTUIChats(parent context.Context, out io.Writer, filter string, timeout time.Duration, run chatBrowserRun) error {
	program, err := expr.Compile(filter, expr.AsBool())
	if err != nil {
		return fmt.Errorf("chat filter: %w", err)
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	events := json.NewEncoder(out)
	emit := func(stage string, items []*chat.Dialog) error {
		var payload []byte
		if items != nil {
			var err error
			payload, err = json.Marshal(items)
			if err != nil {
				return err
			}
		}
		return events.Encode(taskui.ChatEvent{Kind: "chat-page", Stage: stage, Items: payload})
	}
	if err := emit("Connecting to Telegram (up to 30 seconds)…", nil); err != nil {
		return err
	}
	// A separate connection deadline stops silent reconnect loops before pages
	// begin. Once connected, the full browse and individual pages stay bounded.
	connectCtx, connectCancel := context.WithCancel(ctx)
	defer connectCancel()
	timer := time.AfterFunc(min(timeout, 30*time.Second), connectCancel)
	defer timer.Stop()
	count := 0
	err = run(connectCtx, func(child context.Context, client *telegram.Client, kv storage.Storage) error {
		if !timer.Stop() {
			return context.DeadlineExceeded
		}
		if err := emit("Connected · loading recent and archived chats…", nil); err != nil {
			return err
		}
		source := transport.New(child, client, kv, viper.GetInt("pool"), viper.GetInt("threads"), viper.GetDuration("reconnect-timeout"), time.Time{}, time.Time{})
		defer source.Close(true)
		return source.StreamDialogs(child, []string{"personal", "bots", "groups", "channels"}, func(page []archive.Chat) error {
			rows := make([]*chat.Dialog, 0, len(page))
			for _, value := range page {
				kind := "private"
				if strings.HasSuffix(value.Type, "_channel") {
					kind = "channel"
				} else if strings.Contains(value.Type, "group") {
					kind = "group"
				}
				row := &chat.Dialog{ID: value.ID, Type: kind, VisibleName: value.Name, Username: value.Username}
				accepted, err := texpr.Run(program, row)
				if err != nil {
					return fmt.Errorf("chat filter: %w", err)
				}
				if accepted.(bool) {
					rows = append(rows, row)
				}
			}
			count += len(rows)
			return emit(fmt.Sprintf("%d chats loaded · fetching next page…", count), rows)
		})
	})
	if err != nil {
		if parent.Err() != nil {
			return parent.Err()
		}
		if ctx.Err() != nil || connectCtx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("chat loading timed out after %d chats; check your connection or set --proxy / TA_PROXY / ALL_PROXY, then retry", count)
		}
		return err
	}
	return emit(fmt.Sprintf("Loaded %d chats", count), nil)
}
