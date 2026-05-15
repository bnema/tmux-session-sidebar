#!/usr/bin/env bash
set -euo pipefail

DIRNAME_BIN="$(command -v dirname 2>/dev/null || true)"
PWD_BIN="$(command -v pwd 2>/dev/null || true)"
[ -n "$DIRNAME_BIN" ] || { echo 'dirname not found' >&2; exit 1; }
[ -n "$PWD_BIN" ] || { echo 'pwd not found' >&2; exit 1; }
REPO_DIR="$(cd "$($DIRNAME_BIN "${BASH_SOURCE[0]}")/.." && "$PWD_BIN")" || exit 1
work_dir="$(mktemp -d)"
stdout_file="$work_dir/stdout.txt"
stderr_file="$work_dir/stderr.txt"

cleanup() {
  rm -rf "$work_dir"
}
trap cleanup EXIT

# shellcheck source=/dev/null
source "$REPO_DIR/scripts/lib/tmux.sh"

# Exercise sidebar_heat_score_after_interval with an intentionally extreme
# detached-session decay interval: seed score 10, elapsed 432000000 seconds
# (~5000 days), attached count 0, and 8-hour half-life 28800 seconds.
sidebar_heat_score_after_interval 10 432000000 0 28800 >"$stdout_file" 2>"$stderr_file"
output="$(cat "$stdout_file")"
stderr="$(cat "$stderr_file")"

[ -z "$stderr" ] || {
  echo 'expected large decay intervals to avoid awk exp underflow warnings' >&2
  printf 'stderr:\n%s\n' "$stderr" >&2
  exit 1
}

[[ -n "$output" && "$output" =~ ^[+-]?([0-9]+([.][0-9]*)?|[.][0-9]+)([eE][+-]?[0-9]+)?$ ]] || {
  echo "expected numeric heat output, got: ${output:-<empty>}" >&2
  exit 1
}

awk -v value="$output" 'BEGIN { exit !(value >= 0 && value <= 0.000001) }' || {
  echo "expected sufficiently old detached heat to decay to ~0, got: ${output:-<empty>}" >&2
  exit 1
}

echo 'ok: huge heat decay intervals do not emit awk underflow warnings'
