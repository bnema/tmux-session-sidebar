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

assert_file_not_contains() {
  local file="$1" needle="$2" message="$3"
  if [ -e "$file" ] && grep -Fq -- "$needle" "$file"; then
    echo "--- $file ---" >&2
    cat "$file" >&2 || true
    fail "$message: unexpected '$needle'"
  fi
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

new_fixture() {
  local root plugin fakebin
  root="$(mktemp -d)"
  plugin="$root/plugin"
  fakebin="$root/fakebin"
  mkdir -p "$plugin/scripts" "$plugin/.bin" "$fakebin" "$root/statehome"
  cp "$(dirname "$0")/update-runtime.sh" "$plugin/scripts/update-runtime.sh"
  chmod +x "$plugin/scripts/update-runtime.sh"
  cat >"$plugin/.bin/tmux-session-sidebar" <<'STALE'
#!/usr/bin/env bash
if [ "${1:-}" = version ]; then
  echo "tmux-session-sidebar v1.0.0"
else
  echo stale-runtime
fi
STALE
  chmod +x "$plugin/.bin/tmux-session-sidebar"
  printf 'release:bnema/tmux-session-sidebar:latest\n' >"$plugin/.bin/.build-fingerprint"
  printf '%s\n' "$root"
}

prepare_fake_tools() {
  local root="$1" fakebin="$1/fakebin" checksum
  local bash_bin chmod_bin cp_bin dirname_bin gzip_bin mkdir_bin mv_bin pwd_bin rm_bin sha_bin shasum_bin tar_bin
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

  ln -sf "$bash_bin" "$fakebin/bash"
  write_exec_wrapper "$fakebin/dirname" "$dirname_bin"
  write_exec_wrapper "$fakebin/pwd" "$pwd_bin"
  write_exec_wrapper "$fakebin/tar" "$tar_bin"
  write_exec_wrapper "$fakebin/rm" "$rm_bin"
  write_exec_wrapper "$fakebin/mkdir" "$mkdir_bin"
  write_exec_wrapper "$fakebin/mv" "$mv_bin"
  write_exec_wrapper "$fakebin/chmod" "$chmod_bin"
  write_exec_wrapper "$fakebin/cp" "$cp_bin"
  if [ -x "$sha_bin" ]; then
    write_exec_wrapper "$fakebin/sha256sum" "$sha_bin"
  fi
  if [ -x "$shasum_bin" ]; then
    write_exec_wrapper "$fakebin/shasum" "$shasum_bin"
  fi
  ln -sf "$gzip_bin" "$fakebin/gzip"

  cat >"$fakebin/uname" <<'UNAMEFAKE'
#!/usr/bin/env bash
case "${1:-}" in
  -s) printf 'Linux\n' ;;
  -m) printf 'x86_64\n' ;;
  *) printf 'Linux\n' ;;
esac
UNAMEFAKE
  chmod +x "$fakebin/uname"

  cat >"$fakebin/tmux" <<'TMUXFAKE'
#!/usr/bin/env bash
printf 'tmux %s\n' "$*" >>"$TEST_ROOT/tmux.log"
if [ "${TEST_TMUX_FAIL_SOURCE:-}" = 1 ] && [ "${1:-}" = source-file ]; then
  exit 42
fi
TMUXFAKE
  chmod +x "$fakebin/tmux"

  cat >"$fakebin/pkill" <<'PKILLFAKE'
#!/usr/bin/env bash
printf 'pkill %s\n' "$*" >>"$TEST_ROOT/pkill.log"
exit 1
PKILLFAKE
  chmod +x "$fakebin/pkill"

  mkdir -p "$root/download-src"
  cat >"$root/download-src/tmux-session-sidebar" <<'RUNTIME'
#!/usr/bin/env bash
if [ "${1:-}" = version ]; then
  echo "tmux-session-sidebar v2.0.0"
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
  # Generate test signing keypair and sign checksums.txt for signature verification tests
  if command -v openssl >/dev/null 2>&1; then
    openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$root/sign-key.pem" 2>/dev/null
    openssl pkey -in "$root/sign-key.pem" -pubout -out "$root/plugin/scripts/update-runtime.pub.pem" 2>/dev/null
    openssl dgst -sha256 -sign "$root/sign-key.pem" -out "$root/checksums.txt.sig" "$root/checksums.txt" 2>/dev/null
  fi

  cat >"$fakebin/curl" <<'CURLFAKE'
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
  */checksums.txt.sig) cp "$TEST_ROOT/checksums.txt.sig" "$out" ;;
  */checksums.txt) cp "$TEST_ROOT/checksums.txt" "$out" ;;
  *) cp "$TEST_ROOT/release.tar.gz" "$out" ;;
esac
CURLFAKE
  chmod +x "$fakebin/curl"
}

run_update() {
  local root="$1"
  shift
  TEST_ROOT="$root" XDG_STATE_HOME="$root/statehome" TMUX_CONF="$root/tmux.conf" PATH="$root/fakebin:$PATH" "$root/plugin/scripts/update-runtime.sh" "$@"
}

run_download_release_candidate_probe() {
  local root="$1" verify_exit="$2" check_scope="$3"
  TEST_ROOT="$root" UPDATE_RUNTIME_PATH="$root/plugin/scripts/update-runtime.sh" VERIFY_EXIT="$verify_exit" CHECK_SCOPE="$check_scope" bash <<'EOF'
set -euo pipefail

temp_script="$TEST_ROOT/sourceable-update-runtime.sh"
sed '$d' "$UPDATE_RUNTIME_PATH" >"$temp_script"
# shellcheck source=/dev/null
source "$temp_script"

BIN_DIR="$TEST_ROOT/probe-bin"
RELEASE_REPO="bnema/tmux-session-sidebar"
mkdir -p "$BIN_DIR"
cat >"$TEST_ROOT/fake-curl-probe" <<'CURL'
#!/usr/bin/env bash
set -euo pipefail
out=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) shift; out="$1" ;;
  esac
  shift || true
done
: >"$out"
CURL
chmod +x "$TEST_ROOT/fake-curl-probe"
CURL_BIN="$TEST_ROOT/fake-curl-probe"
release_os() { printf 'Linux'; }
release_arch() { printf 'x86_64'; }
log_update() { :; }
verify_release_signature() { return "$VERIFY_EXIT"; }

if [ "$CHECK_SCOPE" = 1 ]; then
  unset -v checksums_sig_url checksums_sig || true
fi

set +e
download_release_candidate "$BIN_DIR/candidate" >/dev/null
status="$?"
set -e

if [ "$CHECK_SCOPE" = 1 ]; then
  [ "${checksums_sig_url+x}" != x ] || exit 10
  [ "${checksums_sig+x}" != x ] || exit 11
fi

exit "$status"
EOF
}

test_manual_update_refreshes_release_replaces_runtime_and_restarts_tmux() {
  local root expected
  root="$(new_fixture)"
  prepare_fake_tools "$root"
  expected="$root/plugin/.bin/tmux-session-sidebar"

  run_update "$root"

  assert_eq "downloaded-runtime" "$($expected)" "manual update should install downloaded runtime"
  assert_file_contains "$root/plugin/.bin/.build-fingerprint" "release-version:v2.0.0" "stamp should record installed release version"
  assert_file_contains "$root/plugin/.bin/.build-fingerprint" "asset-sha256:" "stamp should record release asset checksum"
  assert_file_contains "$root/tmux.log" "tmux source-file $root/tmux.conf" "manual update should restart tmux from configured tmux.conf"
}

test_invalid_candidate_preserves_existing_runtime_and_does_not_restart() {
  local root expected output status
  root="$(new_fixture)"
  prepare_fake_tools "$root"
  expected="$root/plugin/.bin/tmux-session-sidebar"
  cat >"$root/download-src/tmux-session-sidebar" <<'INVALID'
#!/usr/bin/env bash
echo invalid-runtime
INVALID
  chmod +x "$root/download-src/tmux-session-sidebar"
  tar -C "$root/download-src" -czf "$root/release.tar.gz" tmux-session-sidebar
  if command -v sha256sum >/dev/null 2>&1; then
    checksum="$(sha256sum "$root/release.tar.gz")"
  else
    checksum="$(shasum -a 256 "$root/release.tar.gz")"
  fi
  printf '%s  tmux-session-sidebar_Linux_x86_64.tar.gz\n' "${checksum%% *}" >"$root/checksums.txt"
  # Re-sign the modified checksums.txt so signature verification still passes
  if [ -f "$root/sign-key.pem" ]; then
    openssl dgst -sha256 -sign "$root/sign-key.pem" -out "$root/checksums.txt.sig" "$root/checksums.txt" 2>/dev/null
  fi
  cat >"$root/fakebin/go" <<'GOFAKE'
#!/usr/bin/env bash
set -euo pipefail
printf 'go %s\n' "$*" >>"$TEST_ROOT/go.log"
case "${1:-}" in
  version) printf 'go version go1.99.0 test/test\n' ;;
  build)
    out=""
    while [ "$#" -gt 0 ]; do
      if [ "$1" = -o ]; then shift; out="$1"; fi
      shift || true
    done
    [ -n "$out" ] || exit 1
    cat >"$out" <<'RUNTIME'
#!/usr/bin/env bash
if [ "${1:-}" = version ]; then
  echo "tmux-session-sidebar source-fallback"
else
  echo source-fallback
fi
RUNTIME
    chmod +x "$out"
    ;;
  *) exit 1 ;;
esac
GOFAKE
  chmod +x "$root/fakebin/go"

  set +e
  output="$(run_update "$root" 2>&1)"
  status="$?"
  set -e

  [ "$status" -ne 0 ] || fail "invalid candidate should fail update"
  case "$output" in
    *"runtime validation failed"*) ;;
    *) fail "invalid candidate should report validation failure, got: $output" ;;
  esac
  assert_eq "stale-runtime" "$($expected)" "invalid update should preserve existing runtime"
  [ ! -e "$root/go.log" ] || fail "invalid release artifact should not fall back to source build"
  assert_file_not_contains "$root/tmux.log" "source-file" "invalid update should not restart tmux"
}

test_release_only_update_does_not_fall_back_to_source_build() {
  local root expected output status
  root="$(new_fixture)"
  prepare_fake_tools "$root"
  expected="$root/plugin/.bin/tmux-session-sidebar"
  cat >"$root/fakebin/curl" <<'CURLFAIL'
#!/usr/bin/env bash
exit 22
CURLFAIL
  chmod +x "$root/fakebin/curl"
  cat >"$root/fakebin/go" <<'GOFAKE'
#!/usr/bin/env bash
printf 'go should not run\n' >>"$TEST_ROOT/go.log"
exit 1
GOFAKE
  chmod +x "$root/fakebin/go"

  set +e
  output="$(TMUX_SESSION_SIDEBAR_RELEASE_ONLY=1 run_update "$root" 2>&1)"
  status="$?"
  set -e

  [ "$status" -ne 0 ] || fail "release-only update should fail when release download fails"
  case "$output" in
    *"release update failed; existing runtime left untouched"*) ;;
    *) fail "release-only failure should report no source fallback, got: $output" ;;
  esac
  assert_eq "stale-runtime" "$("$expected")" "release-only download failure should preserve existing runtime"
  [ ! -e "$root/go.log" ] || fail "release-only update should not fall back to source build"
}

test_restart_failure_restores_previous_runtime_and_stamp() {
  local root expected output source_attempts status
  root="$(new_fixture)"
  prepare_fake_tools "$root"
  expected="$root/plugin/.bin/tmux-session-sidebar"

  set +e
  output="$(TEST_TMUX_FAIL_SOURCE=1 run_update "$root" 2>&1)"
  status="$?"
  set -e

  [ "$status" -ne 0 ] || fail "tmux source failure should fail update"
  case "$output" in
    *"restart failed after runtime update"*) ;;
    *) fail "restart failure should report rollback, got: $output" ;;
  esac
  assert_eq "stale-runtime" "$($expected)" "restart failure should restore previous runtime"
  assert_file_contains "$root/plugin/.bin/.build-fingerprint" "release:bnema/tmux-session-sidebar:latest" "restart failure should restore previous stamp"
  source_attempts="$(grep -c 'tmux source-file' "$root/tmux.log" || true)"
  assert_eq "2" "$source_attempts" "restart failure should try to source tmux once for new runtime and once after rollback"
}

test_make_update_runtime_target_uses_central_update_path() {
  assert_file_contains "$(dirname "$0")/../Makefile" "update-runtime:" "Makefile should expose a manual update-runtime target"
  assert_file_contains "$(dirname "$0")/../Makefile" "@bash scripts/update-runtime.sh" "manual update-runtime target should use the central updater"
  assert_file_contains "$(dirname "$0")/../Makefile" "@TMUX_SESSION_SIDEBAR_BUILD_FROM_SOURCE=1 bash scripts/update-runtime.sh" "restart-runtime target should use the central one-shot source updater"
}

test_manual_update_refreshes_release_replaces_runtime_and_restarts_tmux
test_invalid_candidate_preserves_existing_runtime_and_does_not_restart
test_release_only_update_does_not_fall_back_to_source_build
test_restart_failure_restores_previous_runtime_and_stamp
test_make_update_runtime_target_uses_central_update_path

# ---------------------------------------------------------------------------
# Signature verification tests
# ---------------------------------------------------------------------------

test_signature_verification_passes_for_valid_release() {
  local root expected
  root="$(new_fixture)"
  prepare_fake_tools "$root"
  expected="$root/plugin/.bin/tmux-session-sidebar"

  [ -f "$root/plugin/scripts/update-runtime.pub.pem" ] || fail "public key not generated by prepare_fake_tools"
  [ -f "$root/checksums.txt.sig" ] || fail "signature not generated by prepare_fake_tools"

  run_update "$root"

  assert_eq "downloaded-runtime" "$($expected)" "valid signature should allow release install"
  assert_file_contains "$root/plugin/.bin/.build-fingerprint" "release-version:v2.0.0" "stamp should record release version"
}

test_missing_signature_asset_fails_install() {
  local root expected output status
  root="$(new_fixture)"
  prepare_fake_tools "$root"
  expected="$root/plugin/.bin/tmux-session-sidebar"
  # Remove signature so curl cannot serve it
  rm -f "$root/checksums.txt.sig"
  # Remove go so there is no fallback
  rm -f "$root/fakebin/go"

  set +e
  output="$(run_update "$root" 2>&1)"
  status="$?"
  set -e

  [ "$status" -ne 0 ] || fail "missing signature should fail update"
  case "$output" in
    *"failed to download signature"*|*"release artifact failed validation"*) ;;
    *) fail "missing signature should report download failure, got: $output" ;;
  esac
  assert_eq "stale-runtime" "$("$expected")" "missing signature should preserve existing runtime"
}

test_bad_signature_fails_install() {
  local root expected output status
  root="$(new_fixture)"
  prepare_fake_tools "$root"
  expected="$root/plugin/.bin/tmux-session-sidebar"
  # Corrupt the signature so verification fails
  printf 'BAD_SIGNATURE' >"$root/checksums.txt.sig"
  # Remove go so there is no fallback
  rm -f "$root/fakebin/go"

  set +e
  output="$(run_update "$root" 2>&1)"
  status="$?"
  set -e

  [ "$status" -ne 0 ] || fail "bad signature should fail update"
  case "$output" in
    *"signature verification failed"*) ;;
    *) fail "bad signature should report verification failure, got: $output" ;;
  esac
  assert_eq "stale-runtime" "$("$expected")" "bad signature should preserve existing runtime"
}

test_release_only_mode_fails_on_missing_signature() {
  local root output status
  root="$(new_fixture)"
  prepare_fake_tools "$root"
  # Remove signature so curl cannot serve it
  rm -f "$root/checksums.txt.sig"

  set +e
  output="$(TMUX_SESSION_SIDEBAR_RELEASE_ONLY=1 run_update "$root" 2>&1)"
  status="$?"
  set -e

  [ "$status" -ne 0 ] || fail "release-only mode should fail on missing signature"
  case "$output" in
    *"release artifact failed validation"*) ;;
    *) fail "release-only mode should report no source fallback, got: $output" ;;
  esac
}

test_download_release_candidate_preserves_missing_openssl_exit_code() {
  local root status
  root="$(new_fixture)"

  set +e
  run_download_release_candidate_probe "$root" 3 0
  status="$?"
  set -e

  assert_eq "3" "$status" "download_release_candidate should preserve verify_release_signature exit code 3"
}

test_download_release_candidate_scopes_signature_paths_locally() {
  local root status
  root="$(new_fixture)"

  set +e
  run_download_release_candidate_probe "$root" 1 1
  status="$?"
  set -e

  case "$status" in
    2) ;;
    10) fail "download_release_candidate should not leak checksums_sig_url into global scope" ;;
    11) fail "download_release_candidate should not leak checksums_sig into global scope" ;;
    *) fail "unexpected probe status: $status" ;;
  esac
}

test_stop_only_without_ps_uses_any_path_pkill_patterns() {
  local root
  root="$(new_fixture)"
  prepare_fake_tools "$root"
  rm -f "$root/fakebin/ps"

  PATH="$root/fakebin" run_update "$root" --stop-only

  assert_file_contains "$root/pkill.log" "pkill -f /tmux-session-sidebar daemon serve-ui" "no-ps stop should pkill stale serve-ui runtimes by any path"
  assert_file_contains "$root/pkill.log" "pkill -f /tmux-session-sidebar daemon bootstrap" "no-ps stop should pkill stale bootstrap runtimes by any path"
  assert_file_contains "$root/pkill.log" "pkill -f /tmux-session-sidebar daemon serve" "no-ps stop should pkill stale serve runtimes by any path"
}

test_signature_verification_passes_for_valid_release
test_missing_signature_asset_fails_install
test_bad_signature_fails_install
test_release_only_mode_fails_on_missing_signature
test_download_release_candidate_preserves_missing_openssl_exit_code
test_download_release_candidate_scopes_signature_paths_locally
test_stop_only_without_ps_uses_any_path_pkill_patterns

echo "update-runtime tests passed"
