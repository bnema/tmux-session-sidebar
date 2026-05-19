#!/usr/bin/env bash
set -euo pipefail

DIRNAME_BIN="$(command -v dirname 2>/dev/null || true)"
PWD_BIN="$(command -v pwd 2>/dev/null || true)"
GO_BIN="$(command -v go 2>/dev/null || true)"
GIT_BIN="$(command -v git 2>/dev/null || true)"
FIND_BIN="$(command -v find 2>/dev/null || true)"
SORT_BIN="$(command -v sort 2>/dev/null || true)"
CKSUM_BIN="$(command -v cksum 2>/dev/null || true)"
[ -n "$DIRNAME_BIN" ] || { echo 'tmux-session-sidebar: dirname not found' >&2; exit 1; }
[ -n "$PWD_BIN" ] || { echo 'tmux-session-sidebar: pwd not found' >&2; exit 1; }
PLUGIN_DIR="$(cd "$("$DIRNAME_BIN" "${BASH_SOURCE[0]}")/.." && "$PWD_BIN")" || exit 1
BIN_DIR="$PLUGIN_DIR/.bin"
runtime_bin="$BIN_DIR/tmux-session-sidebar"
stamp_file="$BIN_DIR/.build-fingerprint"

source_fingerprint() {
  local go_version git_rev
  go_version="$($GO_BIN version 2>/dev/null || true)"

  if [ -n "$GIT_BIN" ]; then
    git_rev="$($GIT_BIN -C "$PLUGIN_DIR" rev-parse HEAD 2>/dev/null || true)"
    if [ -n "$git_rev" ] && [ -z "$("$GIT_BIN" -C "$PLUGIN_DIR" status --porcelain --untracked-files=all 2>/dev/null)" ]; then
      printf 'git:%s\ngo:%s\n' "$git_rev" "$go_version"
      return 0
    fi
  fi

  [ -n "$FIND_BIN" ] || { echo 'tmux-session-sidebar: find not found' >&2; exit 1; }
  [ -n "$SORT_BIN" ] || { echo 'tmux-session-sidebar: sort not found' >&2; exit 1; }
  [ -n "$CKSUM_BIN" ] || { echo 'tmux-session-sidebar: cksum not found' >&2; exit 1; }

  {
    printf 'source\ngo:%s\n' "$go_version"
    cd "$PLUGIN_DIR"
    for path in go.mod go.sum cmd internal core adapters ports; do
      [ -e "$path" ] || continue
      if [ -d "$path" ]; then
        "$FIND_BIN" "$path" -type f
      else
        printf '%s\n' "$path"
      fi
    done | "$SORT_BIN" | while IFS= read -r file; do
      printf '%s ' "$file"
      "$CKSUM_BIN" "$file"
    done
  }
}

[ -n "$GO_BIN" ] || { echo 'tmux-session-sidebar: go not found; cannot build tmux-session-sidebar runtime' >&2; exit 1; }

mkdir -p "$BIN_DIR"
fingerprint="$(source_fingerprint)"

if [ ! -x "$runtime_bin" ] || [ ! -f "$stamp_file" ] || [ "$(cat "$stamp_file" 2>/dev/null || true)" != "$fingerprint" ]; then
  echo "tmux-session-sidebar: building runtime at $runtime_bin" >&2
  (cd "$PLUGIN_DIR" && "$GO_BIN" build -o "$runtime_bin" ./cmd/tmux-session-sidebar)
  printf '%s\n' "$fingerprint" >"$stamp_file"
fi

printf '%s\n' "$runtime_bin"
