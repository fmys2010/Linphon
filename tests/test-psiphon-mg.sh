#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
MANAGER_SCRIPT="$REPO_ROOT/scripts/psiphon-mg.sh"
GO_BINARY_DEFAULT="$REPO_ROOT/tools/psiphon-mg/bin/psiphon-mg"
FAKE_BINARY="$REPO_ROOT/tests/fake-psiphon-tunnel-core-x86_64"
BASE_CONFIG="$REPO_ROOT/psiphon.config"
TEST_ROOT="$REPO_ROOT/.work/test-psiphon-mg"
RUNTIME_ROOT="$TEST_ROOT/runtime"

assert_eq() {
  local expected=$1
  local actual=$2
  local message=$3

  if [ "$expected" != "$actual" ]; then
    printf '[test] assertion failed: %s (expected=%s actual=%s)\n' "$message" "$expected" "$actual" >&2
    exit 1
  fi
}

assert_ne() {
  local left=$1
  local right=$2
  local message=$3

  if [ "$left" = "$right" ]; then
    printf '[test] assertion failed: %s (both=%s)\n' "$message" "$left" >&2
    exit 1
  fi
}

assert_file() {
  local path=$1

  if [ ! -f "$path" ]; then
    printf '[test] missing file: %s\n' "$path" >&2
    exit 1
  fi
}

read_status_value() {
  local status_text=$1
  local key=$2

  printf '%s\n' "$status_text" | awk -F= -v key="$key" '$1 == key { print substr($0, length($1) + 2) }'
}

cleanup() {
  bash "$MANAGER_SCRIPT" stop \
    --runtime-root "$RUNTIME_ROOT" \
    --binary "$FAKE_BINARY" \
    --base-config "$BASE_CONFIG" >/dev/null 2>&1 || true
}

ensure_go_binary() {
  if [ -n "${PSIPHON_MG_GO_BINARY:-}" ] && [ -x "${PSIPHON_MG_GO_BINARY}" ]; then
    return 0
  fi

  if [ -x "$GO_BINARY_DEFAULT" ]; then
    return 0
  fi

  if command -v go >/dev/null 2>&1; then
    mkdir -p "$(dirname -- "$GO_BINARY_DEFAULT")"
    (
      cd "$REPO_ROOT/tools/psiphon-mg"
      go build -o ../../tools/psiphon-mg/bin/psiphon-mg ./cmd/psiphon-mg
    )
    chmod +x "$GO_BINARY_DEFAULT" 2>/dev/null || true
    return 0
  fi

  printf '[test] missing Go manager binary: %s\n' "$GO_BINARY_DEFAULT" >&2
  printf '[test] build it first or set PSIPHON_MG_GO_BINARY to an executable path\n' >&2
  exit 1
}

trap cleanup EXIT

rm -rf "$TEST_ROOT"
mkdir -p "$TEST_ROOT"

ensure_go_binary

printf '[test] verifying stopped state\n'
status_output=$(bash "$MANAGER_SCRIPT" status \
  --runtime-root "$RUNTIME_ROOT")
assert_eq 'stopped' "$(read_status_value "$status_output" state)" 'initial state'
assert_eq 'no' "$(read_status_value "$status_output" running)" 'initial running flag'

if bash "$MANAGER_SCRIPT" current-region --runtime-root "$RUNTIME_ROOT" >/dev/null 2>&1; then
  printf '[test] current-region unexpectedly succeeded while stopped\n' >&2
  exit 1
fi

printf '[test] verifying stale lock recovery\n'
STALE_LOCK_ROOT="$TEST_ROOT/stale-lock"
mkdir -p "$STALE_LOCK_ROOT/lock"
printf '999999\n' > "$STALE_LOCK_ROOT/lock/pid"
printf '%s\n' "$MANAGER_SCRIPT" > "$STALE_LOCK_ROOT/lock/owner"

status_output=$(bash "$MANAGER_SCRIPT" status \
  --runtime-root "$STALE_LOCK_ROOT")
assert_eq 'stopped' "$(read_status_value "$status_output" state)" 'state after stale lock recovery'

if [ -d "$STALE_LOCK_ROOT/lock" ]; then
  printf '[test] stale lock directory unexpectedly remained after recovery\n' >&2
  exit 1
fi

printf '[test] starting US region\n'
bash "$MANAGER_SCRIPT" start US \
  --runtime-root "$RUNTIME_ROOT" \
  --binary "$FAKE_BINARY" \
  --base-config "$BASE_CONFIG" \
  --ready-timeout-seconds 5

sleep 1

assert_eq 'US' "$(bash "$MANAGER_SCRIPT" current-region --runtime-root "$RUNTIME_ROOT")" 'current region after start'

status_output=$(bash "$MANAGER_SCRIPT" status \
  --runtime-root "$RUNTIME_ROOT")
assert_eq 'running' "$(read_status_value "$status_output" state)" 'state after start'
assert_eq 'US' "$(read_status_value "$status_output" region)" 'region after start'
assert_eq '8081' "$(read_status_value "$status_output" http_port)" 'http port after start'
assert_eq '1081' "$(read_status_value "$status_output" socks_port)" 'socks port after start'
assert_eq 'yes' "$(read_status_value "$status_output" http_notice)" 'http notice after start'
assert_eq 'yes' "$(read_status_value "$status_output" socks_notice)" 'socks notice after start'
assert_eq 'yes' "$(read_status_value "$status_output" tunnels_notice)" 'tunnels notice after start'

first_pid=$(read_status_value "$status_output" pid)
first_notices=$(read_status_value "$status_output" notices_path)
assert_file "$first_notices"

printf '[test] verifying start rejects another live region\n'
if bash "$MANAGER_SCRIPT" start CA \
  --runtime-root "$RUNTIME_ROOT" \
  --binary "$FAKE_BINARY" \
  --base-config "$BASE_CONFIG" >/dev/null 2>&1; then
  printf '[test] start unexpectedly succeeded while another region was active\n' >&2
  exit 1
fi

printf '[test] verifying same-region switch is a no-op\n'
bash "$MANAGER_SCRIPT" switch US \
  --runtime-root "$RUNTIME_ROOT"

status_output=$(bash "$MANAGER_SCRIPT" status \
  --runtime-root "$RUNTIME_ROOT")
assert_eq "$first_pid" "$(read_status_value "$status_output" pid)" 'pid preserved on same-region switch'
assert_eq "$first_notices" "$(read_status_value "$status_output" notices_path)" 'notices path preserved on same-region switch'

printf '[test] verifying latest disconnected tunnel state forces a restart\n'
printf '%s\n' '{"noticeType":"Tunnels","data":{"count":0}}' >> "$first_notices"

status_output=$(bash "$MANAGER_SCRIPT" status \
  --runtime-root "$RUNTIME_ROOT")
assert_eq 'no' "$(read_status_value "$status_output" tunnels_notice)" 'latest tunnel notice after disconnect'

bash "$MANAGER_SCRIPT" switch US \
  --runtime-root "$RUNTIME_ROOT"

sleep 1

status_output=$(bash "$MANAGER_SCRIPT" status \
  --runtime-root "$RUNTIME_ROOT")
assert_eq 'US' "$(read_status_value "$status_output" region)" 'region after disconnected same-region switch'
assert_eq 'yes' "$(read_status_value "$status_output" tunnels_notice)" 'tunnels notice after disconnected same-region switch'

refreshed_pid=$(read_status_value "$status_output" pid)
refreshed_notices=$(read_status_value "$status_output" notices_path)
assert_ne "$first_pid" "$refreshed_pid" 'pid refreshed after disconnected same-region switch'
assert_ne "$first_notices" "$refreshed_notices" 'notices path refreshed after disconnected same-region switch'

first_pid=$refreshed_pid
first_notices=$refreshed_notices

printf '[test] switching to CA region\n'
bash "$MANAGER_SCRIPT" switch CA \
  --runtime-root "$RUNTIME_ROOT"

sleep 1

assert_eq 'CA' "$(bash "$MANAGER_SCRIPT" current-region --runtime-root "$RUNTIME_ROOT")" 'current region after switch'
status_output=$(bash "$MANAGER_SCRIPT" status \
  --runtime-root "$RUNTIME_ROOT")
assert_eq 'running' "$(read_status_value "$status_output" state)" 'state after switch'
assert_eq 'CA' "$(read_status_value "$status_output" region)" 'region after switch'
assert_eq '8081' "$(read_status_value "$status_output" http_port)" 'http port after switch'
assert_eq '1081' "$(read_status_value "$status_output" socks_port)" 'socks port after switch'
assert_eq 'yes' "$(read_status_value "$status_output" tunnels_notice)" 'tunnels notice after switch'

second_pid=$(read_status_value "$status_output" pid)
second_notices=$(read_status_value "$status_output" notices_path)
assert_ne "$first_notices" "$second_notices" 'fresh notices path on region switch'
assert_ne "$first_pid" "$second_pid" 'new pid on region switch'

printf '[test] verifying stale-state detection\n'
kill "$second_pid"
sleep 1

status_output=$(bash "$MANAGER_SCRIPT" status \
  --runtime-root "$RUNTIME_ROOT")
assert_eq 'stale' "$(read_status_value "$status_output" state)" 'state after external kill'
assert_eq 'no' "$(read_status_value "$status_output" running)" 'running flag after external kill'

if bash "$MANAGER_SCRIPT" current-region --runtime-root "$RUNTIME_ROOT" >/dev/null 2>&1; then
  printf '[test] current-region unexpectedly succeeded for stale state\n' >&2
  exit 1
fi

printf '[test] recovering from stale state with a fresh start\n'
bash "$MANAGER_SCRIPT" start GB \
  --runtime-root "$RUNTIME_ROOT" \
  --binary "$FAKE_BINARY" \
  --base-config "$BASE_CONFIG" \
  --ready-timeout-seconds 5

sleep 1

assert_eq 'GB' "$(bash "$MANAGER_SCRIPT" current-region --runtime-root "$RUNTIME_ROOT")" 'current region after stale recovery'

printf '[test] stopping active region\n'
bash "$MANAGER_SCRIPT" stop --runtime-root "$RUNTIME_ROOT"
status_output=$(bash "$MANAGER_SCRIPT" status --runtime-root "$RUNTIME_ROOT")
assert_eq 'stopped' "$(read_status_value "$status_output" state)" 'state after stop'

printf '[test] verifying idempotent stop\n'
bash "$MANAGER_SCRIPT" stop --runtime-root "$RUNTIME_ROOT"
status_output=$(bash "$MANAGER_SCRIPT" status --runtime-root "$RUNTIME_ROOT")
assert_eq 'stopped' "$(read_status_value "$status_output" state)" 'state after repeated stop'

printf '[test] repo-local manager verification passed\n'
