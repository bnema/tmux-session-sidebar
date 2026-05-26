package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Router handles a parsed runtime route. Concrete orchestration is wired in
// later; phase 1 only nails down command parsing and dispatch.
type Router interface {
	Handle(ctx context.Context, route Route, stdout io.Writer, stderr io.Writer) error
}

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
	builtBy = "source"
)

type runtimeError struct {
	err error
}

func (e runtimeError) Error() string { return e.err.Error() }
func (e runtimeError) Unwrap() error { return e.err }

// Run parses CLI arguments, dispatches to the supplied router, and maps errors
// to stable process exit codes.
func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, router Router) int {
	cmd := newRootCommand(ctx, stdout, stderr, router)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		if _, ok := errors.AsType[runtimeError](err); ok {
			return 1
		}
		return 2
	}
	return 0
}

func newRootCommand(ctx context.Context, stdout io.Writer, stderr io.Writer, router Router) *cobra.Command {
	command := &cobra.Command{
		Use:           "tmux-session-sidebar",
		Short:         "tmux session sidebar runtime",
		Long:          "tmux-session-sidebar manages the tmux sidebar UI, runtime hooks, and session actions.",
		SilenceUsage:  true,
		SilenceErrors: false,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	command.SetOut(stdout)
	command.SetErr(stderr)
	addRuntimeFlags(command.PersistentFlags())

	runRoute := func(path string) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, args []string) error {
			return dispatchRoute(ctx, router, stdout, stderr, Route{Path: path, Flags: collectFlags(cmd), Args: routeArgs(cmd, args)})
		}
	}
	showHelp := func(cmd *cobra.Command, _ []string) error { return cmd.Help() }

	command.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			_, _ = fmt.Fprintf(stdout, "tmux-session-sidebar %s\ncommit: %s\ndate: %s\nbuiltBy: %s\n", version, commit, date, builtBy)
			return nil
		},
	})

	command.AddCommand(groupCommand("daemon", "Manage the background sidebar daemon",
		leafCommand("serve", "Run the sidebar daemon", runRoute("daemon/serve")),
		leafCommand("serve-ui", "Run the singleton sidebar UI", runRoute("daemon/serve-ui")),
		leafCommand("ensure", "Ensure restored sidebar state is captured", runRoute("daemon/ensure")),
	))
	command.AddCommand(groupCommand("hook", "Handle tmux runtime hooks",
		leafCommand("client-attached", "Handle tmux client-attached", runRoute("hook/client-attached")),
		leafCommand("client-detached", "Handle tmux client-detached", runRoute("hook/client-detached")),
		leafCommand("client-session-changed", "Handle tmux client-session-changed", runRoute("hook/client-session-changed")),
		leafCommand("client-resized", "Handle tmux client-resized", runRoute("hook/client-resized")),
		leafCommand("window-resized", "Handle tmux window-resized", runRoute("hook/window-resized")),
		leafCommand("agent-event", "Record an agent attention event", runRoute("hook/agent-event")),
	))
	command.AddCommand(&cobra.Command{
		Use:   "hooks [setup|uninstall|agent action]",
		Short: "Install, uninstall, or receive agent hooks",
		Long:  "Install agent integrations or receive hook events from supported coding agents.",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return dispatchRoute(ctx, router, stdout, stderr, Route{Path: "hooks/run", Flags: collectFlags(cmd), Args: routeArgs(cmd, args)})
		},
	})
	command.AddCommand(groupCommand("sidebar", "Open, close, or toggle the sidebar",
		leafCommand("toggle", "Toggle the sidebar for a tmux client", runRoute("sidebar/toggle")),
		leafCommand("open", "Open the sidebar for a tmux client", runRoute("sidebar/open")),
		leafCommand("close", "Close the sidebar for a tmux client", runRoute("sidebar/close")),
	))
	command.AddCommand(groupCommand("action", "Run sidebar session actions",
		leafCommand("switch", "Switch the tmux client to a session", runRoute("action/switch")),
		leafCommand("quick-switch", "Switch by visible sidebar slot", runRoute("action/quick-switch")),
		leafCommand("create-project", "Create a session from a selected project", runRoute("action/create-project")),
		leafCommand("create-current-git-project", "Create a session from the current git project", runRoute("action/create-current-git-project")),
		leafCommand("create-adhoc", "Create an ad-hoc session", runRoute("action/create-adhoc")),
		leafCommand("rename", "Rename a session", runRoute("action/rename")),
		leafCommand("kill", "Kill a session", runRoute("action/kill")),
		leafCommand("toggle-numeric", "Toggle numeric session labels", runRoute("action/toggle-numeric")),
	))

	for _, child := range command.Commands() {
		if child.RunE == nil {
			child.RunE = showHelp
		}
	}
	return command
}

func groupCommand(use string, short string, children ...*cobra.Command) *cobra.Command {
	cmd := &cobra.Command{Use: use, Short: short}
	cmd.AddCommand(children...)
	return cmd
}

func leafCommand(use string, short string, run func(*cobra.Command, []string) error) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, RunE: run}
}

func addRuntimeFlags(flags *pflag.FlagSet) {
	for _, name := range []string{"agent", "client", "confirmed", "event", "name", "pane", "project-path", "session", "slot", "window"} {
		flags.String(name, "", "runtime value passed by tmux-session-sidebar")
	}
	flags.BoolP("yes", "y", false, "assume yes when installing or uninstalling hooks")
}

func dispatchRoute(ctx context.Context, router Router, stdout io.Writer, stderr io.Writer, route Route) error {
	if router == nil {
		return runtimeError{err: errors.New("missing router")}
	}
	if err := router.Handle(ctx, route, stdout, stderr); err != nil {
		return runtimeError{err: err}
	}
	return nil
}

func collectFlags(cmd *cobra.Command) map[string]string {
	flags := make(map[string]string)
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if !flag.Changed {
			return
		}
		flags[flag.Name] = flag.Value.String()
	})
	return flags
}

func routeArgs(cmd *cobra.Command, args []string) []string {
	path := strings.Fields(cmd.CommandPath())
	if len(path) > 0 {
		path = path[1:]
	}
	return append(path, args...)
}
