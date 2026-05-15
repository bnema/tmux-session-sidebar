#!/usr/bin/env bash
set -euo pipefail

DIRNAME_BIN="$(command -v dirname 2>/dev/null || true)"
PWD_BIN="$(command -v pwd 2>/dev/null || true)"
[ -n "$DIRNAME_BIN" ] || { echo 'dirname not found' >&2; exit 1; }
[ -n "$PWD_BIN" ] || { echo 'pwd not found' >&2; exit 1; }
REPO_DIR="$(cd "$($DIRNAME_BIN "${BASH_SOURCE[0]}")/.." && "$PWD_BIN")" || exit 1

work_dir="$(mktemp -d)"
fake_bin="$work_dir/bin"
curl_calls="$work_dir/curl-calls.txt"
curl_count="$work_dir/curl-count.txt"
refresh_socket="$work_dir/tss-refresh.sock"
mkdir -p "$fake_bin"

cleanup() {
  rm -rf "$work_dir"
}
trap cleanup EXIT

cat >"$fake_bin/sleep" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "$fake_bin/sleep"

cat >"$fake_bin/curl" <<'EOF'
#!/usr/bin/env bash
count=0
if [ -f "$TEST_CURL_COUNT" ]; then
  count="$(cat "$TEST_CURL_COUNT")"
fi
count=$((count + 1))
printf '%s' "$count" > "$TEST_CURL_COUNT"
printf '%s\n' "$*" >> "$TEST_CURL_CALLS"
if [ "$count" -eq 1 ]; then
  printf 'ok\n'
  exit 0
fi
exit 1
EOF
chmod +x "$fake_bin/curl"

env -u TMUX PATH="$fake_bin:$PATH" TEST_CURL_CALLS="$curl_calls" TEST_CURL_COUNT="$curl_count" "$REPO_DIR/scripts/sidebar.sh" \
  --fzf-refresh-loop \
  --socket "$refresh_socket" \
  --interval 1 \
  --client fake-client \
  --source-path "$work_dir" \
  --show-numbered-sessions on

[ -s "$curl_calls" ] || {
  echo 'expected refresh loop to post at least one reload command' >&2
  exit 1
}

call_count="$(wc -l < "$curl_calls")"
[ "$call_count" -ge 2 ] && [ "$call_count" -le 3 ] || {
  echo "expected refresh loop to stop after curl failure (2-3 calls), got $call_count calls" >&2
  cat "$curl_calls" >&2
  exit 1
}

grep -Fq -- "--unix-socket $refresh_socket http -d reload-sync(" "$curl_calls" || {
  echo 'expected curl call to target the fzf unix socket with reload-sync payload' >&2
  cat "$curl_calls" >&2
  exit 1
}

grep -Fq -- '--client fake-client' "$curl_calls" || {
  echo 'expected reload payload to include the client name' >&2
  cat "$curl_calls" >&2
  exit 1
}

grep -Fq -- '--show-numbered-sessions on' "$curl_calls" || {
  echo 'expected reload payload to preserve numbered-session visibility state' >&2
  cat "$curl_calls" >&2
  exit 1
}

grep -Fq -- '--source-path' "$curl_calls" || {
  echo 'expected reload payload to preserve source path' >&2
  cat "$curl_calls" >&2
  exit 1
}

grep -Fq -- '--render-entries' "$curl_calls" || {
  echo 'expected reload payload to request render-only mode' >&2
  cat "$curl_calls" >&2
  exit 1
}

echo 'ok: fzf refresh loop posts sidebar reload commands'
