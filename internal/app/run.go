package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Router handles a parsed runtime route. Concrete orchestration is wired in
// later; phase 1 only nails down command parsing and dispatch.
type Router interface {
	Handle(ctx context.Context, route Route, stdout io.Writer, stderr io.Writer) error
}

// Run parses CLI arguments, dispatches to the supplied router, and maps errors
// to stable process exit codes.
func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, router Router) int {
	route, err := parseRoute(args)
	if err != nil {
		writeError(stderr, err.Error())
		return 2
	}
	if router == nil {
		writeError(stderr, "missing router")
		return 1
	}
	if err := router.Handle(ctx, route, stdout, stderr); err != nil {
		writeError(stderr, err.Error())
		return 1
	}
	return 0
}

func parseRoute(args []string) (Route, error) {
	if len(args) == 0 {
		return Route{}, errors.New("missing command")
	}

	flags := make(map[string]string)
	rest := make([]string, 0)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") || arg == "--" {
			rest = append(rest, arg)
			continue
		}

		key, value, ok := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		if key == "" {
			return Route{}, fmt.Errorf("invalid flag %q", arg)
		}
		if !ok {
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				value = args[i+1]
				i++
			} else {
				value = "true"
			}
		}
		flags[key] = value
	}

	if len(rest) == 0 {
		return Route{}, errors.New("missing command")
	}

	path, err := routePath(rest)
	if err != nil {
		return Route{}, err
	}
	return Route{Path: path, Flags: flags, Args: rest}, nil
}

func writeError(stderr io.Writer, message string) {
	_, _ = fmt.Fprintln(stderr, message)
}

func routePath(args []string) (string, error) {
	switch args[0] {
	case "daemon":
		if len(args) < 2 {
			return "", errors.New("missing daemon command")
		}
		switch args[1] {
		case "serve", "ensure":
			return "daemon/" + args[1], nil
		}
	case "hook":
		if len(args) < 2 {
			return "", errors.New("missing hook command")
		}
		switch args[1] {
		case "client-attached", "client-detached", "client-session-changed":
			return "hook/" + args[1], nil
		}
	case "sidebar":
		if len(args) < 2 {
			return "", errors.New("missing sidebar command")
		}
		switch args[1] {
		case "toggle", "open", "close":
			return "sidebar/" + args[1], nil
		}
	case "ui":
		if len(args) < 2 {
			return "", errors.New("missing ui command")
		}
		if args[1] == "run" {
			return "ui/run", nil
		}
	case "action":
		if len(args) < 2 {
			return "", errors.New("missing action command")
		}
		switch args[1] {
		case "switch", "quick-switch", "create-project", "create-current-git-project", "create-adhoc", "rename", "kill", "toggle-numeric":
			return "action/" + args[1], nil
		}
	}
	return "", fmt.Errorf("unknown command %q", strings.Join(args, " "))
}
