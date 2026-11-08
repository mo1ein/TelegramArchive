package cmd

import (
	"github.com/spf13/cobra"
	"ta/core/logctx"
	"ta/login"
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
