#!/usr/bin/env bash
set -euo pipefail

DIRNAME_BIN="$(command -v dirname 2>/dev/null || true)"
PWD_BIN="$(command -v pwd 2>/dev/null || true)"
[ -n "$DIRNAME_BIN" ] || { echo 'dirname not found' >&2; exit 1; }
[ -n "$PWD_BIN" ] || { echo 'pwd not found' >&2; exit 1; }
REPO_DIR="$(cd "$($DIRNAME_BIN "${BASH_SOURCE[0]}")/.." && "$PWD_BIN")" || exit 1

work_dir="$(mktemp -d)"
fake_bin="$work_dir/bin"
mkdir -p "$fake_bin"

cleanup() {
  rm -rf "$work_dir"
}
trap cleanup EXIT

cat >"$fake_bin/tmux" <<'EOF'
#!/usr/bin/env bash
case "$1" in
  display-message)
    printf 'alpha\n'
    ;;
  list-sessions)
    printf 'alpha\tattached\t1\n'
    printf '__internal\tdetached\t1\n'
    printf '123\tdetached\t1\n'
    printf '__scratch\tdetached\t1\n'
    printf 'beta\tdetached\t1\n'
    ;;
  has-session)
    exit 0
    ;;
  *)
    echo "unexpected fake tmux call: $*" >&2
    exit 1
    ;;
esac
EOF
chmod +x "$fake_bin/tmux"

TMUX_BIN="$fake_bin/tmux"
# shellcheck source=/dev/null
source "$REPO_DIR/scripts/lib/tmux.sh"

without_numbered="$(sidebar_list_visible_sessions test-client off)"
with_numbered="$(sidebar_list_visible_sessions test-client on)"

if printf '%s\n' "$without_numbered" | grep -Fq $'__internal\t'; then
  echo 'did not expect __internal to appear when numbered sessions are hidden' >&2
  printf 'visible sessions:\n%s\n' "$without_numbered" >&2
  exit 1
fi

if printf '%s\n' "$with_numbered" | grep -Fq $'__scratch\t'; then
  echo 'did not expect __scratch to appear when numbered sessions are shown' >&2
  printf 'visible sessions:\n%s\n' "$with_numbered" >&2
  exit 1
fi

if printf '%s\n' "$without_numbered" | grep -Fq $'123\t'; then
  echo 'did not expect numeric sessions to appear by default' >&2
  printf 'visible sessions:\n%s\n' "$without_numbered" >&2
  exit 1
fi

printf '%s\n' "$with_numbered" | grep -Fq $'123\tdetached\t1\t' || {
  echo 'expected numeric sessions to appear when enabled' >&2
  printf 'visible sessions:\n%s\n' "$with_numbered" >&2
  exit 1
}

second_visible="$(sidebar_visible_session_name_at_index test-client 2 off)" || {
  echo 'expected a second visible session' >&2
  exit 1
}
[ "$second_visible" = 'beta' ] || {
  echo "expected quick-switch index 2 to skip internal sessions and point to beta, got: $second_visible" >&2
  exit 1
}

sync_output="$(sidebar_sync_session_heat __internal)"
[ -z "$sync_output" ] || {
  echo "expected heat sync to ignore internal sessions, got: $sync_output" >&2
  exit 1
}

echo 'ok: sidebar hides internal __ sessions'
