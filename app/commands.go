package app

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// CommandRouter executes command-mode instructions.
type CommandRouter struct {
	ClearHistory func() error
	SetTemp      func(float64) error
	SetModel     func(string) error
	ToggleStats  func() error
	Export       func(string) (string, error)
}

// Execute parses and runs a command, returning a status message.
func (r *CommandRouter) Execute(input string) (string, error) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(input, ":"))
	if trimmed == "" {
		return "", nil
	}

	fields := strings.Fields(trimmed)
	name := strings.ToLower(fields[0])
	args := fields[1:]

	switch name {
	case "clear":
		if r.ClearHistory == nil {
			return "", errors.New("clear not available")
		}
		if err := r.ClearHistory(); err != nil {
			return "", err
		}
		return "history cleared", nil
	case "temp":
		if r.SetTemp == nil {
			return "", errors.New("temp not available")
		}
		if len(args) != 1 {
			return "", errors.New("usage: :temp <value>")
		}
		v, err := strconv.ParseFloat(args[0], 64)
		if err != nil {
			return "", fmt.Errorf("invalid temperature: %w", err)
		}
		if err := r.SetTemp(v); err != nil {
			return "", err
		}
		return fmt.Sprintf("temperature %.2f", v), nil
	case "model":
		if r.SetModel == nil {
			return "", errors.New("model not available")
		}
		if len(args) == 0 {
			return "", errors.New("usage: :model <alias>")
		}
		alias := strings.Join(args, " ")
		if err := r.SetModel(alias); err != nil {
			return "", err
		}
		return fmt.Sprintf("model %s", alias), nil
	case "stats":
		if r.ToggleStats == nil {
			return "", errors.New("stats not available")
		}
		if err := r.ToggleStats(); err != nil {
			return "", err
		}
		return "stats toggled", nil
	case "export":
		if r.Export == nil {
			return "", errors.New("export not available")
		}
		var target string
		if len(args) > 0 {
			target = strings.Join(args, " ")
		} else {
			target = filepath.Join(".", fmt.Sprintf("chat-%d.txt", time.Now().Unix()))
		}
		path, err := r.Export(target)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("exported to %s", path), nil
	default:
		return "", fmt.Errorf("unknown command: %s", name)
	}
}
