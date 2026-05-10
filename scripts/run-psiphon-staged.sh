#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
HARNESS_SCRIPT="$SCRIPT_DIR/psiphon-multi-instance.sh"

RUNTIME_ROOT="$REPO_ROOT/.work/psiphon-harness-staged"
BASE_CONFIG="$REPO_ROOT/psiphon.config"
BINARY_PATH=
DOWNLOAD_IF_MISSING=0
DOWNLOAD_URL=
WAIT_SECONDS=5
STARTUP_GRACE_SECONDS=2
REGIONS=

usage() {
  cat <<'EOF'
Usage:
  scripts/run-psiphon-staged.sh [options]

Options:
  --binary PATH                 Explicit binary path.
  --download-if-missing         Download binary if no local candidate is found.
  --download-url URL            Override binary download URL.
  --base-config PATH            Base config template.
  --runtime-root PATH           Runtime root for staged runs.
  --regions CSV                 Override comma-separated region list.
  --wait-seconds N              Seconds to wait before final metrics per stage.
  --startup-grace-seconds N     Seconds to allow each stage to initialize.
  --help                        Show this message.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --binary)
      BINARY_PATH=$2
      shift 2
      ;;
    --download-if-missing)
      DOWNLOAD_IF_MISSING=1
      shift
      ;;
    --download-url)
      DOWNLOAD_URL=$2
      shift 2
      ;;
    --base-config)
      BASE_CONFIG=$2
      shift 2
      ;;
    --runtime-root)
      RUNTIME_ROOT=$2
      shift 2
      ;;
    --regions)
      REGIONS=$2
      shift 2
      ;;
    --wait-seconds)
      WAIT_SECONDS=$2
      shift 2
      ;;
    --startup-grace-seconds)
      STARTUP_GRACE_SECONDS=$2
      shift 2
      ;;
    --help)
      usage
      exit 0
      ;;
    *)
      printf '[staged] ERROR: unknown option: %s\n' "$1" >&2
      usage >&2
      exit 64
      ;;
  esac
done

mkdir -p "$RUNTIME_ROOT"

RESULTS_PATH="$RUNTIME_ROOT/stage-results.tsv"
printf 'stage\texit_code\trun_dir\tsummary_path\tmetrics_path\n' > "$RESULTS_PATH"

overall_exit=0

for count in 3 8 28; do
  run_name="stage-${count}"
  run_dir="$RUNTIME_ROOT/runs/$run_name"
  summary_path="$run_dir/summary.tsv"
  metrics_path="$run_dir/metrics-final.tsv"
  http_base=$((18080 + (count * 10)))
  socks_base=$((11080 + (count * 10)))

  printf '[staged] running %s instance stage\n' "$count"
  rm -rf "$run_dir"

  args=(
    run
    --base-config "$BASE_CONFIG"
    --runtime-root "$RUNTIME_ROOT"
    --run-name "$run_name"
    --count "$count"
    --http-port-base "$http_base"
    --socks-port-base "$socks_base"
    --wait-seconds "$WAIT_SECONDS"
    --startup-grace-seconds "$STARTUP_GRACE_SECONDS"
  )

  if [ -n "$REGIONS" ]; then
    args+=(--regions "$REGIONS")
  fi

  if [ -n "$BINARY_PATH" ]; then
    args+=(--binary "$BINARY_PATH")
  fi

  if [ "$DOWNLOAD_IF_MISSING" -eq 1 ]; then
    args+=(--download-if-missing)
  fi

  if [ -n "$DOWNLOAD_URL" ]; then
    args+=(--download-url "$DOWNLOAD_URL")
  fi

  if bash "$HARNESS_SCRIPT" "${args[@]}"; then
    stage_exit=0
  else
    stage_exit=$?
    overall_exit=1
  fi

  printf '%s\t%s\t%s\t%s\t%s\n' \
    "$count" \
    "$stage_exit" \
    "$run_dir" \
    "$summary_path" \
    "$metrics_path" >> "$RESULTS_PATH"
done

printf '[staged] results: %s\n' "$RESULTS_PATH"
exit "$overall_exit"
