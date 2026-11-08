package cmd

import (
	"github.com/mo1ein/TelegramArchive/core/logctx"
	"github.com/mo1ein/TelegramArchive/login"
	"github.com/spf13/cobra"
)

func NewLogin() *cobra.Command {
	var (
		//code bool
		opts login.Options
	)
	loginCmd := &cobra.Command{
		Use:   "login",
		Short: "Login to Telegram",
		Long:  "This command allows you to log in to your Telegram account using your phone number.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return login.Run(logctx.Named(cmd.Context(), "login"), opts)
		},
	}
	return loginCmd
}
