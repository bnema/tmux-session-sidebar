# tmux Session Sidebar

A TPM plugin that adds an on-demand left sidebar pane for project-first tmux session management.
Switch sessions, create project-backed sessions from a directory picker, rename sessions, and
more — all from a real tmux left pane in the same terminal client.

## Installation

### With Tmux Plugin Manager (recommended)

Add to `~/.tmux.conf`:

```tmux
set -g @plugin 'brice/tmux-session-sidebar'
```

Then press `prefix + I` to fetch and load the plugin.

### Manual installation

```bash
git clone https://github.com/brice/tmux-session-sidebar ~/clone/path
```

Add to `~/.tmux.conf`:

```tmux
run-shell ~/clone/path/tmux-session-sidebar.tmux
```

Reload with `tmux source-file ~/.tmux.conf`.

## Requirements

- **tmux 3.6+** (needed for pane-scoped user options)
- **fzf** (optional; the sidebar falls back to tmux prompts when `fzf` is unavailable)
