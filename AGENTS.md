# Agent Instructions

## Architecture map

- `cmd/tmux-session-sidebar` wires CLI commands, runtime dependencies, and UI runner entry points.
- `internal/app` coordinates application use cases: daemon loops, hooks, metadata capture, sidebar state, restore/resurrect, IPC, and tmux-facing orchestration.
- `internal/core` contains domain logic for config, sessions, sidebar layout, heat, search, persisted state, runtime aggregation, and versioning. Keep it adapter-free.
- `internal/ports` defines boundaries used by app/core code. Adapters implement these ports; avoid importing concrete adapters into core logic.
- `internal/adapters/*` contains concrete integrations for tmux, git, filesystem watching, state storage, IPC, logging, releases, and process execution.
- `internal/ports/mocks` contains generated mocks. Regenerate with `make mocks`; do not hand-edit generated files.
- `scripts/` contains plugin/runtime shell helpers and their shell tests.
- `plans/` contains executor-oriented implementation plans for larger changes.

## Verification commands

Use focused tests while iterating, then run the relevant gate before handing off:

```bash
make test-go
make test-runtime-bootstrap
make test-race
make lint
make ci
```

`make ci` is the CI-equivalent gate: shell/bootstrap tests, race-enabled Go tests, and lint.

## Safety constraints

- Treat tmux runtime state and files under the user's runtime directories as user data. Prefer temporary test directories and injected ports in tests.
- Be careful with release/update scripts and runtime binary management; avoid changing install/update semantics without shell test coverage.
- Hook installers edit user-owned agent config files. Preserve user content, comments, permissions, and write config updates atomically.
- Keep dependency direction clean: app may compose ports and adapters, core should remain independent of adapters, and adapters should stay behind `internal/ports` interfaces.
- Prefer test-first changes for behavior and performance-sensitive code paths. Preserve behavior for missing tmux sessions, missing git repositories, detached HEADs, malformed state, and disabled config flags.
