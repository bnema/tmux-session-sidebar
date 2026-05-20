# tmux Session Sidebar

A TPM plugin for fast tmux session switching. It opens a full-height left sidebar so you can switch sessions, create project sessions, create ad-hoc sessions, rename sessions, and kill sessions without leaving tmux.

## Requirements

- tmux 3.6+
- Go 1.26+
- bash for the TPM bootstrap script

`fzf` is not required. The old `@session-sidebar-use-fzf` option is still accepted for compatibility, but the Go UI ignores it.

## Install

### TPM

Add the plugin to `~/.tmux.conf`:

```tmux
set -g @plugin 'bnema/tmux-session-sidebar'
```

Keep TPM loaded at the bottom of the file:

```tmux
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

Reload tmux:

```bash
tmux source-file ~/.tmux.conf
```

The bootstrap uses `scripts/ensure-runtime.sh`. It builds a plugin-local Go runtime at `.bin/tmux-session-sidebar` and records a build fingerprint. After TPM updates the plugin checkout, the next reload rebuilds the runtime automatically when the fingerprint changes.

## Local development install

From this checkout:

```bash
make install
```

This symlinks the repo into `~/.tmux/plugins/tmux-session-sidebar`.

The Go runtime is built automatically into the plugin checkout when tmux loads the plugin. To force a rebuild during local development, remove `.bin/tmux-session-sidebar` or `.bin/.build-fingerprint`, then reload tmux.

Remove the local plugin symlink with:

```bash
make uninstall
```

## Configuration

Configure tmux options in `~/.tmux.conf`.

| Option | Default | Meaning |
| --- | --- | --- |
| `@session-sidebar-key` | `M-b` | Global key for opening or closing the sidebar |
| `@session-sidebar-width` | `20` | Sidebar width passed to `tmux split-window -l` |
| `@session-sidebar-project-roots` | `$HOME/projects` | Colon-separated roots for project sessions |
| `@session-sidebar-close-after-switch` | `off` | Close the sidebar after switching when set to `on` |
| `@session-sidebar-heat-colors` | `on` | Color sessions by recent activity |
| `@session-sidebar-heat-half-life-hours` | `8` | Heat decay half-life |
| `@session-sidebar-heat-stale-hours` | `24` | Hours before a session fades to stale |
| `@session-sidebar-heat-refresh-seconds` | `300` | Heat refresh interval while the sidebar is open |
| `@session-sidebar-use-fzf` | `on` | Compatibility option; ignored by the Go UI |

Example:

```tmux
set -g @session-sidebar-key 'M-b'
set -g @session-sidebar-width '20'
set -g @session-sidebar-project-roots "$HOME/projects:$HOME/dev/projects"
set -g @session-sidebar-close-after-switch 'off'
set -g @session-sidebar-heat-colors 'on'
set -g @session-sidebar-heat-half-life-hours '8'
set -g @session-sidebar-heat-stale-hours '24'
set -g @session-sidebar-heat-refresh-seconds '300'
```

## Usage

### Open and close

Press `Alt+b` to open or close the sidebar.

The sidebar opens as a full-height left split in the current tmux window. If `@session-sidebar-close-after-switch` is `off`, the sidebar stays logically open and follows the client after session switches.

### Sidebar keys

The footer is compact by default. Press `M-?` inside the sidebar to show or hide the full key list.

| Key | Action |
| --- | --- |
| `j` / `k` or arrows | Move selection |
| `/` | Filter sessions |
| `Enter` | Switch session, apply filter, or choose project |
| `Esc` | Leave filter/project/confirmation mode, or close the sidebar |
| `M-n` | Open the inline project picker |
| `M-g` | Create or switch to a session for the current git repo |
| `M-a` | Create or switch to an ad-hoc session for the current directory |
| `M-r` | Rename the selected session |
| `M-x` | Kill the selected session after inline confirmation |
| `M-h` | Show or hide numeric session names |
| `M-?` | Show or hide key help |

Kill confirmation happens inside the sidebar: press `y` to confirm, `n`, `Enter`, or `Esc` to cancel.

### Global quick switch

These work without opening the sidebar:

- `Ctrl+1` through `Ctrl+9` switch to visible sessions 1 through 9
- `Ctrl+0` switches to visible session 10

Numeric session names and names beginning with `__` are hidden from the sidebar by default. `M-h` toggles numeric session visibility.

### Session restore

The sidebar remembers named sessions that it creates or observes and recreates them as empty shell sessions when the tmux server is restarted. Numeric sessions and sessions beginning with `__` are ignored. Killing a session with `M-x` removes it from future restore.

## Project sessions

`M-n` opens an inline project picker using the directories under `@session-sidebar-project-roots`. The picker filters by project name. Press `Enter` to create or switch to that project session.

`M-g` creates or switches to a session for the current pane's git repository root, even if it is outside the configured project roots.

Project session names are derived from directory basenames and normalized for tmux. If the name already exists, the plugin switches to it instead of creating a duplicate.

## Ad-hoc sessions

`M-a` starts a session in the current pane path, named after that path's normalized directory basename. If the session already exists, the plugin switches to it.

## Rename and kill

`M-r` prompts for a new name for the selected session.

`M-x` asks for inline confirmation before killing the selected session. The plugin refuses to kill the last remaining session.

## Heat colors

When heat colors are enabled, sessions are colored by recent dwell time:

- current: brightest green
- hot, warm, cool: progressively dimmer greens
- stale: gray after `@session-sidebar-heat-stale-hours`

Heat decays over time. Lifetime visit counts are not used. Colors degrade by terminal capability: RGB, 256-color, basic color, then plain text.

Disable heat colors with:

```tmux
set -g @session-sidebar-heat-colors 'off'
```

## Troubleshooting

### Alt+b does nothing

Check that the plugin is loaded and the global binding exists:

```bash
tmux list-keys -T root M-b
```

You should see a `run-shell` binding for `tmux-session-sidebar sidebar toggle`.

If you changed `@session-sidebar-key`, check that key instead.

### The sidebar disappeared after switching

That is expected when this option is enabled:

```tmux
set -g @session-sidebar-close-after-switch 'on'
```

Set it to `off` to keep the sidebar open across switches.

### Project roots are ignored

Use colon-separated directories that already exist:

```tmux
set -g @session-sidebar-project-roots "$HOME/projects:$HOME/dev/projects"
```

## Development

Useful checks:

```bash
go test ./...
go vet ./...
make test-runtime-bootstrap
```

The current runtime is Go-first. Shell is only used for TPM/bootstrap integration.
