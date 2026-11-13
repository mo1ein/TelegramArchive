package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/mo1ein/TelegramArchive/consts"
	managedcron "github.com/mo1ein/TelegramArchive/internal/cron"
	"github.com/mo1ein/TelegramArchive/internal/schedule"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func NewCron() *cobra.Command {
	return newCron(managedcron.Manager{Directory: filepath.Join(consts.DataDir, "cron"), Table: managedcron.CommandTable{}}, managedcron.Execute)
}

func newCron(manager managedcron.Manager, execute managedcron.Runner) *cobra.Command {
	group := &cobra.Command{Use: "cron", Short: "Install and manage backups in your system crontab", Annotations: map[string]string{"offline": "true"}}
	var runNow, atStartup bool
	add := newSyncPlan(nil, func(cmd *cobra.Command, dir string, timing schedule.Options) error {
		name := cmd.Flags().Args()[0]
		if err := managedcron.ValidateName(name); err != nil {
			return err
		}
		if cmd.Flags().Changed("runs") || cmd.Flags().Changed("stop-on-error") {
			return fmt.Errorf("cron runs one backup per trigger; omit --runs and --stop-on-error")
		}
		expression, err := managedcron.Schedule(timing.Every)
		if err != nil {
			return err
		}
		args, environment, err := cronArguments(cmd)
		if err != nil {
			return err
		}
		working, err := os.Getwd()
		if err != nil {
			return err
		}
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		job := managedcron.Job{Name: name, Schedule: expression, AtStartup: atStartup, Executable: executable, Directory: working, Arguments: args, Environment: environment, Log: filepath.Join(manager.Directory, name+".log")}
		if err := manager.Install(cmd.Context(), job); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Installed cron job %q: %s (cron's timezone)\nOutput: %s\nLog: %s\nTa can exit; cron will launch each backup.\n", name, expression, dir, job.Log)
		if atStartup {
			fmt.Fprintln(cmd.OutOrStdout(), "Also runs at system startup, waiting for a busy output folder.")
		}
		if runNow {
			skipBusy, _ := cmd.Flags().GetBool("skip-busy")
			return managedcron.Run(cmd.Context(), manager.File(name), cmd.OutOrStdout(), cronRunWithSkipBusy(execute, cmd.Flags().Changed("skip-busy") && skipBusy))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Run the first backup now with: ta cron run %s\n", name)
		return nil
	})
	add.Use = "add NAME"
	add.Short = "Install or update a cron backup using sync flags"
	add.Long = "Save one backup job in your user crontab, then exit. Each trigger runs a single incremental sync.\nAbsolute paths and current login/proxy settings are saved privately.\nUse --run-now for an initial backup immediately after installation."
	add.Example = "  ta cron add my-channel --chat channel_username --every 6h --media all --format both --run-now\n  ta cron add everything --everything --every 24h --format both"
	add.Args = cobra.ExactArgs(1)
	add.Flags().Lookup("every").Value.Set("6h")
	add.Flags().Lookup("every").DefValue = "6h"
	add.Flags().Lookup("every").Usage = "cron interval: minutes dividing an hour, hours dividing a day, or 24h"
	add.Flags().BoolVar(&runNow, "run-now", false, "also run the first backup immediately after installing cron")
	add.Flags().BoolVar(&atStartup, "at-startup", false, "also run a backup at system startup to catch up after downtime")
	add.Flags().MarkHidden("runs")
	add.Flags().MarkHidden("stop-on-error")

	var listJSON bool
	list := &cobra.Command{Use: "list", Short: "List ta's cron backups", Args: cobra.NoArgs, Annotations: map[string]string{"offline": "true"}, RunE: func(cmd *cobra.Command, _ []string) error {
		jobs, err := manager.List(cmd.Context())
		if err != nil {
			return err
		}
		if listJSON {
			rows := make([]map[string]any, 0, len(jobs))
			for _, job := range jobs {
				rows = append(rows, map[string]any{"name": job.Name, "schedule": job.Schedule, "installed": job.Installed, "log": job.Log})
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(rows)
		}
		if len(jobs) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No ta cron jobs. Add one with: ta cron add NAME --chat channel_username --every 6h")
			return nil
		}
		out := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		fmt.Fprintln(out, "NAME\tSCHEDULE\tSTATUS\tLOG")
		for _, job := range jobs {
			status := "installed"
			if !job.Installed {
				status = "not in crontab"
			}
			expression := job.Schedule
			if job.AtStartup {
				expression += " + startup"
			}
			fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", job.Name, expression, status, job.Log)
		}
		return out.Flush()
	}}
	list.Flags().BoolVar(&listJSON, "json", false, "output scheduled backups as JSON")
	remove := &cobra.Command{Use: "remove NAME", Aliases: []string{"rm"}, Short: "Remove a cron backup; keep its archives and logs", Args: cobra.ExactArgs(1), Annotations: map[string]string{"offline": "true"}, RunE: func(cmd *cobra.Command, args []string) error {
		if err := manager.Remove(cmd.Context(), args[0]); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Removed cron job %q. Archives and logs are retained; an active run may finish.\n", args[0])
		return nil
	}}
	var file string
	var skipBusy bool
	run := &cobra.Command{Use: "run NAME", Short: "Run an installed cron backup now", Args: cobra.MaximumNArgs(1), Annotations: map[string]string{"offline": "true"}, RunE: func(cmd *cobra.Command, args []string) error {
		jobFile := file
		if jobFile != "" {
			if len(args) > 0 {
				return fmt.Errorf("use NAME or --file, not both")
			}
		} else {
			if len(args) != 1 {
				return fmt.Errorf("specify a cron job name")
			}
			if err := managedcron.ValidateName(args[0]); err != nil {
				return err
			}
			jobFile = manager.File(args[0])
		}
		runner := execute
		if file == "" || cmd.Flags().Changed("skip-busy") {
			runner = cronRunWithSkipBusy(execute, skipBusy)
		} else {
			// Older saved jobs predate --skip-busy. Automatic runs must still
			// skip an occupied output folder instead of building a queue.
			runner = func(ctx context.Context, job managedcron.Job, output io.Writer) error {
				for _, arg := range job.Arguments {
					if arg == "--skip-busy" || strings.HasPrefix(arg, "--skip-busy=") {
						return execute(ctx, job, output)
					}
				}
				return cronRunWithSkipBusy(execute, true)(ctx, job, output)
			}
		}
		return managedcron.Run(cmd.Context(), jobFile, cmd.OutOrStdout(), runner)
	}}
	run.Flags().StringVar(&file, "file", "", "saved cron job path")
	run.Flags().MarkHidden("file")
	run.Flags().BoolVar(&skipBusy, "skip-busy", false, "skip a busy output folder instead of waiting")
	logs := &cobra.Command{Use: "logs NAME", Short: "Show recent output from a cron backup", Args: cobra.ExactArgs(1), Annotations: map[string]string{"offline": "true"}, RunE: func(cmd *cobra.Command, args []string) error {
		if err := managedcron.ValidateName(args[0]); err != nil {
			return err
		}
		log, err := os.Open(filepath.Join(manager.Directory, args[0]+".log"))
		if os.IsNotExist(err) {
			fmt.Fprintln(cmd.OutOrStdout(), "No runs logged yet.")
			return nil
		}
		if err != nil {
			return err
		}
		defer log.Close()
		info, err := log.Stat()
		if err != nil {
			return err
		}
		offset := max(0, info.Size()-64*1024)
		if _, err = log.Seek(offset, io.SeekStart); err != nil {
			return err
		}
		b, err := io.ReadAll(io.LimitReader(log, 64*1024))
		if err != nil {
			return err
		}
		lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
		if offset > 0 && len(lines) > 1 {
			lines = lines[1:]
		}
		if len(lines) > 100 {
			lines = lines[len(lines)-100:]
		}
		fmt.Fprintln(cmd.OutOrStdout(), strings.Join(lines, "\n"))
		return nil
	}}
	group.AddCommand(add, list, remove, run, logs)
	return group
}

func cronRunWithSkipBusy(execute managedcron.Runner, skipBusy bool) managedcron.Runner {
	return func(ctx context.Context, job managedcron.Job, output io.Writer) error {
		args := slices.DeleteFunc(slices.Clone(job.Arguments), func(arg string) bool {
			return arg == "--skip-busy" || strings.HasPrefix(arg, "--skip-busy=")
		})
		job.Arguments = append(args, fmt.Sprintf("--skip-busy=%t", skipBusy))
		return execute(ctx, job, output)
	}
}

// Freeze effective settings rather than relying on cron inheriting a terminal's
// environment. Proxy credentials stay in the 0600 job file, not in crontab.
func cronArguments(cmd *cobra.Command) ([]string, map[string]string, error) {
	for _, key := range []string{"threads", "pool", "limit"} {
		value := viper.GetInt(key)
		if value < 1 || value > 64 {
			return nil, nil, fmt.Errorf("%s must be between 1 and 64", key)
		}
	}
	var args []string
	var optionErr error
	cmd.Root().PersistentFlags().VisitAll(func(flag *pflag.Flag) {
		if flag.Deprecated != "" || flag.Name == "proxy" || flag.Name == flagResultFile {
			return
		}
		value := ""
		switch flag.Value.Type() {
		case "string":
			value = viper.GetString(flag.Name)
		case "bool":
			value = fmt.Sprint(viper.GetBool(flag.Name))
		case "int":
			value = fmt.Sprint(viper.GetInt(flag.Name))
		case "duration":
			value = viper.GetDuration(flag.Name).String()
		case "stringToString":
			settings := viper.GetStringMapString(flag.Name)
			if flag.Name == "storage" {
				if _, set := productSetting("STORAGE"); !set && !flag.Changed {
					settings = defaultProductStorage(consts.HomeDir)
				}
				if path := settings["path"]; path != "" {
					absolute, err := filepath.Abs(path)
					if err != nil {
						optionErr = err
						return
					}
					settings["path"] = absolute
				}
			}
			var pairs []string
			for key, item := range settings {
				pairs = append(pairs, key+"="+item)
			}
			sort.Strings(pairs)
			value = csvValues(pairs)
		default:
			value = flag.Value.String()
		}
		args = append(args, "--"+flag.Name+"="+value)
	})
	if optionErr != nil {
		return nil, nil, optionErr
	}
	args = append(args, "sync")
	cmd.LocalNonPersistentFlags().VisitAll(func(flag *pflag.Flag) {
		switch flag.Name {
		case "every", "runs", "stop-on-error", "run-now", "at-startup", "help":
			return
		}
		// These flags enable account backup / override its takeout default by
		// their presence, so preserve whether the user actually supplied them.
		if (flag.Name == "account-include" || flag.Name == "takeout") && !flag.Changed {
			return
		}
		args = append(args, flagArgument(flag)...)
	})
	environment := map[string]string{"HOME": consts.HomeDir, "TA_PROXY": telegramProxy(), "TDL_PROXY": "", "NO_COLOR": "1", "TERM": "dumb"}
	for _, key := range []string{"ALL_PROXY", "all_proxy", "HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		environment[key] = ""
	}
	for _, key := range []string{"PATH", "TZ", "LANG", "LC_ALL", "LC_CTYPE"} {
		if value, ok := os.LookupEnv(key); ok {
			environment[key] = value
		}
	}
	return args, environment, nil
}
