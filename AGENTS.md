# Agent Instructions

## What this project is

- Go 1.26 TPM plugin/runtime for a left-hand tmux session sidebar.
- The Go daemon owns the sidebar UI pane, parks it in hidden session `__tmux-session-sidebar`, and shell scripts bootstrap/install/update the runtime binary.

## Architecture boundaries

- `cmd/tmux-session-sidebar`: main entry point and CLI wiring; keep it as thin as possible.
- `internal/app`: target boundary for new changes is runtime/bootstrap orchestration and dependency injection only.
- `internal/core`: target home for all entities and business logic; keep it adapter-free.
- `internal/ports`: interfaces used by app/core. `internal/ports/mocks` is generated; regenerate with `make mocks`, never hand-edit.
- `internal/adapters/*`: concrete tmux, git, filesystem, IPC, logging, release, process, and watcher integrations behind ports.
- `scripts/`: plugin/runtime shell helpers and their shell tests.
- `plans/`: executor-oriented implementation plans; read the relevant plan before executing one.

## Verification commands

- Focused Go tests: `make test-go`
- Shell/bootstrap tests: `make test-runtime-bootstrap`
- Race tests: `make test-race`
- Lint: `make lint`
- CI-equivalent gate: `make ci`

## Runtime/log paths

- State root: `${XDG_STATE_HOME:-$HOME/.local/state}/tmux-session-sidebar`.
- Live daemon stderr log inside tmux: `${XDG_STATE_HOME:-$HOME/.local/state}/tmux-session-sidebar/servers/<hash>/errors.log`.
- Outside tmux, legacy log path: `${XDG_STATE_HOME:-$HOME/.local/state}/tmux-session-sidebar/errors.log`.
- Optional activity debug log: `${XDG_STATE_HOME:-$HOME/.local/state}/tmux-session-sidebar/activity.log`.
