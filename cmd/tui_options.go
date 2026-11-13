package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/mo1ein/TelegramArchive/internal/archive"
	transport "github.com/mo1ein/TelegramArchive/internal/telegram"
	"github.com/spf13/cobra"
	"go.etcd.io/bbolt"
)

func annotateTUIChoices(root *cobra.Command) {
	options := map[string][]string{
		"media":           append([]string{"all", "none"}, archive.MediaKinds...),
		"types":           transport.ChatTypes,
		"include":         append([]string{"all"}, transport.AccountKinds...),
		"account-include": append([]string{"all"}, transport.AccountKinds...),
	}
	for _, path := range []string{"archive", "account", "sync", "cron add"} {
		command, _, err := root.Find(strings.Fields(path))
		if err != nil {
			continue
		}
		for name, values := range options {
			if flag := command.Flags().Lookup(name); flag != nil {
				if flag.Annotations == nil {
					flag.Annotations = map[string][]string{}
				}
				flag.Annotations["ta:choices"] = slices.Clone(values)
			}
		}
	}
}

// Discover names locally without authorizing a Telegram connection or opening
// any per-account session. Legacy databases are opened read-only and briefly.
func tuiSavedAccounts(options map[string]string) ([]string, error) {
	path := options["path"]
	if path == "" {
		return nil, fmt.Errorf("session storage has no path")
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var names []string
	switch options["type"] {
	case "bolt":
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.Type().IsRegular() && !strings.HasPrefix(entry.Name(), ".") {
				names = append(names, entry.Name())
			}
		}
	case "file":
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		var values map[string]json.RawMessage
		if err := json.NewDecoder(io.LimitReader(file, 32<<20)).Decode(&values); err != nil {
			return nil, fmt.Errorf("read saved account names: %w", err)
		}
		for name := range values {
			names = append(names, name)
		}
	case "legacy":
		db, err := bbolt.Open(path, 0600, &bbolt.Options{ReadOnly: true, Timeout: 150 * time.Millisecond})
		if err != nil {
			return nil, fmt.Errorf("read saved account names: %w", err)
		}
		defer db.Close()
		if err := db.View(func(tx *bbolt.Tx) error {
			return tx.ForEach(func(name []byte, _ *bbolt.Bucket) error {
				names = append(names, string(name))
				return nil
			})
		}); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported session storage %q", options["type"])
	}
	slices.Sort(names)
	return slices.Compact(names), nil
}
