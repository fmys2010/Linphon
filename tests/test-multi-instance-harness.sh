#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
HARNESS_SCRIPT="$REPO_ROOT/scripts/psiphon-multi-instance.sh"
STAGED_SCRIPT="$REPO_ROOT/scripts/run-psiphon-staged.sh"
FAKE_BINARY="$REPO_ROOT/tests/fake-psiphon-tunnel-core-x86_64"
TEST_ROOT="$REPO_ROOT/.work/test-harness-offline"
SINGLE_ROOT="$TEST_ROOT/single"
STAGED_ROOT="$TEST_ROOT/staged"

assert_file() {
  local path=$1
  if [ ! -f "$path" ]; then
    printf '[test] missing file: %s\n' "$path" >&2
    exit 1
  fi
}

assert_dir() {
  local path=$1
  if [ ! -d "$path" ]; then
    printf '[test] missing directory: %s\n' "$path" >&2
    exit 1
  fi
}

assert_eq() {
  local expected=$1
  local actual=$2
  local message=$3
  if [ "$expected" != "$actual" ]; then
    printf '[test] assertion failed: %s (expected=%s actual=%s)\n' "$message" "$expected" "$actual" >&2
    exit 1
  fi
}

rm -rf "$TEST_ROOT"
mkdir -p "$TEST_ROOT"

printf '[test] running single-stage offline harness smoke test\n'
bash "$HARNESS_SCRIPT" run \
  --binary "$FAKE_BINARY" \
  --base-config "$REPO_ROOT/psiphon.config" \
  --runtime-root "$SINGLE_ROOT" \
  --run-name smoke-3 \
  --count 3 \
  --http-port-base 19080 \
  --socks-port-base 12080 \
  --wait-seconds 1 \
  --startup-grace-seconds 1

SINGLE_RUN_DIR="$SINGLE_ROOT/runs/smoke-3"
SINGLE_SUMMARY="$SINGLE_RUN_DIR/summary.tsv"
SINGLE_METRICS="$SINGLE_RUN_DIR/metrics-final.tsv"
SINGLE_CGROUP_START="$SINGLE_RUN_DIR/cgroup-start.snapshot"
SINGLE_CGROUP_FINAL="$SINGLE_RUN_DIR/cgroup-final.snapshot"

assert_dir "$SINGLE_RUN_DIR"
assert_file "$SINGLE_SUMMARY"
assert_file "$SINGLE_METRICS"
assert_file "$SINGLE_CGROUP_START"
assert_file "$SINGLE_CGROUP_FINAL"
assert_eq 4 "$(wc -l < "$SINGLE_SUMMARY" | tr -d '[:space:]')" "single summary line count"
assert_eq 4 "$(wc -l < "$SINGLE_METRICS" | tr -d '[:space:]')" "single metrics line count"
assert_eq 3 "$(find "$SINGLE_RUN_DIR/instances" -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d '[:space:]')" "single instance directory count"
assert_eq 3 "$(grep -h 'RemoteServerListDownloadFilename' "$SINGLE_RUN_DIR"/instances/*/config.json | sort -u | wc -l | tr -d '[:space:]')" "unique remote list filenames"
assert_eq 3 "$(awk 'NR == 1 { next } { print $3 }' "$SINGLE_SUMMARY" | sort -u | wc -l | tr -d '[:space:]')" "single distinct region count"
assert_eq 'AT,BE,BG' "$(awk 'NR == 1 { next } { print $3 }' "$SINGLE_SUMMARY" | paste -sd, -)" "single default region order"

awk 'NR == 1 { next } $6 != "yes" || $7 != "yes" || $8 != "yes" || $9 != "yes" { exit 1 }' "$SINGLE_SUMMARY"
grep -q $'^memory.current\t\|^memory.usage_in_bytes\t' "$SINGLE_CGROUP_START"
grep -q $'^pids.current\t\|^pids.current.v1\t' "$SINGLE_CGROUP_FINAL"
grep -q '"EgressRegion":"AT"' "$SINGLE_RUN_DIR/instances/instance-001/config.json"
grep -q '"EgressRegion":"BE"' "$SINGLE_RUN_DIR/instances/instance-002/config.json"
grep -q '"EgressRegion":"BG"' "$SINGLE_RUN_DIR/instances/instance-003/config.json"

printf '[test] running staged offline harness test (3, 8, 28)\n'
bash "$STAGED_SCRIPT" \
  --binary "$FAKE_BINARY" \
  --base-config "$REPO_ROOT/psiphon.config" \
  --runtime-root "$STAGED_ROOT" \
  --wait-seconds 1 \
  --startup-grace-seconds 1

STAGED_RESULTS="$STAGED_ROOT/stage-results.tsv"
assert_file "$STAGED_RESULTS"
assert_eq 4 "$(wc -l < "$STAGED_RESULTS" | tr -d '[:space:]')" "stage results line count"

awk 'NR == 1 { next } $2 != 0 { exit 1 }' "$STAGED_RESULTS"

assert_eq 29 "$(wc -l < "$STAGED_ROOT/runs/stage-28/summary.tsv" | tr -d '[:space:]')" "stage-28 summary line count"
assert_eq 29 "$(wc -l < "$STAGED_ROOT/runs/stage-28/metrics-final.tsv" | tr -d '[:space:]')" "stage-28 metrics line count"
assert_file "$STAGED_ROOT/runs/stage-28/cgroup-start.snapshot"
assert_file "$STAGED_ROOT/runs/stage-28/cgroup-final.snapshot"
assert_eq 28 "$(awk 'NR == 1 { next } { print $3 }' "$STAGED_ROOT/runs/stage-28/summary.tsv" | sort -u | wc -l | tr -d '[:space:]')" "stage-28 distinct region count"
assert_eq 'AT,BE,BG,CA,CH,CZ,DE,DK,EE,ES,FI,FR,GB,HU,IE,IN,IT,JP,LV,NL,NO,PL,RO,RS,SE,SG,SK,US' "$(awk 'NR == 1 { next } { print $3 }' "$STAGED_ROOT/runs/stage-28/summary.tsv" | paste -sd, -)" "stage-28 default region order"

printf '[test] offline multi-instance harness verification passed\n'
