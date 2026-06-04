package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

var selfUpdateExecutablePath = os.Executable

func runSelfUpdate(ctx context.Context, stdout io.Writer, stderr io.Writer) error {
	pluginDir, updaterPath, err := selfUpdatePaths()
	if err != nil {
		return err
	}
	return runUpdater(ctx, pluginDir, updaterPath, stdout, stderr)
}

func selfUpdatePaths() (string, string, error) {
	executable, err := selfUpdateExecutablePath()
	if err != nil {
		return "", "", fmt.Errorf("locating current executable: %w", err)
	}
	binDir := filepath.Dir(executable)
	if filepath.Base(binDir) != ".bin" {
		return "", "", fmt.Errorf("self-update requires the plugin runtime layout: expected executable under <plugin>/.bin, got %s", executable)
	}
	pluginDir := filepath.Dir(binDir)
	updaterPath := filepath.Join(pluginDir, "scripts", "update-runtime.sh")
	info, err := os.Stat(updaterPath)
	if err != nil {
		return "", "", fmt.Errorf("self-update updater not found at %s: %w", updaterPath, err)
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return "", "", fmt.Errorf("self-update updater is not executable: %s", updaterPath)
	}
	return pluginDir, updaterPath, nil
}

func runUpdater(ctx context.Context, pluginDir string, updaterPath string, stdout io.Writer, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, updaterPath)
	configureSelfUpdateCommand(cmd, pluginDir)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func configureSelfUpdateCommand(cmd *exec.Cmd, pluginDir string) {
	cmd.Dir = pluginDir
	cmd.Env = selfUpdateEnvironment(os.Environ())
}

func selfUpdateEnvironment(environ []string) []string {
	filtered := make([]string, 0, len(environ)+1)
	for _, entry := range environ {
		if strings.HasPrefix(entry, "TMUX_SESSION_SIDEBAR_BUILD_FROM_SOURCE=") || strings.HasPrefix(entry, "TMUX_SESSION_SIDEBAR_RELEASE_ONLY=") {
			continue
		}
		filtered = append(filtered, entry)
	}
	return append(filtered, "TMUX_SESSION_SIDEBAR_RELEASE_ONLY=1")
}

func configureBackgroundSelfUpdateCommand(cmd *exec.Cmd, pluginDir string) {
	configureSelfUpdateCommand(cmd, pluginDir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func runSelfUpdateBackground() error {
	pluginDir, updaterPath, err := selfUpdatePaths()
	if err != nil {
		return err
	}
	cmd := exec.Command(updaterPath)
	configureBackgroundSelfUpdateCommand(cmd, pluginDir)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}
