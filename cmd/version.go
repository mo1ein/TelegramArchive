package cmd

import (
	"fmt"
	"runtime"

	"github.com/gotd/td/tg"
	"github.com/spf13/cobra"
	"ta/consts"
)

func NewVersion() *cobra.Command {
	versionCmd := &cobra.Command{
		Use:         "version",
		Short:       "Print the ta version",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"offline": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "ta %s\nCommit: %s\nDate: %s\n%s %s/%s\nTelegram API layer: %d\n", consts.Version, consts.Commit, consts.CommitDate, runtime.Version(), runtime.GOOS, runtime.GOARCH, tg.Layer)
			return err
		},
	}
	return versionCmd
}
