package cmd

import (
	"context"
	"encoding/json"
	"errors"
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
		command.Flags().Bool("check", false, "verify imported session data without saving a login or connecting to Telegram")
		command.Flags().Lookup("desktop").Usage = "Telegram Desktop tdata or native macOS data directory; auto-detect if empty"
		command.Short = "Log in to the selected Telegram account namespace"
		command.Long = "Log in to the selected namespace. Desktop session import is the default and supports Telegram Desktop tdata and the native Telegram for macOS client. Use --check to verify session data without saving it, -T qr for QR login, or -T code for phone/code login. Use ta accounts to see saved accounts and -n <namespace> to choose one."
		command.RunE = func(cmd *cobra.Command, args []string) error {
			check, _ := cmd.Flags().GetBool("check")
			mode := cmd.Flags().Lookup("type")
			legacy, _ := cmd.Flags().GetBool("code")
			if check && (mode == nil || mode.Value.String() != "desktop" || legacy) {
				return fmt.Errorf("--check applies to Desktop/native session imports; omit -T qr, -T code and --code")
			}
			native, err := prepareDesktopLogin(cmd)
			if err != nil {
				return err
			}
			if native {
				return runNativeLogin(cmd, check)
			}
			if check {
				return checkDesktopLogin(cmd)
			}
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
	return newAccounts(func(ctx context.Context) ([]string, error) {
		return sessionkv.From(ctx).Namespaces()
	}, readSavedAccount)
}

func newAccounts(list func(context.Context) ([]string, error), lookup func(context.Context, string) (int64, string, error)) *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use:   "accounts",
		Short: "List saved Telegram accounts and their namespaces",
		Long:  "List saved Telegram accounts. Busy sessions are marked and skipped. Choose an account with -n <namespace>.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if asJSON {
				// Keep partial rows machine-readable even when one account fails.
				cmd.SilenceUsage = true
			}
			namespaces, err := list(cmd.Context())
			if err != nil {
				return err
			}
			selected := viper.GetString("ns")
			var failures []error
			rows := []map[string]any{}
			for _, namespace := range namespaces {
				if err := cmd.Context().Err(); err != nil {
					return err
				}
				marker := ""
				if namespace == selected {
					marker = " [selected]"
				}
				id, username, err := lookup(cmd.Context(), namespace)
				if canceled := cmd.Context().Err(); canceled != nil {
					return canceled
				}
				if err != nil {
					var busy *sessionBusyError
					if errors.As(err, &busy) {
						if asJSON {
							rows = append(rows, map[string]any{"namespace": namespace, "status": "busy"})
						} else {
							fmt.Fprintf(cmd.OutOrStdout(), "%s  [busy]%s\n", namespace, marker)
						}
						continue
					}
					if asJSON {
						rows = append(rows, map[string]any{"namespace": namespace, "status": "unavailable"})
					} else {
						fmt.Fprintf(cmd.OutOrStdout(), "%s  [unavailable]%s\n", namespace, marker)
					}
					failures = append(failures, fmt.Errorf("read account in namespace %q: %w", namespace, err))
					continue
				}
				if asJSON {
					rows = append(rows, map[string]any{"namespace": namespace, "id": id, "username": username, "status": "ready"})
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "%s  %d  %s%s\n", namespace, id, usernameOrPlaceholder(username), marker)
				}
			}
			if len(namespaces) == 0 && !asJSON {
				fmt.Fprintln(cmd.OutOrStdout(), "No saved accounts. Log in with ta login -T qr.")
			}
			if asJSON {
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(rows); err != nil {
					return err
				}
			}
			return errors.Join(failures...)
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "output saved accounts as JSON")
	return command
}

func readSavedAccount(ctx context.Context, namespace string) (int64, string, error) {
	selected := viper.GetString("ns")
	viper.Set("ns", namespace)
	defer viper.Set("ns", selected)
	var id int64
	var username string
	err := tRun(ctx, func(ctx context.Context, client *tdclient.Client, kv storage.Storage) error {
		source := archiveTelegram.New(ctx, client, kv, viper.GetInt("pool"), viper.GetInt("threads"), viper.GetDuration("reconnect-timeout"), time.Time{}, time.Time{})
		defer source.Close(true)
		if err := source.Identity(ctx); err != nil {
			return err
		}
		id, username = source.AccountID(), source.Username()
		return nil
	})
	return id, username, err
}

func usernameOrPlaceholder(username string) string {
	if username == "" {
		return "(no username)"
	}
	return "@" + username
}
