# tmux Session Sidebar

A TPM plugin that adds an on-demand left sidebar pane for project-first tmux session management.
It lets you switch sessions in the same tmux client, create project-backed sessions from configured roots, create ad-hoc sessions, rename sessions, and kill sessions without leaving tmux.

## Requirements

- `tmux 3.6+`
- `bash`
- `fzf` optional but recommended

`tmux 3.6+` is required because the plugin marks the sidebar with a pane-scoped user option.

## Installation

### With Tmux Plugin Manager

Add this to `~/.tmux.conf`:

```tmux
set -g @plugin 'brice/tmux-session-sidebar'
```

Keep TPM itself loaded at the bottom of the file:

```tmux
run '~/.tmux/plugins/tpm/tpm'
```

Then press `prefix + I` inside tmux.

### Manual install

```bash
git clone https://github.com/brice/tmux-session-sidebar ~/.tmux/plugins/tmux-session-sidebar
```

Add this to `~/.tmux.conf`:

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
| `@session-sidebar-key` | `B` | Key used after the tmux prefix to toggle the sidebar |
| `@session-sidebar-width` | `30%` | Width passed to the left split |
| `@session-sidebar-project-roots` | `$HOME/projects` | Colon-separated roots searched for project sessions |
| `@session-sidebar-use-fzf` | `on` | Use `fzf` when it is installed |
| `@session-sidebar-close-after-switch` | `on` | Close the sidebar pane after a successful session switch |

Example:

```tmux
set -g @session-sidebar-key 'B'
set -g @session-sidebar-width '25%'
set -g @session-sidebar-project-roots "$HOME/projects:$HOME/dev/projects"
set -g @session-sidebar-use-fzf 'on'
set -g @session-sidebar-close-after-switch 'on'
```

## Usage

### Open the sidebar

- `prefix + B` opens or closes the left sidebar in the current window.

The sidebar is a real tmux pane, not a global overlay. If you switch sessions and `@session-sidebar-close-after-switch` is `on`, the pane closes and can be reopened in the new session.

### Session browser

Each row shows:

- session name
- current-session marker (`*`)
- attached or detached state
- window count

### Keys in `fzf` mode

These keys are used when `fzf` is available and `@session-sidebar-use-fzf` is not `off`.

| Key | Action |
| --- | --- |
| `Enter` | Switch to the selected session |
| `Alt+n` | Create or switch to a project session |
| `Alt+a` | Create or switch to an ad-hoc session |
| `Alt+r` | Rename the selected session |
| `Alt+x` | Kill the selected session |
| `Esc` | Close the sidebar |

The modified `Alt+...` bindings are intentional. They keep plain letters available for fuzzy search instead of stealing them as commands.

### Keys in fallback mode

Fallback mode is used when `fzf` is unavailable or when:

```tmux
set -g @session-sidebar-use-fzf 'off'
```

Prompt actions:

- `[number]` switch to a session
- `n` create or switch to a project session
- `a` create or switch to an ad-hoc session
- `r` rename a session
- `x` kill a session
- `q` close the sidebar

For rename and kill in fallback mode, pressing `Enter` at the session-number prompt targets the current session.

## Behavior notes

### Project session creation

- The plugin lists one directory level under each configured project root.
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

- `on`: after a successful switch, the sidebar pane is closed.
- `off`: after a successful switch, the old sidebar pane remains alive in the old session. This does **not** create a global cross-session sidebar.

## Troubleshooting

### Nothing happens when I press `prefix + B`

Check that the plugin is loaded:

```bash
tmux list-keys -T prefix B
```

You should see a `run-shell` binding for `scripts/open-sidebar.sh`.

### `fzf` is installed but I want the plain fallback menu

Set:

```tmux
set -g @session-sidebar-use-fzf 'off'
```

### I switched sessions and the sidebar disappeared

That is expected when `@session-sidebar-close-after-switch` is `on`. Reopen it with `prefix + B` in the new session.

### I switched sessions and the old sidebar stayed behind

That is expected when `@session-sidebar-close-after-switch` is `off`. The pane stays in the old session; it is not a global sidebar.

### My configured project roots are ignored

They must be colon-separated and must already exist on disk.

Example:

```tmux
set -g @session-sidebar-project-roots "$HOME/projects:$HOME/dev/projects"
```

## Development notes

The implementation is shell-first and is designed to be testable against isolated tmux sockets.
Human-visible UI checks are still useful, but the core pane/session behavior can be validated headlessly with `tmux -L <socket>`.
