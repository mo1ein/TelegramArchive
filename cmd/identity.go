package cmd

import (
	"context"
	"fmt"
	"time"

	tdclient "github.com/gotd/td/telegram"
	"github.com/iyear/tdl/core/storage"
	sessionkv "github.com/iyear/tdl/pkg/kv"
	archiveTelegram "github.com/mo1ein/TelegramArchive/internal/telegram"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func wrapLoginIdentity(root *cobra.Command) {
	for _, command := range root.Commands() {
		if command.Name() != "login" || command.RunE == nil {
			continue
		}
		login := command.RunE
		command.Short = "Log in to the selected Telegram account namespace"
		command.Long = "Log in to the selected namespace. Use ta accounts to see saved accounts and -n <namespace> to choose one."
		command.RunE = func(cmd *cobra.Command, args []string) error {
			if err := login(cmd, args); err != nil {
				return err
			}
			return printCurrentIdentity(cmd)
		}
		return
	}
}

func printCurrentIdentity(cmd *cobra.Command) error {
	return tRun(cmd.Context(), func(ctx context.Context, client *tdclient.Client, kv storage.Storage) error {
		source := archiveTelegram.New(ctx, client, kv, viper.GetInt("pool"), viper.GetInt("threads"), viper.GetDuration("reconnect-timeout"), time.Time{}, time.Time{})
		defer source.Close(true)
		if err := source.Identity(ctx); err != nil {
			return fmt.Errorf("login succeeded, but account identity could not be read: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Telegram account: %d  %s  (namespace %s)\n", source.AccountID(), usernameOrPlaceholder(source.Username()), viper.GetString("ns"))
		return nil
	})
}

func NewAccounts() *cobra.Command {
	return &cobra.Command{
		Use:   "accounts",
		Short: "List saved Telegram accounts and their namespaces",
		Long:  "List saved Telegram accounts. Choose one for a command with -n <namespace>.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			namespaces, err := sessionkv.From(cmd.Context()).Namespaces()
			if err != nil {
				return err
			}
			selected := viper.GetString("ns")
			for _, namespace := range namespaces {
				viper.Set("ns", namespace)
				err := tRun(cmd.Context(), func(ctx context.Context, client *tdclient.Client, kv storage.Storage) error {
					source := archiveTelegram.New(ctx, client, kv, viper.GetInt("pool"), viper.GetInt("threads"), viper.GetDuration("reconnect-timeout"), time.Time{}, time.Time{})
					defer source.Close(true)
					if err := source.Identity(ctx); err != nil {
						return err
					}
					marker := ""
					if namespace == selected {
						marker = " [selected]"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s  %d  %s%s\n", namespace, source.AccountID(), usernameOrPlaceholder(source.Username()), marker)
					return nil
				})
				if err != nil {
					return fmt.Errorf("read account in namespace %q: %w", namespace, err)
				}
			}
			if len(namespaces) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No saved accounts. Log in with ta login -T qr.")
			}
			return nil
		},
	}
}

func usernameOrPlaceholder(username string) string {
	if username == "" {
		return "(no username)"
	}
	return "@" + username
}
