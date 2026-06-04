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

On load, `scripts/ensure-runtime.sh` prepares `.bin/tmux-session-sidebar` inside the plugin checkout. Normal installs use the latest GitHub release binary and fall back to a local Go build only if the release download is unavailable. Set `TMUX_SESSION_SIDEBAR_BUILD_FROM_SOURCE=1` to force a source build. The plugin also installs a managed git `post-merge` hook so future TPM `prefix + U` updates run `scripts/update-runtime.sh`: the hook refreshes the release runtime, replaces the binary atomically, stops the old daemon/UI, and restarts tmux on the new binary.

## Configuration

Set options before the plugin is loaded.

```tmux
set -g @session-sidebar-key 'M-b'
set -g @session-sidebar-width '30'
set -g @session-sidebar-project-roots "$HOME/projects:$HOME/dev/projects"
set -g @session-sidebar-close-after-switch 'off'
set -g @session-sidebar-heat-colors 'on'
set -g @session-sidebar-heat-recent '1h'
set -g @session-sidebar-heat-max-highlighted '0'
set -g @session-sidebar-agent-attention 'on'
set -g @session-sidebar-agent-attention-animation 'pulse'
set -g @session-sidebar-auto-sort-recent 'off'
set -g @session-sidebar-restore-sessions 'auto'
set -g @session-sidebar-continuum-grace-seconds '3'
set -g @session-sidebar-metadata-subline 'on'
```

| Option | Default | Used for |
| --- | --- | --- |
| `@session-sidebar-key` | `M-b` | Global key that toggles the sidebar |
| `@session-sidebar-width` | `30` | Sidebar pane width |
| `@session-sidebar-project-roots` | `$HOME/projects` | Colon-separated directories scanned by the project picker |
| `@session-sidebar-close-after-switch` | `off` | Close the sidebar after selecting a session when set to `on` |
| `@session-sidebar-heat-colors` | `on` | Enable activity-based session colors |
| `@session-sidebar-heat-recent` | `1h` | Relative window used for recent-activity heat colors, for example `10m`, `2h`, or `3d`. Empty, `1`, `on`, `yes`, and `true` are treated as `1h`. |
| `@session-sidebar-heat-max-highlighted` | `0` | Maximum highlighted sessions at once; `0` means no limit |
| `@session-sidebar-agent-attention` | `on` | Enable bell markers from supported agent hooks |
| `@session-sidebar-agent-attention-animation` | `pulse` | Bell marker animation style: `off`, `pulse`, `rainbow`, or `blink` |
| `@session-sidebar-auto-sort-recent` | `off` | Relative interval for reordering sessions by most recent real pane activity, for example `10m`, `2h`, or `3d` |
| `@session-sidebar-restore-sessions` | `auto` | Lightweight missing-session restore mode: `auto` skips during tmux-continuum startup restore, `on` always restores, `off` never restores |
| `@session-sidebar-continuum-grace-seconds` | `3` | Extra seconds added to `@continuum-restore-max-delay` before lightweight restore resumes in `auto` mode |
| `@session-sidebar-metadata-subline` | `on` | Show an event-driven metadata line under each session when available; set to `off` to keep one-line session rows |

Persistent state and the daemon IPC socket are stored under `${XDG_STATE_HOME:-~/.local/state}/tmux-session-sidebar`.

## Daemon lifecycle

Reloading tmux starts or restarts the Go runtime daemon through the plugin bootstrap:

```bash
tmux source-file ~/.tmux.conf
```

If the daemon exits later, sidebar open/close/toggle/refresh actions attempt a lightweight background restart before falling back to direct handling. Reload tmux to force a clean restart. To stop it manually, kill the PID recorded in the state directory:

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
| `j` / `k`, arrows, mouse wheel | Move selection |
| `/` | Filter sessions |
| `Enter` | Switch to the selected session, apply a filter, or choose a project |
| `Esc` | Leave the current mode, or close the sidebar |
| `F5` | Reload the session list |
| `n` | Open the project picker when not searching/filtering |
| `g` | Create or switch to a session for the current pane's git repository when not searching/filtering |
| `a` | Create or switch to a session for the current pane's directory when not searching/filtering |
| `r` | Rename the selected session when not searching/filtering |
| `x` | Kill the selected session after confirmation when not searching/filtering |
| `J` / `K` | Move the selected session in the sidebar order when not searching/filtering |
| `h` | Show or hide numeric session names when not searching/filtering |
| `?` | Show or hide key help when not searching/filtering |
| `Ctrl+c` | Quit the sidebar UI |

Global quick-switch keys are also installed:

- `Ctrl+1` through `Ctrl+9` switch to visible sidebar slots 1 through 9
- `Ctrl+0` switches to visible slot 10

Session names that are all digits, or that start with `__`, are hidden by default. `h` toggles numeric names. Hidden `__` sessions are not shown.

## Session actions

### Project sessions

`n` lists one directory level under each configured project root. Selecting a project creates or switches to a tmux session named from the directory basename.

`g` uses `git rev-parse --show-toplevel` from the current pane path and creates or switches to that repository session.

Generated names are lowercased and normalized to letters, digits, `_`, and `-`. Invalid or empty names fall back to `session`.

### Ad-hoc sessions

`a` creates or switches to a session for the current pane path, using the normalized directory basename as the session name.

### Rename, kill, and reorder

`r` uses tmux `command-prompt` for the new name. `x` asks for inline confirmation before killing, and the runtime refuses to kill the last remaining session. Reordering is saved in the plugin state file.

## Session restore

The plugin records persistable session names and paths in its state file. On daemon startup or client attach, it recreates missing remembered sessions.

Restore skips sessions with numeric names, names beginning with `__`, invalid names, and sessions that already exist. A restored session starts in its last recorded path, then its project path, then the home directory, then `.` if no usable absolute directory is available.

Killing a session through the sidebar removes it from future restore. Renaming through the sidebar updates persisted state.

### tmux-resurrect and tmux-continuum compatibility

The plugin cooperates with `tmux-resurrect` by installing `@resurrect-hook-post-save-layout` when that hook is empty. The hook runs `tmux-session-sidebar resurrect post-save-layout <file>` and removes the internal `__tmux-session-sidebar` session, any pane marked `@session-sidebar-pane=1`, and sidebar-visible layouts from Resurrect save files. If you already use `@resurrect-hook-post-save-layout`, the plugin does not overwrite it; chain the command manually from your hook if you want automatic cleanup.

With `tmux-continuum`, leave `@session-sidebar-restore-sessions` at `auto`. During Continuum's startup restore window, tmux-session-sidebar skips its lightweight missing-session restore so Resurrect can restore full windows, panes, layouts, and processes first. After that window, the daemon captures the live state as usual. Set `@session-sidebar-restore-sessions` to `on` to force the sidebar restore even during Continuum startup, or `off` to disable the lightweight restore entirely.

If the sidebar was open before reboot, that intent is persisted. The old tmux client name is treated as transient; on the next `client-attached`, the sidebar adopts the attached client and reopens without stealing focus.

Recommended TPM order when using Continuum:

```tmux
set -g @plugin 'tmux-plugins/tmux-resurrect'
set -g @plugin 'tmux-plugins/tmux-continuum'
set -g @plugin 'bnema/tmux-session-sidebar'
```

## Heat colors and agent bells

The sidebar can color recently active sessions. The current session is bright with `*`. Other sessions fade from very light green toward the default inactive gray based on their activity age relative to `@session-sidebar-heat-recent`: with `1h`, a session active 5 minutes ago is hot while one near 1 hour old is almost gray; with `3d`, that same 1-hour-old session remains hot.

```tmux
set -g @session-sidebar-heat-colors 'off'
set -g @session-sidebar-heat-recent '10m'
set -g @session-sidebar-heat-max-highlighted '5'
```

To keep recently used projects near the top, set an automatic sorting interval:

```tmux
set -g @session-sidebar-auto-sort-recent '10m'
```

Accepted intervals are minutes (`10m`), hours (`2h`, `24h`), or days (`3d`). `off` disables automatic sorting. For compatibility, `on` is treated as `24h`. This uses real pane activity tracked by the heat state, not a simple session switch. Accidentally switching to a session and switching back does not move it up unless pane output changes there.

Agent bells are separate. If enabled, supported agent hooks can mark a session with a bell when an agent stops or needs attention. The bell clears when that session becomes current in any attached tmux client. While unread, the bell animates with `@session-sidebar-agent-attention-animation`; use `off`, `pulse`, `rainbow`, or `blink`.

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

Force a runtime refresh from the latest GitHub release and restart the daemon/UI:

```bash
~/.tmux/plugins/tmux-session-sidebar/.bin/tmux-session-sidebar self-update
```

This binary shortcut delegates to `scripts/update-runtime.sh`. You can also run that updater script directly if the installed runtime is too old to support `self-update` yet:

```bash
~/.tmux/plugins/tmux-session-sidebar/scripts/update-runtime.sh
```

TPM updates normally run that same command through the managed git update hook. If TPM updates still leave an old binary, use `self-update` or the updater script once, then reload tmux so the plugin can install the hook:

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
make update-runtime
make test-go
make test-runtime-bootstrap
```

`make install` symlinks the current checkout into `~/.tmux/plugins/tmux-session-sidebar`. `make build-runtime` ensures the plugin-local `.bin/tmux-session-sidebar` runtime exists through the centralized `--ensure` path; set `TMUX_SESSION_SIDEBAR_BUILD_FROM_SOURCE=1` to force a source build. Normal plugin installs prefer the latest GitHub release binary and only fall back to a local Go build when the release download is unavailable. `make restart-runtime` rebuilds from source through the same one-shot update path used by the runtime updater, stopping old daemon/UI processes, restoring on failure, and reloading tmux config. `make update-runtime` runs the release refresh and restart path used by TPM's managed update hook. `make uninstall` removes the symlink.
