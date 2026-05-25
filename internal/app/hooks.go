package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
)

func runHooksCommand(ctx context.Context, args []string, flags map[string]string, stdout io.Writer, _ io.Writer) error {
	if len(args) < 2 {
		return fmt.Errorf("missing hooks command")
	}
	first := strings.ToLower(strings.TrimSpace(args[1]))
	switch first {
	case "setup":
		return runHooksSetup(stdout, flags, positionalHookTarget(args[2:]), false)
	case "uninstall":
		return runHooksSetup(stdout, flags, positionalHookTarget(args[2:]), true)
	default:
		def, ok := agentHookDefNamed(first)
		if !ok {
			return fmt.Errorf("unknown hooks target %q", first)
		}
		if len(args) < 3 {
			return fmt.Errorf("missing hooks action for %s", def.Name)
		}
		action := strings.ToLower(strings.TrimSpace(args[2]))
		switch action {
		case "install":
			return installHooksForAgent(stdout, def, yesFlag(flags))
		case "uninstall":
			return uninstallHooksForAgent(stdout, def, yesFlag(flags))
		default:
			eventFlags := cloneFlags(flags)
			eventFlags["agent"] = def.Name
			eventFlags["event"] = action
			if strings.TrimSpace(eventFlags["pane"]) == "" {
				eventFlags["pane"] = strings.TrimSpace(os.Getenv("TMUX_PANE"))
			}
			return recordAgentHookEvent(ctx, eventFlags)
		}
	}
}

func runHooksSetup(stdout io.Writer, flags map[string]string, positionalTarget string, uninstall bool) error {
	flagTarget := strings.TrimSpace(flags["agent"])
	if flagTarget != "" && positionalTarget != "" {
		flagDef, ok := agentHookDefNamed(flagTarget)
		if !ok {
			return fmt.Errorf("unknown hooks target %q", flagTarget)
		}
		positionalDef, ok := agentHookDefNamed(positionalTarget)
		if !ok {
			return fmt.Errorf("unknown hooks target %q", positionalTarget)
		}
		if flagDef.Name != positionalDef.Name {
			return fmt.Errorf("conflicting hooks target: use either --agent or a positional target")
		}
	}
	target := flagTarget
	if target == "" {
		target = positionalTarget
	}
	var filter *agentHookDef
	if target != "" {
		def, ok := agentHookDefNamed(target)
		if !ok {
			return fmt.Errorf("unknown hooks target %q", target)
		}
		filter = &def
	}

	if err := writef(stdout, "tmux-session-sidebar hooks %s: %s agent hooks\n\n", map[bool]string{true: "uninstall", false: "setup"}[uninstall], map[bool]string{true: "uninstalling", false: "installing"}[uninstall]); err != nil {
		return err
	}

	installed := 0
	skipped := 0
	skippedNoBinary := []string{}
	for _, def := range supportedAgentHookDefs() {
		if filter != nil && filter.Name != def.Name {
			continue
		}
		if !uninstall && !binaryOnPath(def.BinaryName) {
			if err := writef(stdout, "  %s: skipped (binary not found on PATH)\n", def.Name); err != nil {
				return err
			}
			skipped++
			skippedNoBinary = append(skippedNoBinary, def.Name)
			continue
		}
		if err := writef(stdout, "  %s:\n", def.Name); err != nil {
			return err
		}
		var err error
		if uninstall {
			err = uninstallHooksForAgent(stdout, def, true)
		} else {
			err = installHooksForAgent(stdout, def, yesFlag(flags))
		}
		if err != nil {
			return err
		}
		installed++
		if err := writeln(stdout); err != nil {
			return err
		}
	}
	if err := writef(stdout, "Done: %d %s, %d skipped\n", installed, map[bool]string{true: "uninstalled", false: "installed"}[uninstall], skipped); err != nil {
		return err
	}
	if len(skippedNoBinary) > 0 {
		if err := writef(stdout, "  skipped %d agents (not found on PATH): %s\n", len(skippedNoBinary), strings.Join(skippedNoBinary, ", ")); err != nil {
			return err
		}
	}
	return nil
}

func installHooksForAgent(stdout io.Writer, def agentHookDef, assumeYes bool) error {
	switch def.Format {
	case agentHookFormatFlatJSON, agentHookFormatNestedJSON:
		return installJSONHooks(stdout, def, assumeYes)
	case agentHookFormatOpenCodePlugin:
		return installManagedFile(stdout, def, openCodePluginSource(def), assumeYes)
	case agentHookFormatPiExtension:
		return installManagedFile(stdout, def, piExtensionSource(def), assumeYes)
	case agentHookFormatAmpPlugin:
		return installManagedFile(stdout, def, ampPluginSource(def), assumeYes)
	case agentHookFormatRovoYAML:
		return installRovoHooks(stdout, def, assumeYes)
	default:
		return fmt.Errorf("unsupported hook format %q", def.Format)
	}
}

func uninstallHooksForAgent(stdout io.Writer, def agentHookDef, _ bool) error {
	switch def.Format {
	case agentHookFormatFlatJSON, agentHookFormatNestedJSON:
		return uninstallJSONHooks(stdout, def)
	case agentHookFormatOpenCodePlugin, agentHookFormatPiExtension, agentHookFormatAmpPlugin:
		return uninstallManagedFile(stdout, def)
	case agentHookFormatRovoYAML:
		return uninstallRovoHooks(stdout, def)
	default:
		return fmt.Errorf("unsupported hook format %q", def.Format)
	}
}

func installManagedFile(stdout io.Writer, def agentHookDef, content string, assumeYes bool) error {
	path := def.configPath()
	existingBytes, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	existing := string(existingBytes)
	if existing == content {
		return writef(stdout, "    %s hooks already up to date at %s\n", def.DisplayName, path)
	}
	if existing != "" && !strings.Contains(existing, def.ownedMarker()) {
		return fmt.Errorf("%s exists and is not a tmux-session-sidebar-managed integration; leaving it alone", path)
	}
	if !assumeYes {
		ok, err := confirmWrite(path)
		if err != nil {
			return err
		}
		if !ok {
			return writeln(stdout, "    Aborted.")
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}
	return writef(stdout, "    %s hooks installed at %s\n", def.DisplayName, path)
}

func uninstallManagedFile(stdout io.Writer, def agentHookDef) error {
	path := def.configPath()
	existingBytes, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return writef(stdout, "    No %s integration found at %s\n", def.DisplayName, path)
	}
	if err != nil {
		return err
	}
	if !strings.Contains(string(existingBytes), def.ownedMarker()) {
		return writef(stdout, "    Refusing to remove %s: missing tmux-session-sidebar marker\n", path)
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	return writef(stdout, "    Removed %s integration from %s\n", def.DisplayName, path)
}

func confirmWrite(path string) (bool, error) {
	fmt.Printf("Will modify %s\nProceed? [y/N] ", path)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	line = strings.ToLower(strings.TrimSpace(line))
	return strings.HasPrefix(line, "y"), nil
}

func positionalHookTarget(args []string) string {
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		if trimmed == "" || strings.HasPrefix(trimmed, "-") {
			continue
		}
		return trimmed
	}
	return ""
}

func yesFlag(flags map[string]string) bool {
	return strings.EqualFold(strings.TrimSpace(flags["yes"]), "true") || strings.EqualFold(strings.TrimSpace(flags["y"]), "true")
}

func cloneFlags(flags map[string]string) map[string]string {
	cloned := make(map[string]string, len(flags))
	maps.Copy(cloned, flags)
	return cloned
}

func writef(w io.Writer, format string, args ...any) error {
	_, err := fmt.Fprintf(w, format, args...)
	return err
}

func writeln(w io.Writer, args ...any) error {
	_, err := fmt.Fprintln(w, args...)
	return err
}
