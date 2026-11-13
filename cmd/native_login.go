package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/AlecAivazis/survey/v2"
	"github.com/gotd/td/session"
	"github.com/gotd/td/session/tdesktop"
	"github.com/iyear/tdl/core/storage"
	"github.com/iyear/tdl/pkg/key"
	sessionkv "github.com/iyear/tdl/pkg/kv"
	client "github.com/iyear/tdl/pkg/tclient"
	"github.com/iyear/tdl/pkg/tpath"
	"github.com/mo1ein/TelegramArchive/consts"
	"github.com/mo1ein/TelegramArchive/internal/macsession"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const nativeApp = "macos-native"

// The direct-download native client uses this public application profile.
// Keeping it with the saved session avoids switching its API identity to tdata's.
func configureNativeApp() {
	client.Apps[nativeApp] = client.App{AppID: 9, AppHash: "3975f648bb682ee889f35483bc618d1c"}
}

func offlineCommand(cmd *cobra.Command) bool {
	if cmd.Name() == "stats" || cmd.Annotations["offline"] == "true" {
		return true
	}
	if cmd.Name() == "login" {
		check, _ := cmd.Flags().GetBool("check")
		return check
	}
	return false
}

func runNativeLogin(cmd *cobra.Command, check bool) error {
	path, _ := cmd.Flags().GetString("desktop")
	passcode, _ := cmd.Flags().GetString("passcode")
	var prompt func() (string, error)
	if terminalFile(cmd.InOrStdin()) && terminalFile(cmd.ErrOrStderr()) {
		prompt = func() (string, error) {
			var value string
			err := survey.AskOne(&survey.Password{Message: "Telegram local app passcode (not your two-step verification password):"}, &value,
				survey.WithStdio(cmd.InOrStdin().(*os.File), cmd.ErrOrStderr().(*os.File), cmd.ErrOrStderr().(*os.File)))
			return value, err
		}
	}
	accounts, err := readNativeAccounts(cmd.Context(), path, passcode, prompt)
	if err != nil {
		return fmt.Errorf("read native Telegram session: %w", err)
	}
	defer func() {
		for _, account := range accounts {
			account.Clear()
		}
	}()
	if check {
		fmt.Fprintf(cmd.OutOrStdout(), "Native Telegram session data verified: %d authorized account(s). No login saved; no network connection made.\n", len(accounts))
		return nil
	}
	selected := 0
	if len(accounts) > 1 {
		options := make([]string, len(accounts))
		for i, account := range accounts {
			options[i] = strconv.FormatInt(account.UserID, 10)
		}
		if err := survey.AskOne(&survey.Select{Message: "Choose a native Telegram account:", Options: options}, &selected); err != nil {
			return err
		}
	}
	if err := cmd.Context().Err(); err != nil {
		return err
	}
	namespace := viper.GetString("ns")
	kv, err := sessionkv.From(cmd.Context()).Open(namespace)
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.ErrOrStderr(), "WARN: Existing ta login data in this namespace will be overwritten.")
	loader := session.Loader{Storage: storage.NewSession(kv, true)}
	if err := loader.Save(cmd.Context(), accounts[selected].Data); err != nil {
		return err
	}
	if err := kv.Set(cmd.Context(), key.App(), []byte(nativeApp)); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Imported native Telegram account %d into namespace %q.\n", accounts[selected].UserID, namespace)
	fmt.Fprintln(cmd.OutOrStdout(), "Quit Telegram for macOS before running other ta commands with this imported login; both use the same authorization. The source app's files were left unchanged.")
	return nil
}

// Prompt only when the default key fails. Explicit passcodes and noninteractive
// invocations keep their existing behavior; a failed retry never loops.
func readNativeAccounts(ctx context.Context, path, passcode string, prompt func() (string, error)) ([]macsession.Account, error) {
	accounts, err := macsession.Read(ctx, path, passcode)
	if !errors.Is(err, macsession.ErrPasscode) || passcode != "" || prompt == nil {
		return accounts, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	passcode, err = prompt()
	if err != nil {
		return nil, err
	}
	if passcode == "" {
		return nil, macsession.ErrPasscode
	}
	return macsession.Read(ctx, path, passcode)
}

func checkDesktopLogin(cmd *cobra.Command) error {
	path, _ := cmd.Flags().GetString("desktop")
	passcode, _ := cmd.Flags().GetString("passcode")
	if path == "" {
		for _, candidate := range tpath.Desktop.AppData(consts.HomeDir) {
			accounts, err := tdesktop.Read(filepath.Join(candidate, "tdata"), []byte(passcode))
			if err == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Telegram Desktop session data verified: %d account(s). No login saved.\n", len(accounts))
				return nil
			}
		}
		return fmt.Errorf("no readable Telegram Desktop session data found; specify its directory with -d")
	}
	if filepath.Base(path) != "tdata" {
		path = filepath.Join(path, "tdata")
	}
	accounts, err := tdesktop.Read(path, []byte(passcode))
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Telegram Desktop session data verified: %d account(s). No login saved.\n", len(accounts))
	return nil
}
