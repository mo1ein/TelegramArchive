package tui

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
)

type AccountTask struct {
	Account, Directory string
	Args               []string
}

func (f Form) withAccounts(plan Plan) (Plan, error) {
	field := f.field("ns")
	if field == nil || field.Type != "accounts" {
		return plan, nil
	}
	if err := validate(*field); err != nil {
		return plan, field.Error(err)
	}
	names, err := readCSV(field.Value)
	if err != nil || len(names) == 0 {
		return plan, fmt.Errorf("choose at least one saved account")
	}
	seen := map[string]bool{}
	accounts := make([]string, 0, len(names))
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			return plan, fmt.Errorf("choose a saved account")
		}
		if !seen[name] {
			accounts = append(accounts, name)
			seen[name] = true
		}
	}
	plan.Args = replaceArgument(plan.Args, "ns", accounts[0])
	if len(accounts) == 1 {
		return plan, nil
	}
	if every := f.field("every"); every != nil {
		interval, _ := time.ParseDuration(every.Value)
		if interval > 0 {
			return plan, fmt.Errorf("multi-account backups run once; set the repeat interval to 0 and use scheduled backups for recurring runs")
		}
	}
	output := f.field("output")
	if output == nil {
		return plan, fmt.Errorf("this workflow does not support multiple backup accounts")
	}
	root := output.Value
	if root == "" {
		root = "data"
	}
	for _, account := range accounts {
		directory := filepath.Join(root, "accounts", accountFolder(account))
		args := replaceArgument(plan.Args, "ns", account)
		args = replaceArgument(args, "output", directory)
		plan.Tasks = append(plan.Tasks, AccountTask{Account: account, Directory: directory, Args: args})
	}
	return plan, nil
}

func accountFolder(name string) string {
	label := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, name)
	label = strings.Trim(label, "-")
	if label == "" {
		label = "account"
	}
	if runes := []rune(label); len(runes) > 36 {
		label = string(runes[:36])
	}
	digest := sha256.Sum256([]byte(name))
	return fmt.Sprintf("%s-%x", label, digest[:6])
}

func replaceArgument(args []string, name, value string) []string {
	result := make([]string, 0, len(args)+1)
	flag := "--" + name
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], flag+"=") {
			continue
		}
		if args[i] == flag && i+1 < len(args) {
			i++
			continue
		}
		result = append(result, args[i])
	}
	return append(slices.Clip(result), flag+"="+value)
}
