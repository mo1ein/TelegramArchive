package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/iyear/tdl/app/migrate"
	upstreamcmd "github.com/iyear/tdl/cmd"
	upstreamconsts "github.com/iyear/tdl/pkg/consts"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"ta/consts"
)

func configureProductPaths() {
	upstreamconsts.DataDir = consts.DataDir
	upstreamconsts.LogPath = consts.LogPath
	upstreamconsts.ExtensionsPath = filepath.Join(consts.DataDir, "extensions")
	upstreamconsts.ExtensionsDataPath = filepath.Join(upstreamconsts.ExtensionsPath, "data")
	upstreamcmd.DefaultBoltStorage = map[string]string{"type": "bolt", "path": filepath.Join(consts.DataDir, "data")}
}

func bindProductEnvironment(root *cobra.Command) {
	root.PersistentFlags().VisitAll(func(flag *pflag.Flag) {
		key := strings.ToUpper(strings.ReplaceAll(flag.Name, "-", "_"))
		// Keep existing installations working while TA_* is the primary interface.
		_ = viper.BindEnv(flag.Name, "TA_"+key, "TDL_"+key)
	})
}

func productSetting(key string) (string, bool) {
	for _, prefix := range []string{"TA_", "TDL_"} {
		if value, ok := os.LookupEnv(prefix + key); ok && strings.TrimSpace(value) != "" {
			return value, true
		}
	}
	return "", false
}

func defaultProductStorage(home string) map[string]string {
	for _, directory := range []string{".ta", ".tdl"} {
		for _, candidate := range []struct{ driver, name string }{{"bolt", "data"}, {"legacy", "data.kv"}} {
			path := filepath.Join(home, directory, candidate.name)
			if _, err := os.Stat(path); err == nil {
				return map[string]string{"type": candidate.driver, "path": path}
			}
		}
	}
	return map[string]string{"type": "bolt", "path": filepath.Join(home, ".ta", "data")}
}

func prepareProductStorage(cmd *cobra.Command) error {
	if err := os.MkdirAll(consts.DataDir, 0700); err != nil {
		return fmt.Errorf("create ta data directory: %w", err)
	}
	if _, configured := productSetting("STORAGE"); !configured && !cmd.Flags().Changed("storage") {
		viper.Set("storage", defaultProductStorage(consts.HomeDir))
	}
	return nil
}

var helpBrand = strings.NewReplacer("tdl", "ta", "TDL", "TA")
var runtimeBrand = strings.NewReplacer(
	"Export only generates minimal JSON for tdl download, not for backup.", "Export only generates minimal JSON for ta download, not for backup.",
	"can't be automatically upgraded by tdl", "can't be automatically upgraded by ta",
	"only GitHub extension can be upgraded by tdl", "only GitHub extension can be upgraded by ta",
)

// Rebrand product messages without rewriting usernames, paths, or dependency URLs.
type productOutput struct{ io.Writer }

func (w productOutput) Write(p []byte) (int, error) {
	message := runtimeBrand.Replace(string(p))
	n, err := io.WriteString(w.Writer, message)
	if err != nil {
		return 0, err
	}
	if n != len(message) {
		return 0, io.ErrShortWrite
	}
	return len(p), nil
}

type productError struct{ error }

func (e productError) Error() string { return runtimeBrand.Replace(e.error.Error()) }
func (e productError) Unwrap() error { return e.error }

func brandCommands(command *cobra.Command) {
	command.Short = helpBrand.Replace(command.Short)
	command.Long = helpBrand.Replace(command.Long)
	command.Example = helpBrand.Replace(command.Example)
	command.Deprecated = helpBrand.Replace(command.Deprecated)
	command.Flags().VisitAll(func(flag *pflag.Flag) {
		flag.Usage = helpBrand.Replace(flag.Usage)
		if flag.DefValue == "tdl-export.json" || flag.DefValue == "tdl-users.json" {
			flag.DefValue = helpBrand.Replace(flag.DefValue)
			_ = flag.Value.Set(flag.DefValue)
		}
	})
	if run := command.RunE; run != nil {
		command.RunE = func(cmd *cobra.Command, args []string) error {
			if cmd.Name() == "backup" && cmd.Parent() == cmd.Root() && cmd.Flags().Lookup("dst").Value.String() == "" {
				if err := cmd.Flags().Set("dst", time.Now().Format("2006-01-02-15_04_05")+".backup.ta"); err != nil {
					return err
				}
			}
			if err := run(cmd, args); err != nil {
				return productError{err}
			}
			return nil
		}
	}
	if _, branded := color.Output.(productOutput); !branded {
		color.Output = productOutput{color.Output}
	}
	for _, child := range command.Commands() {
		brandCommands(child)
	}
}

func newSessionRecover() *cobra.Command {
	var file string
	command := &cobra.Command{
		Use: "recover", Short: "Recover ta session data", GroupID: "account", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return migrate.Recover(cmd.Context(), file) },
	}
	command.Flags().StringVarP(&file, "file", "f", "", "session backup file path")
	_ = command.MarkFlagRequired("file")
	_ = command.MarkFlagFilename("file", "ta", "tdl")
	return command
}
