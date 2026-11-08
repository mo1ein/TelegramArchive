package cmd

import (
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/gotd/td/tg"
	"github.com/mo1ein/TelegramArchive/consts"
	"github.com/spf13/cobra"
)

func NewVersion() *cobra.Command {
	versionCmd := &cobra.Command{
		Use:         "version",
		Short:       "Print the ta version",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"offline": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "ta %s\nCommit: %s\nDate: %s\n%s %s/%s\nTelegram API layer: %d\n", version(), consts.Commit, consts.CommitDate, runtime.Version(), runtime.GOOS, runtime.GOARCH, tg.Layer)
			return err
		},
	}
	return versionCmd
}

func version() string {
	if consts.Version != "dev" {
		return consts.Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return consts.Version
}
