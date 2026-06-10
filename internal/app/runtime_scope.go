package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bnema/tmux-session-sidebar/ports"
)

// RuntimeScope describes the filesystem namespace used by one sidebar runtime.
// Outside tmux it preserves the historical global layout. Inside tmux it scopes
// volatile and persisted runtime files by tmux server identity.
type RuntimeScope struct {
	RootDir       string
	Dir           string
	IPCSocketPath string
	PIDPath       string
	ErrorsLogPath string
	LocksDir      string
	Legacy        bool
	SocketPath    string
	ServerPID     string
	IdentityKey   string
}

var runtimeScopeOverride struct {
	mu    sync.RWMutex
	scope RuntimeScope
	set   bool
}

func SetRuntimeScope(scope RuntimeScope) {
	runtimeScopeOverride.mu.Lock()
	defer runtimeScopeOverride.mu.Unlock()
	runtimeScopeOverride.scope = scope
	runtimeScopeOverride.set = true
}

func ResetRuntimeScopeForTest() {
	runtimeScopeOverride.mu.Lock()
	defer runtimeScopeOverride.mu.Unlock()
	runtimeScopeOverride.scope = RuntimeScope{}
	runtimeScopeOverride.set = false
}

func CurrentRuntimeScope() RuntimeScope {
	runtimeScopeOverride.mu.RLock()
	if runtimeScopeOverride.set {
		scope := runtimeScopeOverride.scope
		runtimeScopeOverride.mu.RUnlock()
		return scope
	}
	runtimeScopeOverride.mu.RUnlock()
	return RuntimeScopeForProcess(context.Background(), nil)
}

func RuntimeScopeForProcess(ctx context.Context, process ports.ProcessPort) RuntimeScope {
	root := LegacyStateRoot()
	tmuxEnv := strings.TrimSpace(os.Getenv("TMUX"))
	if tmuxEnv == "" {
		return runtimeScopeFromDir(root, true, "", "")
	}
	if process != nil {
		socketPath, pid, ok := queryTmuxServerIdentity(ctx, process)
		if ok {
			return runtimeScopeFromDir(root, false, canonicalPath(socketPath), pid)
		}
	}
	return runtimeScopeFromTmuxEnv(root, tmuxEnv)
}

func runtimeScopeFromTmuxEnv(root string, tmuxEnv string) RuntimeScope {
	fields := strings.Split(tmuxEnv, ",")
	if len(fields) >= 2 && strings.TrimSpace(fields[0]) != "" && strings.TrimSpace(fields[1]) != "" {
		return runtimeScopeFromDir(root, false, canonicalPath(strings.TrimSpace(fields[0])), strings.TrimSpace(fields[1]))
	}
	identity := "tmux-env\x00" + tmuxEnv
	return runtimeScopeFromIdentity(root, "", "", identity)
}

func runtimeScopeFromDir(root string, legacy bool, socketPath string, pid string) RuntimeScope {
	identityKey := ""
	if !legacy {
		identityKey = socketPath + "\x00" + pid
	}
	return runtimeScopeFromIdentity(root, socketPath, pid, identityKey)
}

func runtimeScopeFromIdentity(root string, socketPath string, pid string, identityKey string) RuntimeScope {
	dir := root
	legacy := identityKey == ""
	if !legacy {
		dir = filepath.Join(root, "servers", boundedScopeHash(identityKey))
	}
	return RuntimeScope{
		RootDir:       root,
		Dir:           dir,
		IPCSocketPath: filepath.Join(dir, "sidebar.sock"),
		PIDPath:       filepath.Join(dir, "daemon.pid"),
		ErrorsLogPath: filepath.Join(dir, "errors.log"),
		LocksDir:      filepath.Join(dir, "locks"),
		Legacy:        legacy,
		SocketPath:    socketPath,
		ServerPID:     pid,
		IdentityKey:   identityKey,
	}
}

func queryTmuxServerIdentity(ctx context.Context, process ports.ProcessPort) (string, string, bool) {
	result, err := process.Exec(ctx, "tmux", []string{"display-message", "-p", "#{socket_path}\t#{pid}"})
	if err != nil {
		return "", "", false
	}
	fields := strings.Split(strings.TrimSpace(result.Stdout), "\t")
	if len(fields) != 2 || strings.TrimSpace(fields[0]) == "" || strings.TrimSpace(fields[1]) == "" {
		return "", "", false
	}
	return strings.TrimSpace(fields[0]), strings.TrimSpace(fields[1]), true
}

func canonicalPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil && resolved != "" {
		return resolved
	}
	if abs, err := filepath.Abs(path); err == nil && abs != "" {
		return filepath.Clean(abs)
	}
	return filepath.Clean(path)
}

func writeRuntimeScopeMetadata(scope RuntimeScope) error {
	if err := EnsureRuntimeDirPrivate(scope.Dir); err != nil {
		return err
	}
	data, err := json.MarshalIndent(runtimeScopeMetadata{
		Legacy:      scope.Legacy,
		SocketPath:  scope.SocketPath,
		ServerPID:   scope.ServerPID,
		IdentityKey: scope.IdentityKey,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(runtimeScopeMetadataPath(scope), append(data, '\n'), 0o600)
}

type runtimeScopeMetadata struct {
	Legacy      bool   `json:"legacy"`
	SocketPath  string `json:"socketPath,omitempty"`
	ServerPID   string `json:"serverPid,omitempty"`
	IdentityKey string `json:"identityKey,omitempty"`
}

func runtimeScopeMetadataPath(scope RuntimeScope) string {
	return filepath.Join(scope.Dir, "server.json")
}

func runtimeScopeStillCurrent(ctx context.Context, scope RuntimeScope) (bool, error) {
	if scope.Legacy || scope.IdentityKey == "" {
		return true, nil
	}
	out, err := tmux(ctx, "display-message", "-p", "#{socket_path}\t#{pid}")
	if err != nil {
		return false, fmt.Errorf("verify tmux server identity: %w", err)
	}
	fields := strings.Split(strings.TrimSpace(out), "\t")
	if len(fields) != 2 || strings.TrimSpace(fields[0]) == "" || strings.TrimSpace(fields[1]) == "" {
		return false, fmt.Errorf("verify tmux server identity: malformed output %q", strings.TrimSpace(out))
	}
	currentSocket := canonicalPath(strings.TrimSpace(fields[0]))
	currentPID := strings.TrimSpace(fields[1])
	return currentSocket == scope.SocketPath && currentPID == scope.ServerPID, nil
}

func boundedScopeHash(identity string) string {
	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:])[:32]
}
