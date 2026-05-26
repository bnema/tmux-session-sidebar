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
    if [ "${TEST_INVALID_GO_BUILD:-}" = 1 ]; then
      cat >"$out" <<'RUNTIME'
#!/usr/bin/env bash
echo invalid-runtime
RUNTIME
    else
      cat >"$out" <<'RUNTIME'
#!/usr/bin/env bash
if [ "${1:-}" = version ]; then
  echo "tmux-session-sidebar test"
else
  echo built-runtime
fi
RUNTIME
    fi
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

run_ensure_without_go() {
  local root="$1" nogobin
  nogobin="$root/nogobin"
  mkdir -p "$nogobin"
  ln -s /usr/bin/bash "$nogobin/bash"
  printf '#!/usr/bin/env bash\nexec /usr/bin/dirname "$@"\n' >"$nogobin/dirname"
  printf '#!/usr/bin/env bash\nexec /usr/bin/pwd "$@"\n' >"$nogobin/pwd"
  printf '#!/usr/bin/env bash\nexec /usr/bin/cat "$@"\n' >"$nogobin/cat"
  chmod +x "$nogobin/dirname" "$nogobin/pwd" "$nogobin/cat"
  TEST_ROOT="$root" PATH="$nogobin" "$root/plugin/scripts/ensure-runtime.sh"
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

test_existing_runtime_is_reused_when_go_is_unavailable() {
  local root expected output
  root="$(new_fixture)"
  expected="$root/plugin/.bin/tmux-session-sidebar"
  mkdir -p "$root/plugin/.bin"
  cat >"$expected" <<'RUNTIME'
#!/usr/bin/env bash
if [ "${1:-}" = version ]; then
  echo "tmux-session-sidebar cached"
else
  echo cached-runtime
fi
RUNTIME
  chmod +x "$expected"
  printf 'release:bnema/tmux-session-sidebar:latest\n' >"$root/plugin/.bin/.build-fingerprint"

  output="$(run_ensure_without_go "$root")"
  assert_eq "$expected" "$output" "existing plugin-local runtime should be returned when go is unavailable"
}

real_command() {
  command -v "$1" 2>/dev/null || printf '/usr/bin/%s\n' "$1"
}

write_exec_wrapper() {
  local path="$1" target="$2"
  cat >"$path" <<WRAPPER
#!/usr/bin/env bash
exec $target "\$@"
WRAPPER
  chmod +x "$path"
}

prepare_release_download_fixture() {
  local root="$1" bash_bin checksum chmod_bin cp_bin dirname_bin gzip_bin mkdir_bin mv_bin pwd_bin rm_bin sha_bin shasum_bin tar_bin
  bash_bin="$(real_command bash)"
  chmod_bin="$(real_command chmod)"
  cp_bin="$(real_command cp)"
  dirname_bin="$(real_command dirname)"
  gzip_bin="$(real_command gzip)"
  mkdir_bin="$(real_command mkdir)"
  mv_bin="$(real_command mv)"
  pwd_bin="$(real_command pwd)"
  rm_bin="$(real_command rm)"
  sha_bin="$(real_command sha256sum)"
  shasum_bin="$(real_command shasum)"
  tar_bin="$(real_command tar)"

  rm -f "$root/fakebin/go"
  mkdir -p "$root/fakebin" "$root/download-src"
  cat >"$root/download-src/tmux-session-sidebar" <<'RUNTIME'
#!/usr/bin/env bash
if [ "${1:-}" = version ]; then
  echo "tmux-session-sidebar downloaded"
else
  echo downloaded-runtime
fi
RUNTIME
  chmod +x "$root/download-src/tmux-session-sidebar"
  tar -C "$root/download-src" -czf "$root/release.tar.gz" tmux-session-sidebar
  if command -v sha256sum >/dev/null 2>&1; then
    checksum="$(sha256sum "$root/release.tar.gz")"
  else
    checksum="$(shasum -a 256 "$root/release.tar.gz")"
  fi
  printf '%s  tmux-session-sidebar_Linux_x86_64.tar.gz\n' "${checksum%% *}" >"$root/checksums.txt"
  ln -s "$bash_bin" "$root/fakebin/bash"
  write_exec_wrapper "$root/fakebin/dirname" "$dirname_bin"
  write_exec_wrapper "$root/fakebin/pwd" "$pwd_bin"
  cat >"$root/fakebin/uname" <<'UNAMEFAKE'
#!/usr/bin/env bash
case "${1:-}" in
  -s) printf 'Linux\n' ;;
  -m) printf 'x86_64\n' ;;
  *) printf 'Linux\n' ;;
esac
UNAMEFAKE
  chmod +x "$root/fakebin/uname"
  write_exec_wrapper "$root/fakebin/tar" "$tar_bin"
  write_exec_wrapper "$root/fakebin/rm" "$rm_bin"
  if [ -x "$sha_bin" ]; then
    write_exec_wrapper "$root/fakebin/sha256sum" "$sha_bin"
  fi
  if [ -x "$shasum_bin" ]; then
    write_exec_wrapper "$root/fakebin/shasum" "$shasum_bin"
  fi
  write_exec_wrapper "$root/fakebin/mkdir" "$mkdir_bin"
  write_exec_wrapper "$root/fakebin/mv" "$mv_bin"
  write_exec_wrapper "$root/fakebin/chmod" "$chmod_bin"
  write_exec_wrapper "$root/fakebin/cp" "$cp_bin"
  ln -s "$gzip_bin" "$root/fakebin/gzip"
  cat >"$root/fakebin/curl" <<'CURLFAKE'
#!/usr/bin/env bash
set -euo pipefail
out=""
url=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) shift; out="$1" ;;
    http*) url="$1" ;;
  esac
  shift || true
done
[ -n "$out" ] || { echo "missing output" >&2; exit 1; }
printf '%s\n' "$url" >>"$TEST_ROOT/curl.log"
case "$url" in
  */checksums.txt) cp "$TEST_ROOT/checksums.txt" "$out" ;;
  *) cp "$TEST_ROOT/release.tar.gz" "$out" ;;
esac
CURLFAKE
  chmod +x "$root/fakebin/curl"
}

test_downloads_latest_release_when_go_is_unavailable_and_runtime_missing() {
  local root expected output
  root="$(new_fixture)"
  expected="$root/plugin/.bin/tmux-session-sidebar"
  rm -rf "$root/plugin/.bin"
  prepare_release_download_fixture "$root"

  output="$(TEST_ROOT="$root" PATH="$root/fakebin" "$root/plugin/scripts/ensure-runtime.sh")"
  assert_eq "$expected" "$output" "downloaded release runtime should be installed when go is unavailable"
  assert_file_contains "$root/curl.log" "https://github.com/bnema/tmux-session-sidebar/releases/latest/download/tmux-session-sidebar_Linux_x86_64.tar.gz" "ensure-runtime should request the Linux x86_64 latest release asset"
  assert_file_contains "$root/curl.log" "https://github.com/bnema/tmux-session-sidebar/releases/latest/download/checksums.txt" "ensure-runtime should request release checksums"
  assert_eq "downloaded-runtime" "$($expected)" "downloaded runtime should be executable"
}

test_refreshes_existing_release_runtime_when_requested() {
  local root expected output
  root="$(new_fixture)"
  expected="$root/plugin/.bin/tmux-session-sidebar"
  mkdir -p "$root/plugin/.bin"
  cat >"$expected" <<'RUNTIME'
#!/usr/bin/env bash
if [ "${1:-}" = version ]; then
  echo stale-runtime
else
  echo stale-runtime
fi
RUNTIME
  chmod +x "$expected"
  prepare_release_download_fixture "$root"

  output="$(TEST_ROOT="$root" PATH="$root/fakebin" TMUX_SESSION_SIDEBAR_REFRESH_RELEASE=1 "$root/plugin/scripts/ensure-runtime.sh")"

  assert_eq "$expected" "$output" "forced refresh should still return plugin-local runtime"
  assert_file_contains "$root/curl.log" "tmux-session-sidebar_Linux_x86_64.tar.gz" "forced refresh should download the release asset"
  assert_eq "downloaded-runtime" "$($expected)" "forced refresh should replace stale runtime"
}

test_matching_release_stamp_refreshes_invalid_cached_runtime() {
  local root expected output
  root="$(new_fixture)"
  expected="$root/plugin/.bin/tmux-session-sidebar"
  mkdir -p "$root/plugin/.bin"
  cat >"$expected" <<'RUNTIME'
#!/usr/bin/env bash
echo invalid-runtime
RUNTIME
  chmod +x "$expected"
  printf 'release:bnema/tmux-session-sidebar:latest\n' >"$root/plugin/.bin/.build-fingerprint"
  prepare_release_download_fixture "$root"

  output="$(TEST_ROOT="$root" PATH="$root/fakebin" "$root/plugin/scripts/ensure-runtime.sh")"

  assert_eq "$expected" "$output" "invalid cached runtime should be refreshed even when release stamp matches"
  assert_file_contains "$root/curl.log" "tmux-session-sidebar_Linux_x86_64.tar.gz" "invalid cached runtime should trigger release download"
  assert_eq "downloaded-runtime" "$($expected)" "invalid cached runtime should be replaced with validated release runtime"
}

test_rejects_downloaded_runtime_without_version_output() {
  local root expected output status
  root="$(new_fixture)"
  expected="$root/plugin/.bin/tmux-session-sidebar"
  rm -rf "$root/plugin/.bin"
  prepare_release_download_fixture "$root"
  printf '#!/usr/bin/env bash\necho invalid-runtime\n' >"$root/download-src/tmux-session-sidebar"
  chmod +x "$root/download-src/tmux-session-sidebar"
  tar -C "$root/download-src" -czf "$root/release.tar.gz" tmux-session-sidebar
  if command -v sha256sum >/dev/null 2>&1; then
    checksum="$(sha256sum "$root/release.tar.gz")"
  else
    checksum="$(shasum -a 256 "$root/release.tar.gz")"
  fi
  printf '%s  tmux-session-sidebar_Linux_x86_64.tar.gz\n' "${checksum%% *}" >"$root/checksums.txt"

  set +e
  output="$(TEST_ROOT="$root" PATH="$root/fakebin" "$root/plugin/scripts/ensure-runtime.sh" 2>&1)"
  status="$?"
  set -e

  [ "$status" -ne 0 ] || fail "invalid downloaded runtime should fail installation"
  case "$output" in
    *"runtime validation failed"*) ;;
    *) fail "invalid downloaded runtime should report validation failure, got: $output" ;;
  esac
  [ ! -e "$expected" ] || fail "invalid downloaded runtime should not be installed"
}

test_rejects_go_build_runtime_without_version_output() {
  local root expected output status
  root="$(new_fixture)"
  expected="$root/plugin/.bin/tmux-session-sidebar"

  set +e
  output="$(TEST_ROOT="$root" TEST_INVALID_GO_BUILD=1 PATH="$root/fakebin:$PATH" "$root/plugin/scripts/ensure-runtime.sh" 2>&1)"
  status="$?"
  set -e

  [ "$status" -ne 0 ] || fail "invalid built runtime should fail installation"
  case "$output" in
    *"valid output (expected 'tmux-session-sidebar <version>')"*) ;;
    *) fail "invalid built runtime should report validation failure, got: $output" ;;
  esac
  [ -x "$expected" ] || fail "go build should have produced the runtime before validation failed"
  [ ! -e "$root/plugin/.bin/.build-fingerprint" ] || fail "invalid built runtime should not be stamped"
}

test_rejects_downloaded_runtime_with_checksum_mismatch() {
  local root expected output status
  root="$(new_fixture)"
  expected="$root/plugin/.bin/tmux-session-sidebar"
  rm -rf "$root/plugin/.bin"
  prepare_release_download_fixture "$root"
  printf '0000000000000000000000000000000000000000000000000000000000000000  tmux-session-sidebar_Linux_x86_64.tar.gz\n' >"$root/checksums.txt"

  set +e
  output="$(TEST_ROOT="$root" PATH="$root/fakebin" "$root/plugin/scripts/ensure-runtime.sh" 2>&1)"
  status="$?"
  set -e

  [ "$status" -ne 0 ] || fail "checksum mismatch should fail installation"
  case "$output" in
    *"checksum mismatch"*) ;;
    *) fail "checksum mismatch should be reported, got: $output" ;;
  esac
  [ ! -e "$expected" ] || fail "checksum mismatch should not install runtime"
}

test_rejects_downloaded_runtime_without_checksum_entry() {
  local root expected output status
  root="$(new_fixture)"
  expected="$root/plugin/.bin/tmux-session-sidebar"
  rm -rf "$root/plugin/.bin"
  prepare_release_download_fixture "$root"
  printf '0000000000000000000000000000000000000000000000000000000000000000  other.tar.gz\n' >"$root/checksums.txt"

  set +e
  output="$(TEST_ROOT="$root" PATH="$root/fakebin" "$root/plugin/scripts/ensure-runtime.sh" 2>&1)"
  status="$?"
  set -e

  [ "$status" -ne 0 ] || fail "missing checksum entry should fail installation"
  case "$output" in
    *"checksum not found"*) ;;
    *) fail "missing checksum entry should be reported, got: $output" ;;
  esac
  [ ! -e "$expected" ] || fail "missing checksum entry should not install runtime"
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
test_existing_runtime_is_reused_when_go_is_unavailable
test_downloads_latest_release_when_go_is_unavailable_and_runtime_missing
test_refreshes_existing_release_runtime_when_requested
test_matching_release_stamp_refreshes_invalid_cached_runtime
test_rejects_downloaded_runtime_without_version_output
test_rejects_go_build_runtime_without_version_output
test_rejects_downloaded_runtime_with_checksum_mismatch
test_rejects_downloaded_runtime_without_checksum_entry

echo "ensure-runtime tests passed"
