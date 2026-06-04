package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

var selfUpdateExecutablePath = os.Executable

func runSelfUpdate(ctx context.Context, stdout io.Writer, stderr io.Writer) error {
	executable, err := selfUpdateExecutablePath()
	if err != nil {
		return fmt.Errorf("locating current executable: %w", err)
	}
	binDir := filepath.Dir(executable)
	if filepath.Base(binDir) != ".bin" {
		return fmt.Errorf("self-update requires the plugin runtime layout: expected executable under <plugin>/.bin, got %s", executable)
	}
	pluginDir := filepath.Dir(binDir)
	updaterPath := filepath.Join(pluginDir, "scripts", "update-runtime.sh")
	info, err := os.Stat(updaterPath)
	if err != nil {
		return fmt.Errorf("self-update updater not found at %s: %w", updaterPath, err)
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return fmt.Errorf("self-update updater is not executable: %s", updaterPath)
	}
	cmd := exec.CommandContext(ctx, updaterPath)
	cmd.Dir = pluginDir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}
