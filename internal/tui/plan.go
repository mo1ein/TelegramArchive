package tui

import (
	"encoding/csv"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kballard/go-shellquote"
)

type Plan struct {
	Title         string
	Args          []string
	Path          []string
	width, height int
}

func (f Form) Plan(base []string) (Plan, error) {
	plan := Plan{Title: f.Action.Title, Args: slices.Clone(base), Path: slices.Clone(f.Action.Path)}
	// Changed global fields replace startup flags, rather than repeating them in
	// the preview (or appending values to a string-slice setting).
	for _, field := range f.Fields {
		if !field.Global || !field.Dirty {
			continue
		}
		plan.Args = slices.DeleteFunc(plan.Args, func(arg string) bool {
			return strings.HasPrefix(arg, "--"+field.Name+"=")
		})
	}
	var local, positionals []string
	for _, field := range f.Fields {
		if field.Type == "arguments" {
			var err error
			positionals, err = shellquote.Split(field.Value)
			if err != nil {
				return plan, field.Error(err)
			}
			continue
		}
		if field.Required && (!field.Dirty || strings.TrimSpace(field.Value) == "") {
			return plan, fmt.Errorf("%s is required; enter a value", field.Label)
		}
		if !field.Dirty {
			continue
		}
		if err := validate(field); err != nil {
			return plan, field.Error(err)
		}
		args := []string{"--" + field.Name + "=" + field.Value}
		if field.Type == "stringArray" {
			args = nil
			values, err := readCSV(field.Value)
			if err != nil {
				return plan, field.Error(err)
			}
			for _, value := range values {
				args = append(args, "--"+field.Name+"="+value)
			}
		}
		if field.Global {
			plan.Args = append(plan.Args, args...)
		} else {
			local = append(local, args...)
		}
	}
	if err := f.Action.Command.ValidateArgs(positionals); err != nil {
		return plan, err
	}
	value := func(name string) string {
		if field := f.field(name); field != nil {
			return field.Value
		}
		return ""
	}
	path := strings.Join(f.Action.Path, " ")
	if path == "archive" || path == "sync" || path == "cron add" {
		if value("all") == "true" && value("chat") != "" {
			return plan, fmt.Errorf("choose all chats or specific chats; clear one of these options")
		}
		if value("include-left") == "true" && (value("all") != "true" || value("takeout") != "true") {
			return plan, fmt.Errorf("including left channels requires all chats and takeout")
		}
	}
	for _, name := range []string{"since", "until"} {
		if date := value(name); date != "" && !validDate(date) {
			return plan, fmt.Errorf("%s: use YYYY-MM-DD, RFC3339 or Unix seconds", name)
		}
	}
	plan.Args = append(plan.Args, f.Action.Path...)
	plan.Args = append(plan.Args, local...)
	if len(positionals) > 0 {
		if !f.Action.Command.DisableFlagParsing {
			plan.Args = append(plan.Args, "--")
		}
		plan.Args = append(plan.Args, positionals...)
	}
	return plan, nil
}

func validate(field Field) error {
	var err error
	switch field.Type {
	case "bool":
		_, err = strconv.ParseBool(field.Value)
	case "int", "int8", "int16", "int32", "int64", "count":
		bits := 64
		if field.Type == "int8" {
			bits = 8
		}
		if field.Type == "int16" {
			bits = 16
		}
		if field.Type == "int32" {
			bits = 32
		}
		_, err = strconv.ParseInt(field.Value, 0, bits)
	case "uint", "uint8", "uint16", "uint32", "uint64":
		bits := 64
		if field.Type == "uint8" {
			bits = 8
		}
		if field.Type == "uint16" {
			bits = 16
		}
		if field.Type == "uint32" {
			bits = 32
		}
		_, err = strconv.ParseUint(field.Value, 0, bits)
	case "float32", "float64":
		_, err = strconv.ParseFloat(field.Value, 64)
	case "duration":
		_, err = time.ParseDuration(field.Value)
	case "stringSlice", "stringArray", "stringToString", "intSlice", "int64Slice", "uintSlice", "boolSlice", "durationSlice", "float64Slice":
		var values []string
		values, err = readCSV(field.Value)
		if err == nil {
			for _, value := range values {
				if field.Type == "stringToString" && !strings.Contains(value, "=") {
					return fmt.Errorf("use comma-separated key=value pairs")
				}
				if subtype := strings.TrimSuffix(field.Type, "Slice"); subtype != field.Type && subtype != "string" {
					if e := validate(Field{Type: subtype, Value: value}); e != nil {
						return e
					}
				}
			}
		}
	}
	if err != nil {
		return err
	}
	if len(field.Choices) > 0 && !strings.Contains(field.Type, "Slice") && !slices.Contains(field.Choices, field.Value) {
		return fmt.Errorf("choose %s", strings.Join(field.Choices, ", "))
	}
	return nil
}

func readCSV(value string) ([]string, error) {
	if value == "" {
		return nil, nil
	}
	return csv.NewReader(strings.NewReader(value)).Read()
}

func validDate(value string) bool {
	if _, err := strconv.ParseInt(value, 10, 64); err == nil {
		return true
	}
	for _, layout := range []string{"2006-01-02", time.RFC3339} {
		if _, err := time.Parse(layout, value); err == nil {
			return true
		}
	}
	return false
}

// Preview is suitable for display and never exposes credentials in global flags.
// Execution uses Args directly; no command string is evaluated by a shell.
func (p Plan) Preview() string {
	args := append([]string{"ta"}, p.Args...)
	secretNext := false
	for i, arg := range args {
		if secretNext {
			args[i], secretNext = "<hidden>", false
			continue
		}
		name, _, hasValue := strings.Cut(arg, "=")
		if strings.HasPrefix(name, "--") && (name == "--proxy" || strings.Contains(name, "password") || strings.Contains(name, "passcode") || strings.Contains(name, "token")) {
			if hasValue {
				args[i] = name + "=<hidden>"
			} else {
				secretNext = true
			}
		}
	}
	return shellquote.Join(args...)
}
