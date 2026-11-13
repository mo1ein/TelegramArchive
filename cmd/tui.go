package cmd

import (
	"context"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/mo1ein/TelegramArchive/internal/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

func NewTUI() *cobra.Command {
	return &cobra.Command{
		Use: "tui", Short: "Open the interactive command workspace", Args: cobra.NoArgs,
		Annotations: map[string]string{"offline": "true"},
		RunE:        runTUI,
	}
}

func terminalFile(stream any) bool {
	file, ok := stream.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

func runTUI(cmd *cobra.Command, _ []string) error {
	if !terminalFile(cmd.InOrStdin()) || !terminalFile(cmd.OutOrStdout()) {
		return fmt.Errorf("the interactive workspace needs a terminal; run ta tui in a terminal, or use ta --help for commands")
	}
	root := cmd.Root()
	root.InitDefaultCompletionCmd()
	taskCtx, cancel := context.WithCancel(cmd.Context())
	defer cancel()
	model := tui.New(tui.Catalog(root), inheritedArguments(cmd), viper.GetString("ns"), taskCtx)
	_, err := tea.NewProgram(model, tea.WithContext(cmd.Context()), tea.WithInput(cmd.InOrStdin()), tea.WithOutput(cmd.OutOrStdout())).Run()
	return err
}

func runDefault(cmd *cobra.Command, args []string) error {
	if terminalFile(cmd.InOrStdin()) && terminalFile(cmd.OutOrStdout()) {
		return runTUI(cmd, args)
	}
	// Piped invocations retain useful, script-friendly help output.
	return cmd.Help()
}
