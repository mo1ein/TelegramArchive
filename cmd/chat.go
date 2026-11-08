package cmd

import "github.com/spf13/cobra"

func NewChat() *cobra.Command {
	chatCmd := &cobra.Command{
		Use:   "chat",
		Short: "Chat with Telegram",
		Long:  "This command allows you to chat with your Telegram contacts.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	return chatCmd
}
