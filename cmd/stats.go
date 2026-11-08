package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"ta/internal/archive"
)

func NewStats() *cobra.Command {
	return &cobra.Command{Use: "stats result.json [result_part*.json ...]", Short: "Calculate statistics offline from chat JSON exports", Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var stats archive.Stats
		for _, name := range args {
			f, err := os.Open(name)
			if err != nil {
				return err
			}
			err = archive.ReadStats(cmd.Context(), f, &stats)
			closeErr := f.Close()
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			if closeErr != nil {
				return closeErr
			}
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(stats)
	}}
}
