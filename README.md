# tmux Session Sidebar

A TPM plugin that adds an on-demand left sidebar pane for project-first tmux session management.
It lets you switch sessions in the same tmux client, create project-backed sessions from configured roots, create ad-hoc sessions, rename sessions, and kill sessions without leaving tmux.

## Requirements

- `tmux 3.6+`
- `bash` for the small TPM bootstrap
- Go-built `tmux-session-sidebar` runtime; `scripts/ensure-runtime.sh` builds it locally with `go` when `bin/tmux-session-sidebar` is missing

The Go runtime owns sidebar UI, actions, state reconciliation, and tmux command dispatch. `fzf` is no longer required; `@session-sidebar-use-fzf` is accepted for compatibility but ignored by the Go UI.

## Installation

### With Tmux Plugin Manager

Add this to `~/.tmux.conf`:

```tmux
set -g @plugin 'bnema/tmux-session-sidebar'
```

If you are using a fork, replace `bnema` with your GitHub user or organization.

Keep TPM itself loaded at the bottom of the file:

```tmux
run '~/.tmux/plugins/tpm/tpm'
```

Then reload tmux or press `prefix + I` inside tmux.

### Manual install

Clone the repository into the standard TPM plugin directory:

```bash
git clone https://github.com/bnema/tmux-session-sidebar ~/.tmux/plugins/tmux-session-sidebar
```

Or install a specific release tag:

```bash
git clone --branch v0.1.0 --depth 1 https://github.com/bnema/tmux-session-sidebar ~/.tmux/plugins/tmux-session-sidebar
```

Then add this to `~/.tmux.conf`:

```tmux
run-shell ~/.tmux/plugins/tmux-session-sidebar/tmux-session-sidebar.tmux
```

Reload tmux:

```bash
tmux source-file ~/.tmux.conf
```

### Quick local testing

For local development from this checkout:

```bash
make install
```

That symlinks this repository into `~/.tmux/plugins/tmux-session-sidebar`.

To remove the test install:

```bash
make uninstall
```

## Configuration

Configure the plugin with tmux user options in `~/.tmux.conf`.

| Option | Default | Meaning |
| --- | --- | --- |
| `@session-sidebar-key` | `M-b` | Global tmux key used to toggle the sidebar |
| `@session-sidebar-width` | `20` | Width passed to the left split; defaults to a fixed column count |
| `@session-sidebar-project-roots` | `$HOME/projects` | Colon-separated roots searched for project sessions |
| `@session-sidebar-use-fzf` | `on` | Compatibility option only; the Go UI ignores it |
| `@session-sidebar-close-after-switch` | `off` | `on` closes the sidebar after a switch; `off` keeps it open and follows the client into the new current window/session |
| `@session-sidebar-heat-colors` | `on` | Color session rows by recent working-set heat; set to `off` to disable heat colors and periodic heat refresh |
| `@session-sidebar-heat-half-life-hours` | `8` | Heat decay half-life in hours for recent dwell time |
| `@session-sidebar-heat-stale-hours` | `24` | Fade a session to gray after it has not been seen for this many hours |
| `@session-sidebar-heat-refresh-seconds` | `300` | Runtime heat refresh interval while sidebars are open |

Example:

```tmux
set -g @session-sidebar-key 'M-b'
set -g @session-sidebar-width '20'
set -g @session-sidebar-project-roots "$HOME/projects:$HOME/dev/projects"
set -g @session-sidebar-use-fzf 'on'
set -g @session-sidebar-close-after-switch 'off'
set -g @session-sidebar-heat-colors 'on'
# set -g @session-sidebar-heat-colors 'off'  # disable heat colors and periodic heat refresh
set -g @session-sidebar-heat-half-life-hours '8'
set -g @session-sidebar-heat-stale-hours '24'
set -g @session-sidebar-heat-refresh-seconds '300'
```

## Usage

### Open the sidebar

- `Alt+b` opens or closes the left sidebar in the current window.

The sidebar is a real tmux pane backed by a Go runtime. If the current window already has pane splits, the sidebar still opens as a full-height left pane for the whole window. By default it remains logically open after a switch and follows the client into the new current window/session. If you set `@session-sidebar-close-after-switch` to `on`, the pane closes after a successful switch.

The sidebar browser still fills the pane height. The default width is now a fixed column count instead of a percentage, though tmux-style percentage values still work if you set them explicitly.

### Session browser

Each row shows:

- session name
- quick-switch badge (`[1]` … `[9]`, then `[0]` for the 10th quick-switchable session)
- current-session marker (`*`)
- optional heat color based on your recent working set

Purely numeric session names are hidden by default to reduce noise. Toggle them on or off from the sidebar when needed.

With heat colors enabled, the current session is brightest green, then hot, warm, and cool sessions fade through progressively darker greens. Sessions you have not seen for more than `@session-sidebar-heat-stale-hours` move to the stale bucket and fade to gray. The score is based on recent dwell time with decay, not lifetime visit counts.

### Global quick-switch keys

These work without opening the sidebar:

- `Ctrl+1` through `Ctrl+9` switch to visible sessions 1 through 9
- `Ctrl+0` switches to visible session 10

The quick-switch order matches the sidebar's default visible session list, so purely numeric session names are skipped unless you switch to them some other way. The sidebar shows this mapping with `[1]` through `[9]` and `[0]` badges beside the first 10 quick-switchable sessions.

### Sidebar keys

The Go UI starts in browse mode, so search is inactive until you press `/`.

| Key | Action |
| --- | --- |
| `j` / `k` | Move down or up in browse mode |
| `/` | Enter search mode |
| `Enter` | Apply filter (search mode) or switch session (browse mode) |
| `n` | Create or switch to a project session |
| `g` | Create or switch to a project session from the current pane's git repo root |
| `a` | Create or switch to an ad-hoc session |
| `r` | Rename the selected session |
| `x` | Kill the selected session |
| `h` | Show or hide purely numeric session names |
| `Esc` | Clear search and return to browse mode, or close the sidebar from browse mode |
| `M-?` | Show or hide key help |

After you press `Enter` in search mode, the current filter stays applied and the sidebar returns to browse mode.

## Behavior notes

### Project session creation

- The plugin lists one directory level under each configured project root.
- Configured project roots may be symlinked paths; the plugin resolves them before listing projects.
- The picker shows the project name first, with its parent path as context.
- `g` treats the current pane's enclosing git repository root as a project, even if it is outside configured project roots.
- The default session name is derived from the project directory basename.
- Names are normalized to a tmux-safe form.
- If the derived name already exists, the plugin switches to that session instead of creating a suffixed variant.

### Ad-hoc session creation

- Ad-hoc sessions start in the current pane path.
- If the name already exists, the plugin switches to the existing session.

### Rename and kill

- Rename rejects invalid names and duplicate names.
- Kill asks for confirmation.
- The plugin refuses to kill the last remaining session.

### `@session-sidebar-close-after-switch`

- Default `off`: after a successful switch, the sidebar remains logically open and follows the client into the new current window/session.
- `on`: after a successful switch, the sidebar pane is closed.
- If you already had an older version loaded in a running tmux server, unset this option or restart the tmux server to pick up the new semantics.

### Session heat colors

- Heat is tracked from recent dwell time while a session is attached, then decays over time.
- The five heat buckets are `current`, `hot`, `warm`, `cool`, and `stale`: the current session is brightest green, hot/warm/cool fade through progressively darker greens, and stale sessions turn gray after `@session-sidebar-heat-stale-hours`.
- Lifetime visit counts are intentionally **not** used, so a session only stays hot if it is part of your recent working set.
- Default half-life is `8` hours via `@session-sidebar-heat-half-life-hours`.
- Sessions unseen for `24` hours fade to gray by default via `@session-sidebar-heat-stale-hours`.
- Set `@session-sidebar-heat-colors` to `off` if you prefer simpler output or want to avoid the periodic heat-refresh overhead.
- Plugin-driven session switches trigger an immediate sidebar rerender, so current-session markers and heat colors update without waiting for the periodic refresh.
- Manual `tmux switch-client` session changes also trigger the same refresh through a tmux `client-session-changed` hook.
- The runtime lazily refreshes heat colors every `300` seconds by default via `@session-sidebar-heat-refresh-seconds` while sidebars are open.
- Colors degrade gracefully by terminal capability: RGB when available, then 256-color, then basic colors, then plain text.

### Zoomed windows

If you trigger the sidebar from a zoomed pane, tmux first unzooms the pane and then opens the sidebar. This is native tmux zoom/unzoom behavior. The plugin does not add custom zoom restoration logic in v1.

## Troubleshooting

### Nothing happens when I press `prefix + b`

Check that the plugin is loaded:

```bash
tmux list-keys -T prefix b
```

You should see a `run-shell` binding for `tmux-session-sidebar sidebar toggle`.

### I switched sessions and the sidebar disappeared

That is expected when `@session-sidebar-close-after-switch` is `on`. Reopen it with `prefix + b` in the new session.

### I switched sessions and the sidebar followed me

That is the default behavior. When `@session-sidebar-close-after-switch` is `off`, the logical sidebar follows the client into the new current window/session.

### I want simpler output or less periodic refresh work

Set:

```tmux
set -g @session-sidebar-heat-colors 'off'
```

This disables heat coloring and the periodic heat-refresh work that supports it.

### My configured project roots are ignored

They must be colon-separated and must already exist on disk.

Example:

```tmux
set -g @session-sidebar-project-roots "$HOME/projects:$HOME/dev/projects"
```

## Development notes

The implementation is shell-first and is designed to be testable against isolated tmux sockets.
Human-visible UI checks are still useful, but the core pane/session behavior can be validated headlessly with `tmux -L <socket>`.
