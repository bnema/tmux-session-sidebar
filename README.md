# tmux Session Sidebar

A TPM plugin that opens a left-hand tmux sidebar for switching and managing sessions.

The sidebar is owned by a single Go runtime process. The process keeps one tmux pane for the UI, parks it in a hidden `__tmux-session-sidebar` session (keeps it alive but detached) when hidden, and joins it into the active tmux window when opened.

## Requirements

- tmux with support for user options, hooks, and format quoting
- bash, plus standard Unix tools used by the bootstrap script
- either Go 1.26 or newer to build from source, or `curl`/`tar` to install the latest released runtime binary
- git for TPM/manual installation and the current-repository session action
- optional: fzf for the non-sidebar `action create-project` picker
- optional: a Nerd Font for the bell marker glyph

## Install

### TPM

Add the plugin to `~/.tmux.conf`:

```tmux
set -g @plugin 'bnema/tmux-session-sidebar'
run '~/.tmux/plugins/tpm/tpm'
```

Reload tmux or press `prefix + I`.

### Manual

```bash
git clone https://github.com/bnema/tmux-session-sidebar ~/.tmux/plugins/tmux-session-sidebar
```

Add this to `~/.tmux.conf`:

```tmux
run-shell ~/.tmux/plugins/tmux-session-sidebar/tmux-session-sidebar.tmux
```

Then reload tmux:

```bash
tmux source-file ~/.tmux.conf
```

On load, `scripts/ensure-runtime.sh` prepares `.bin/tmux-session-sidebar` inside the plugin checkout. Normal installs use the latest GitHub release binary and fall back to a local Go build only if the release download is unavailable. Set `TMUX_SESSION_SIDEBAR_BUILD_FROM_SOURCE=1` to force a source build. The plugin also installs a managed git `post-merge` hook so future TPM `prefix + U` updates refresh the runtime after `git pull`.

## Configuration

Set options before the plugin is loaded.

```tmux
set -g @session-sidebar-key 'M-b'
set -g @session-sidebar-width '20'
set -g @session-sidebar-project-roots "$HOME/projects:$HOME/dev/projects"
set -g @session-sidebar-close-after-switch 'off'
set -g @session-sidebar-heat-colors 'on'
set -g @session-sidebar-heat-recent-hours '1'
set -g @session-sidebar-agent-attention 'on'
set -g @session-sidebar-auto-sort-recent 'off'
```

| Option | Default | Used for |
| --- | --- | --- |
| `@session-sidebar-key` | `M-b` | Global key that toggles the sidebar |
| `@session-sidebar-width` | `20` | Sidebar pane width |
| `@session-sidebar-project-roots` | `$HOME/projects` | Colon-separated directories scanned by the project picker |
| `@session-sidebar-close-after-switch` | `off` | Close the sidebar after selecting a session when set to `on` |
| `@session-sidebar-heat-colors` | `on` | Enable activity-based session colors |
| `@session-sidebar-heat-recent-hours` | `1` | Hours a visited or active session stays highlighted |
| `@session-sidebar-agent-attention` | `on` | Enable bell markers from supported agent hooks |
| `@session-sidebar-auto-sort-recent` | `off` | Once per day, reorder sessions by most recent real pane activity |

Persistent state and the daemon IPC socket are stored under `${XDG_STATE_HOME:-~/.local/state}/tmux-session-sidebar`.

## Daemon lifecycle

Reloading tmux starts or restarts the Go runtime daemon through the plugin bootstrap:

```bash
tmux source-file ~/.tmux.conf
```

If the daemon exits later, it is not restarted automatically; reload tmux to start it again. To stop it manually, kill the PID recorded in the state directory:

```bash
kill "$(cat "${XDG_STATE_HOME:-$HOME/.local/state}/tmux-session-sidebar/daemon.pid")"
```

To inspect the hidden sidebar session:

```bash
tmux list-windows -t __tmux-session-sidebar
tmux list-panes -t __tmux-session-sidebar -F '#{pane_id} #{window_id} #{pane_current_command}'
```

## Usage

Press `Alt+b` to open or close the sidebar. It opens as a full-height left split in the current tmux window. By default the sidebar stays open and follows you when you switch sessions; set `@session-sidebar-close-after-switch` to `on` if you want it to park instead.

Pane sizes are remembered separately with and without the sidebar. If you arrange splits while the sidebar is visible, hiding it restores your full-width layout, and reopening it restores the last compatible sidebar-visible layout. If panes were added, removed, or otherwise made incompatible while the sidebar was hidden, tmux-session-sidebar falls back to the configured sidebar width.

Inside the sidebar:

| Key | Action |
| --- | --- |
| `j` / `k`, arrows | Move selection |
| `/` | Filter sessions |
| `Enter` | Switch to the selected session, apply a filter, or choose a project |
| `Esc` | Leave the current mode, or close the sidebar |
| `F5` | Reload the session list |
| `M-n` | Open the project picker |
| `M-g` | Create or switch to a session for the current pane's git repository |
| `M-a` | Create or switch to a session for the current pane's directory |
| `M-r` | Rename the selected session |
| `M-x` | Kill the selected session after confirmation |
| `M-j` / `M-k` or `M-Down` / `M-Up` | Move the selected session in the sidebar order |
| `M-h` | Show or hide numeric session names |
| `M-?` | Show or hide key help |
| `Ctrl+c` | Quit the sidebar UI |

Global quick-switch keys are also installed:

- `Ctrl+1` through `Ctrl+9` switch to visible sidebar slots 1 through 9
- `Ctrl+0` switches to visible slot 10

Session names that are all digits, or that start with `__`, are hidden by default. `M-h` toggles numeric names. Hidden `__` sessions are not shown.

## Session actions

### Project sessions

`M-n` lists one directory level under each configured project root. Selecting a project creates or switches to a tmux session named from the directory basename.

`M-g` uses `git rev-parse --show-toplevel` from the current pane path and creates or switches to that repository session.

Generated names are lowercased and normalized to letters, digits, `_`, and `-`. Invalid or empty names fall back to `session`.

### Ad-hoc sessions

`M-a` creates or switches to a session for the current pane path, using the normalized directory basename as the session name.

### Rename, kill, and reorder

`M-r` uses tmux `command-prompt` for the new name. `M-x` asks for inline confirmation before killing, and the runtime refuses to kill the last remaining session. Reordering is saved in the plugin state file.

## Session restore

The plugin records persistable session names and paths in its state file. On daemon startup or client attach, it recreates missing remembered sessions.

Restore skips sessions with numeric names, names beginning with `__`, invalid names, and sessions that already exist. A restored session starts in its last recorded path, then its project path, then the home directory, then `.` if no usable absolute directory is available.

Killing a session through the sidebar removes it from future restore. Renaming through the sidebar updates persisted state.

## Heat colors and agent bells

The sidebar can color recently active sessions. The current session is bright with `*`, sessions active or visited within `@session-sidebar-heat-recent-hours` are light green, and inactive sessions are gray.

```tmux
set -g @session-sidebar-heat-colors 'off'
```

To keep recently used projects near the top, enable daily sorting:

```tmux
set -g @session-sidebar-auto-sort-recent 'on'
```

This uses real pane activity tracked by the heat state, not a simple session switch. Accidentally switching to a session and switching back does not move it up unless pane output changes there.

Agent bells are separate. If enabled, supported agent hooks can mark a session with a bell when an agent stops or needs attention. The bell clears when that session becomes current in any attached tmux client.

Install hooks for supported agents found on `PATH`:

```bash
~/.tmux/plugins/tmux-session-sidebar/.bin/tmux-session-sidebar hooks setup --yes
```

Or install one integration:

```bash
~/.tmux/plugins/tmux-session-sidebar/.bin/tmux-session-sidebar hooks codex install
~/.tmux/plugins/tmux-session-sidebar/.bin/tmux-session-sidebar hooks pi install
```

See [docs/agent-hooks.md](docs/agent-hooks.md) for the supported-agent list and uninstall commands.

## Troubleshooting

Check the toggle binding:

```bash
tmux list-keys -T root M-b
```

Check the runtime binary:

```bash
~/.tmux/plugins/tmux-session-sidebar/.bin/tmux-session-sidebar --help
~/.tmux/plugins/tmux-session-sidebar/.bin/tmux-session-sidebar version
```

Force a runtime refresh from the latest GitHub release after a TPM update:

```bash
TMUX_SESSION_SIDEBAR_REFRESH_RELEASE=1 ~/.tmux/plugins/tmux-session-sidebar/scripts/ensure-runtime.sh
```

If TPM updates still leave an old binary, reload tmux once so the plugin can install its managed git update hook:

```bash
tmux source-file ~/.tmux.conf
```

Enable activity debug logging:

```tmux
set -g @session-sidebar-activity-debug-log 'on'
```

Then inspect:

```bash
tail -f ~/.local/state/tmux-session-sidebar/activity.log
```

## Development

For local development:

```bash
make install
make build-runtime
make restart-runtime
make test-go
make test-runtime-bootstrap
```

`make install` symlinks the current checkout into `~/.tmux/plugins/tmux-session-sidebar`. `make build-runtime` updates the plugin-local `.bin/tmux-session-sidebar` runtime from source for fast local testing. Normal plugin installs prefer the latest GitHub release binary and only fall back to a local Go build when the release download is unavailable; set `TMUX_SESSION_SIDEBAR_BUILD_FROM_SOURCE=1` to force a source build. `make restart-runtime` rebuilds the runtime, stops old daemon/UI processes, removes the hidden singleton session, and reloads tmux config so the next sidebar launch uses the new binary. `make uninstall` removes the symlink.
