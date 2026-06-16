package daemonctl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
	"github.com/bnema/tmux-session-sidebar/internal/runtimefs"
)

// Launcher asks tmux to start the background daemon asynchronously. The daemon
// itself owns singleton enforcement with its state-file lock, so this adapter
// stays cheap and avoids blocking latency-sensitive sidebar routes.
type Launcher struct {
	Process     ports.ProcessPort
	RuntimePath string
	StateDir    string
}

func (l Launcher) EnsureStarted(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if l.Process == nil {
		return fmt.Errorf("daemon launcher missing process port")
	}
	runtimePath := strings.TrimSpace(l.RuntimePath)
	if runtimePath == "" {
		resolved, err := os.Executable()
		if err != nil {
			return err
		}
		runtimePath = resolved
	}
	stateDir := strings.TrimSpace(l.StateDir)
	if stateDir == "" {
		stateDir = filepath.Join(os.TempDir(), ".tmux-session-sidebar-daemon")
	}
	if err := runtimefs.EnsureRuntimeDirPrivate(stateDir); err != nil {
		return err
	}
	logPath := filepath.Join(stateDir, "errors.log")
	command := shellQuote(runtimePath) + " daemon serve >/dev/null 2>>" + shellQuote(logPath)
	_, err := l.Process.Exec(ctx, "tmux", []string{"run-shell", "-b", command})
	return err
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
