#!/usr/bin/env bash
set -euo pipefail

DIRNAME_BIN="$(command -v dirname 2>/dev/null || true)"
PWD_BIN="$(command -v pwd 2>/dev/null || true)"
[ -n "$DIRNAME_BIN" ] || { echo 'dirname not found' >&2; exit 1; }
[ -n "$PWD_BIN" ] || { echo 'pwd not found' >&2; exit 1; }
REPO_DIR="$(cd "$($DIRNAME_BIN "${BASH_SOURCE[0]}")/.." && "$PWD_BIN")" || exit 1

work_dir="$(mktemp -d)"
fake_fzf="$work_dir/fzf"
trap 'rm -rf "$work_dir"' EXIT

cat >"$fake_fzf" <<'EOF'
#!/usr/bin/env bash
exit "${SESSION_SIDEBAR_FAKE_FZF_STATUS:-0}"
EOF
chmod +x "$fake_fzf"

run_filtered_entries() {
  local fzf_status="$1"
  SESSION_SIDEBAR_FAKE_FZF_STATUS="$fzf_status" bash -c '
    set -euo pipefail
    source "$1/scripts/lib/sidebar-render.sh"
    active_filter="alpha"
    FZF_BIN="$2"
    render_session_entries() {
      printf "alpha\tlabel\n"
    }
    set +e
    filtered_session_entries >/dev/null
    status="$?"
    set -e
    exit "$status"
  ' _ "$REPO_DIR" "$fake_fzf"
}

set +e
run_filtered_entries 1
no_matches_status="$?"
run_filtered_entries 2
error_status="$?"
set -e

[ "$no_matches_status" -eq 0 ] || {
  echo "expected fzf --filter exit 1 (no matches) to be treated as success, got $no_matches_status" >&2
  exit 1
}

[ "$error_status" -eq 2 ] || {
  echo "expected fzf --filter exit 2 to propagate, got $error_status" >&2
  exit 1
}

echo 'ok: filtered render treats no matches as success and propagates fzf errors'
