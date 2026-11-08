package cmd

import (
	upstreamcmd "github.com/iyear/tdl/cmd"
	sessionkv "github.com/iyear/tdl/pkg/kv"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Extend the transfer commands without copying authentication and transfer code.
func New() *cobra.Command {
	configureProductPaths()
	root := upstreamcmd.New()
	root.Use = "ta"
	root.Short = "Back up Telegram chats and account data as JSON or offline HTML"
	root.Version = version()
	for _, command := range root.Commands() {
		if command.Name() == "version" || command.Name() == "recover" {
			root.RemoveCommand(command)
		}
	}
	root.PersistentFlags().Lookup("ns").Usage = "saved Telegram login; concurrent terminals need separate namespaces (the same account is supported); list with ta accounts"
	root.AddCommand(NewArchive(), NewStats(), NewRender(), NewSchedule(), NewAccount(), NewSync(), NewCron(), NewAccounts(), NewVersion(), newSessionRecover())
	viper.SetEnvPrefix("TA")
	bindProductEnvironment(root)
	brandCommands(root)
	wrapLoginIdentity(root)
	pre, post := root.PersistentPreRunE, root.PersistentPostRunE
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if cmd.Name() == "stats" || cmd.Annotations["offline"] == "true" {
			return nil
		}
		if err := prepareProductStorage(cmd); err != nil {
			return err
		}
		// Session compatibility is handled above, including the old legacy driver.
		// Bypass the dependency's migration check against its original data path.
		flag := cmd.Flags().Lookup("storage")
		changed := flag.Changed
		flag.Changed = true
		defer func() { flag.Changed = changed }()
		if err := pre(cmd, args); err != nil {
			return explainSessionLock(err, viper.GetStringMapString("storage")["type"], "")
		}
		cmd.SetContext(sessionkv.With(cmd.Context(), sessionStorage{sessionkv.From(cmd.Context())}))
		return nil
	}
	root.PersistentPostRunE = func(cmd *cobra.Command, args []string) error {
		if cmd.Name() == "stats" || cmd.Annotations["offline"] == "true" {
			return nil
		}
		return post(cmd, args)
	}
	return root
}
