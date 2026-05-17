#!/usr/bin/env bash
set -euo pipefail

DIRNAME_BIN="$(command -v dirname 2>/dev/null || true)"
PWD_BIN="$(command -v pwd 2>/dev/null || true)"
GO_BIN="$(command -v go 2>/dev/null || true)"
[ -n "$DIRNAME_BIN" ] || { echo 'tmux-session-sidebar: dirname not found' >&2; exit 1; }
[ -n "$PWD_BIN" ] || { echo 'tmux-session-sidebar: pwd not found' >&2; exit 1; }
PLUGIN_DIR="$(cd "$($DIRNAME_BIN "${BASH_SOURCE[0]}")/.." && "$PWD_BIN")" || exit 1

runtime_from_path="$(command -v tmux-session-sidebar 2>/dev/null || true)"
if [ -n "$runtime_from_path" ] && [ -x "$runtime_from_path" ]; then
  printf '%s\n' "$runtime_from_path"
  exit 0
fi

[ -n "$GO_BIN" ] || { echo 'tmux-session-sidebar: go not found and tmux-session-sidebar is not in PATH' >&2; exit 1; }

go_bin_dir="$($GO_BIN env GOBIN 2>/dev/null || true)"
if [ -z "$go_bin_dir" ]; then
  go_path="$($GO_BIN env GOPATH)"
  go_bin_dir="$go_path/bin"
fi
runtime_bin="$go_bin_dir/tmux-session-sidebar"

mkdir -p "$go_bin_dir"
(cd "$PLUGIN_DIR" && "$GO_BIN" install ./cmd/tmux-session-sidebar)
printf '%s\n' "$runtime_bin"
