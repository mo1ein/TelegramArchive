package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"ta/internal/archive"
)

func NewRender() *cobra.Command {
	var format string
	var partSize int64
	var pageSize int
	cmd := &cobra.Command{Use: "render ARCHIVE_DIRECTORY", Short: "Render JSON or HTML from an existing local checkpoint", Annotations: map[string]string{"offline": "true"}, Args: cobra.ExactArgs(1)}
	cmd.Flags().StringVar(&format, "format", "html", "json, html or both")
	cmd.Flags().Int64Var(&partSize, "part-size", 0, "maximum JSON part size in bytes")
	cmd.Flags().IntVar(&pageSize, "html-page-size", 1000, "maximum messages per HTML page")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		result, err := archive.Render(cmd.Context(), args[0], format, partSize, pageSize)
		if err != nil {
			return err
		}
		for _, p := range result.Paths {
			fmt.Fprintln(cmd.OutOrStdout(), p)
		}
		return nil
	}
	return cmd
}
