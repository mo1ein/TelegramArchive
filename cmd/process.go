package cmd

import (
	"context"
	"encoding/csv"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func flagArgument(f *pflag.Flag) []string {
	if v, ok := f.Value.(pflag.SliceValue); ok {
		if f.Value.Type() == "stringArray" {
			var args []string
			for _, value := range v.GetSlice() {
				args = append(args, "--"+f.Name+"="+value)
			}
			return args
		}
		return []string{"--" + f.Name + "=" + csvValues(v.GetSlice())}
	}
	value := f.Value.String()
	if f.Value.Type() == "stringToString" {
		value = strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
	}
	return []string{"--" + f.Name + "=" + value}
}

func csvValues(values []string) string {
	var b strings.Builder
	w := csv.NewWriter(&b)
	w.Write(values)
	w.Flush()
	return strings.TrimSuffix(b.String(), "\n")
}

func inheritedArguments(cmd *cobra.Command) []string {
	var args []string
	cmd.Root().PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if f.Changed {
			args = append(args, flagArgument(f)...)
		}
	})
	return args
}

func runChild(ctx context.Context, cmd *cobra.Command, args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// Each child releases its session/storage locks before the next phase.
	child := exec.CommandContext(ctx, exe, args...)
	child.Stdout, child.Stderr, child.Stdin = cmd.OutOrStdout(), cmd.ErrOrStderr(), cmd.InOrStdin()
	child.WaitDelay = 30 * time.Second
	child.Cancel = func() error {
		if err := child.Process.Signal(syscall.SIGTERM); err != nil {
			return child.Process.Kill()
		}
		return nil
	}
	return child.Run()
}
