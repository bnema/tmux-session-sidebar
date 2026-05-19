#!/usr/bin/env bash
set -euo pipefail

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_eq() {
  local expected="$1" actual="$2" message="$3"
  if [ "$expected" != "$actual" ]; then
    fail "$message: expected '$expected', got '$actual'"
  fi
}

assert_file_contains() {
  local file="$1" needle="$2" message="$3"
  if ! grep -Fq -- "$needle" "$file"; then
    echo "--- $file ---" >&2
    cat "$file" >&2 || true
    fail "$message: missing '$needle'"
  fi
}

new_fixture() {
  local root plugin fakebin
  root="$(mktemp -d)"
  plugin="$root/plugin"
  fakebin="$root/fakebin"
  mkdir -p "$plugin/scripts" "$plugin/cmd/tmux-session-sidebar" "$fakebin" "$root/gopath/bin"
  cp "$(dirname "$0")/ensure-runtime.sh" "$plugin/scripts/ensure-runtime.sh"
  chmod +x "$plugin/scripts/ensure-runtime.sh"
  cat >"$plugin/go.mod" <<'MOD'
module example.com/tmux-session-sidebar-test

go 1.22
MOD
  cat >"$plugin/.gitignore" <<'IGNORE'
.bin/
IGNORE
  cat >"$plugin/cmd/tmux-session-sidebar/main.go" <<'GO'
package main

func main() {}
GO
  (cd "$plugin" && git init -q && git config user.email test@example.com && git config user.name Test && git add . && git commit -qm initial)

  cat >"$fakebin/go" <<'GOFAKE'
#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  env)
    case "$2" in
      GOBIN) exit 0 ;;
      GOPATH) printf '%s\n' "$TEST_ROOT/gopath" ;;
      *) exit 1 ;;
    esac
    ;;
  version)
    printf 'go version go1.99.0 test/test\n'
    ;;
  build)
    out=""
    while [ "$#" -gt 0 ]; do
      if [ "$1" = "-o" ]; then
        shift
        out="$1"
      fi
      shift || true
    done
    [ -n "$out" ] || { echo 'missing -o' >&2; exit 1; }
    mkdir -p "$(dirname "$out")"
    printf '#!/usr/bin/env bash\necho built-runtime\n' >"$out"
    chmod +x "$out"
    printf 'build %s\n' "$out" >>"$TEST_ROOT/go-build.log"
    ;;
  *)
    echo "unexpected go command: $*" >&2
    exit 1
    ;;
esac
GOFAKE
  chmod +x "$fakebin/go"

  cat >"$fakebin/tmux-session-sidebar" <<'OLD'
#!/usr/bin/env bash
echo stale-path-runtime
OLD
  chmod +x "$fakebin/tmux-session-sidebar"

  printf '%s\n' "$root"
}

run_ensure() {
  local root="$1"
  TEST_ROOT="$root" PATH="$root/fakebin:$PATH" "$root/plugin/scripts/ensure-runtime.sh"
}

test_uses_plugin_local_binary_even_when_path_has_stale_runtime() {
  local root output expected
  root="$(new_fixture)"
  output="$(run_ensure "$root")"
  expected="$root/plugin/.bin/tmux-session-sidebar"
  assert_eq "$expected" "$output" "ensure-runtime should return plugin-local binary"
  assert_file_contains "$root/go-build.log" "build $expected" "ensure-runtime should build plugin-local binary"
}

test_cached_runtime_is_reused_when_fingerprint_matches() {
  local root expected first second builds
  root="$(new_fixture)"
  expected="$root/plugin/.bin/tmux-session-sidebar"
  first="$(run_ensure "$root")"
  second="$(run_ensure "$root")"
  assert_eq "$expected" "$first" "first run should return plugin-local binary"
  assert_eq "$expected" "$second" "second run should return plugin-local binary"
  builds="$(wc -l <"$root/go-build.log" | tr -d ' ')"
  assert_eq "1" "$builds" "matching fingerprint should not rebuild"
}

test_runtime_rebuilds_after_commit_changes() {
  local root expected builds
  root="$(new_fixture)"
  expected="$root/plugin/.bin/tmux-session-sidebar"
  run_ensure "$root" >/dev/null
  printf '\n// changed\n' >>"$root/plugin/cmd/tmux-session-sidebar/main.go"
  (cd "$root/plugin" && git add . && git commit -qm changed)
  assert_eq "$expected" "$(run_ensure "$root")" "changed checkout should still return plugin-local binary"
  builds="$(wc -l <"$root/go-build.log" | tr -d ' ')"
  assert_eq "2" "$builds" "changed fingerprint should rebuild"
}

test_runtime_rebuilds_for_untracked_source_files() {
  local root builds
  root="$(new_fixture)"
  run_ensure "$root" >/dev/null
  mkdir -p "$root/plugin/internal/untracked"
  cat >"$root/plugin/internal/untracked/source.go" <<'GO'
package untracked
GO
  run_ensure "$root" >/dev/null
  builds="$(wc -l <"$root/go-build.log" | tr -d ' ')"
  assert_eq "2" "$builds" "untracked source files should force source fingerprint rebuild"
}

test_uses_plugin_local_binary_even_when_path_has_stale_runtime
test_cached_runtime_is_reused_when_fingerprint_matches
test_runtime_rebuilds_after_commit_changes
test_runtime_rebuilds_for_untracked_source_files

echo "ensure-runtime tests passed"
