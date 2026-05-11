#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)

DEFAULT_RUNTIME_ROOT="$REPO_ROOT/.work/psiphon-harness"
DEFAULT_BASE_CONFIG="$REPO_ROOT/psiphon.config"
DEFAULT_BINARY_DOWNLOAD_URL="https://raw.githubusercontent.com/Psiphon-Labs/psiphon-tunnel-core-binaries/master/linux/psiphon-tunnel-core-x86_64"
DEFAULT_REGIONS_FALLBACK="AT,BE,BG,CA,CH,CZ,DE,DK,EE,ES,FI,FR,GB,HU,IE,IN,IT,JP,LV,NL,NO,PL,RO,RS,SE,SG,SK,US"
REGIONS_CATALOG_PATH="$REPO_ROOT/regions.txt"

EXIT_USAGE=64
EXIT_BINARY_NOT_FOUND=65
EXIT_DOWNLOAD_FAILED=66
EXIT_INSTANCE_FAILED=67
EXIT_VALIDATION_FAILED=68

KEEP_RUNNING=0
declare -a RUN_PIDS=()

load_default_regions() {
  local csv=

  if [ -r "$REGIONS_CATALOG_PATH" ]; then
    csv=$(
      awk '
        /^[[:space:]]*(#|$)/ { next }
        {
          line = $0
          sub(/^[[:space:]]+/, "", line)
          sub(/[[:space:]]+$/, "", line)
          if (line == "") {
            next
          }
          if (count > 0) {
            printf ","
          }
          printf "%s", line
          count++
        }
      ' "$REGIONS_CATALOG_PATH" 2>/dev/null
    ) || csv=

    if [ -n "$csv" ]; then
      printf '%s' "$csv"
      return 0
    fi
  fi

  printf '%s' "$DEFAULT_REGIONS_FALLBACK"
}

DEFAULT_REGIONS=$(load_default_regions)

log() {
  printf '[harness] %s\n' "$*"
}

err() {
  printf '[harness] ERROR: %s\n' "$*" >&2
}

usage() {
  cat <<'EOF'
Usage:
  scripts/psiphon-multi-instance.sh locate-binary [--binary PATH] [--runtime-root PATH]
  scripts/psiphon-multi-instance.sh download-binary [--output PATH] [--url URL]
  scripts/psiphon-multi-instance.sh run [options]

Commands:
  locate-binary     Resolve a repo-local psiphon-tunnel-core-x86_64 path.
  download-binary   Disabled until executable authenticity verification exists.
  run               Generate isolated configs and launch N instances.

Run options:
  --binary PATH                 Explicit binary path.
  --download-if-missing         Disabled until executable authenticity verification exists.
  --download-url URL            Disabled until executable authenticity verification exists.
  --base-config PATH            Base config template (default: ./psiphon.config).
  --runtime-root PATH           Runtime root (default: ./.work/psiphon-harness).
  --run-name NAME               Stable run directory name under runtime root.
  --count N                     Number of instances to launch (default: 1).
  --regions CSV                 Comma-separated EgressRegion values.
  --http-port-base N            First LocalHttpProxyPort (default: 18080).
  --socks-port-base N           First LocalSocksProxyPort (default: 11080).
  --wait-seconds N              Seconds to wait before final metrics (default: 5).
  --startup-grace-seconds N     Seconds to allow processes to initialize (default: 2).
  --keep-running                Leave processes running on exit.
  --help                        Show this message.

Artifacts:
  runtime-root/
    bin/
    runs/<run-name>/
      instances/instance-XXX/{config.json,data/,notices.jsonl,stdout.log,stderr.log,pid}
      summary.tsv
      metrics-start.tsv
      metrics-final.tsv
      cgroup-start.snapshot
      cgroup-final.snapshot
EOF
}

cleanup() {
  local exit_code=${1:-0}
  trap - EXIT INT TERM

  if [ "$KEEP_RUNNING" -eq 0 ] && [ "${#RUN_PIDS[@]}" -gt 0 ]; then
    local pid
    for pid in "${RUN_PIDS[@]}"; do
      if kill -0 "$pid" 2>/dev/null; then
        kill "$pid" 2>/dev/null || true
      fi
    done

    for pid in "${RUN_PIDS[@]}"; do
      wait "$pid" 2>/dev/null || true
    done
  fi

  exit "$exit_code"
}

has_command() {
  command -v "$1" >/dev/null 2>&1
}

require_non_negative_integer() {
  local label=$1
  local value=$2

  case "$value" in
    ''|*[!0-9]*)
      err "$label must be a non-negative integer: $value"
      return 1
      ;;
  esac
}

require_positive_integer() {
  local label=$1
  local value=$2

  require_non_negative_integer "$label" "$value" || return 1

  if [ "$value" -le 0 ]; then
    err "$label must be greater than zero: $value"
    return 1
  fi
}

trim_spaces() {
  local value=$1

  value=${value#${value%%[![:space:]]*}}
  value=${value%${value##*[![:space:]]}}
  printf '%s' "$value"
}

is_known_region() {
  local region=$1
  local known_region
  local IFS=,
  local -a known_regions=($DEFAULT_REGIONS)

  for known_region in "${known_regions[@]}"; do
    if [ "$region" = "$known_region" ]; then
      return 0
    fi
  done

  return 1
}

build_region_list() {
  local instance_count=$1
  local override_csv=${2:-}
  local output_path=$3
  local source_csv region raw_region count=0
  local IFS=,
  local -a source_regions=()

  if [ -n "$override_csv" ]; then
    source_csv=$override_csv
  else
    source_csv=$DEFAULT_REGIONS
  fi

  source_regions=($source_csv)
  : > "$output_path"

  for raw_region in "${source_regions[@]}"; do
    region=$(trim_spaces "$raw_region")

    if [ -z "$region" ]; then
      continue
    fi

    if ! is_known_region "$region"; then
      err "unknown region code: $region"
      return 1
    fi

    printf '%s\n' "$region" >> "$output_path"
    count=$((count + 1))

    if [ "$count" -eq "$instance_count" ]; then
      break
    fi
  done

  if [ "$count" -ne "$instance_count" ]; then
    err "need at least $instance_count region value(s); only found $count"
    return 1
  fi
}

resolve_binary() {
  local explicit_binary=${1:-}
  local runtime_root=${2:-$DEFAULT_RUNTIME_ROOT}
  local -a candidates=()
  local candidate

  if [ -n "$explicit_binary" ]; then
    candidates+=("$explicit_binary")
  fi

  candidates+=(
    "$REPO_ROOT/psiphon-tunnel-core-x86_64"
    "$REPO_ROOT/archive/psiphon-tunnel-core-x86_64"
    "$runtime_root/bin/psiphon-tunnel-core-x86_64"
  )

  for candidate in "${candidates[@]}"; do
    if [ -f "$candidate" ]; then
      chmod +x "$candidate" 2>/dev/null || true
      printf '%s\n' "$candidate"
      return 0
    fi
  done

  return 1
}

download_binary() {
  local output_path=$1
  local download_url=$2

  mkdir -p "$(dirname -- "$output_path")"

  if has_command curl; then
    curl -fsSL "$download_url" -o "$output_path"
  elif has_command wget; then
    wget -q -O "$output_path" "$download_url"
  else
    err "curl or wget is required to download the binary"
    return 1
  fi

  chmod +x "$output_path"
}

download_disabled() {
  err "automatic download is disabled until executable authenticity verification exists"
  return "$EXIT_DOWNLOAD_FAILED"
}

render_instance_config() {
  local base_config=$1
  local output_path=$2
  local http_port=$3
  local socks_port=$4
  local remote_filename=$5
  local egress_region=$6

  awk \
    -v http_port="$http_port" \
    -v socks_port="$socks_port" \
    -v remote_filename="$remote_filename" \
    -v egress_region="$egress_region" '
      BEGIN {
        seen_http = 0
        seen_socks = 0
        seen_remote = 0
        seen_region = 0
      }
      {
        if ($0 ~ /"LocalHttpProxyPort"[[:space:]]*:/) {
          print "\"LocalHttpProxyPort\":" http_port ","
          seen_http = 1
          next
        }
        if ($0 ~ /"LocalSocksProxyPort"[[:space:]]*:/) {
          print "\"LocalSocksProxyPort\":" socks_port ","
          seen_socks = 1
          next
        }
        if ($0 ~ /"RemoteServerListDownloadFilename"[[:space:]]*:/) {
          print "\"RemoteServerListDownloadFilename\":\"" remote_filename "\"," 
          seen_remote = 1
          next
        }
        if ($0 ~ /"EgressRegion"[[:space:]]*:/) {
          print "\"EgressRegion\":\"" egress_region "\"," 
          seen_region = 1
          next
        }
        print
      }
      END {
        if (!(seen_http && seen_socks && seen_remote && seen_region)) {
          exit 11
        }
      }
    ' "$base_config" > "$output_path"
}

snapshot_cgroup_state() {
  local output_path=$1
  local path label value
  local -a probes=(
    "/sys/fs/cgroup/memory.current:memory.current"
    "/sys/fs/cgroup/memory.max:memory.max"
    "/sys/fs/cgroup/pids.current:pids.current"
    "/sys/fs/cgroup/pids.max:pids.max"
    "/sys/fs/cgroup/memory/memory.usage_in_bytes:memory.usage_in_bytes"
    "/sys/fs/cgroup/memory/memory.limit_in_bytes:memory.limit_in_bytes"
    "/sys/fs/cgroup/pids/pids.current:pids.current.v1"
    "/sys/fs/cgroup/pids/pids.max:pids.max.v1"
  )

  {
    printf 'timestamp\t%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    printf 'dockerenv_present\t'
    if [ -e '/.dockerenv' ]; then
      printf 'yes\n'
    else
      printf 'no\n'
    fi

    for path in '/proc/1/cgroup' '/proc/self/cgroup' '/proc/1/cpuset'; do
      if [ -r "$path" ]; then
        label=$(basename "$path")
        value=$(tr '\n' ';' < "$path")
        printf '%s\t%s\n' "$label" "$value"
      fi
    done

    for probe in "${probes[@]}"; do
      path=${probe%%:*}
      label=${probe#*:}

      if [ -r "$path" ]; then
        value=$(tr -d '\n' < "$path")
        printf '%s\t%s\n' "$label" "$value"
      else
        printf '%s\tunavailable\n' "$label"
      fi
    done
  } > "$output_path"
}

collect_metrics() {
  local output_path=$1
  shift
  local -a pids=("$@")
  local pid

  {
    printf 'pid\tppid\tstate\tcpu_percent\trss_kb\tvsz_kb\telapsed\tcommand\n'

    for pid in "${pids[@]}"; do
      if kill -0 "$pid" 2>/dev/null; then
        ps -o pid= -o ppid= -o state= -o pcpu= -o rss= -o vsz= -o etime= -o command= -p "$pid" \
          | awk '{
              pid = $1
              ppid = $2
              state = $3
              cpu = $4
              rss = $5
              vsz = $6
              elapsed = $7
              $1 = ""
              $2 = ""
              $3 = ""
              $4 = ""
              $5 = ""
              $6 = ""
              $7 = ""
              sub(/^[[:space:]]+/, "", $0)
              printf "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", pid, ppid, state, cpu, rss, vsz, elapsed, $0
            }'
      else
        printf '%s\t-\texited\t0\t0\t0\t-\t-\n' "$pid"
      fi
    done
  } > "$output_path"
}

notice_flag() {
  local notices_path=$1
  local needle=$2

  if [ -s "$notices_path" ] && grep -q "$needle" "$notices_path"; then
    printf 'yes'
  else
    printf 'no'
  fi
}

run_instances() {
  local binary_path=
  local download_if_missing=0
  local download_url=$DEFAULT_BINARY_DOWNLOAD_URL
  local download_url_requested=0
  local base_config=$DEFAULT_BASE_CONFIG
  local runtime_root=$DEFAULT_RUNTIME_ROOT
  local run_name=
  local instance_count=1
  local regions_csv=
  local http_port_base=18080
  local socks_port_base=11080
  local wait_seconds=5
  local startup_grace_seconds=2

  while [ "$#" -gt 0 ]; do
    case "$1" in
      --binary)
        binary_path=$2
        shift 2
        ;;
      --download-if-missing)
        download_if_missing=1
        shift
        ;;
      --download-url)
        download_url=$2
        download_url_requested=1
        shift 2
        ;;
      --base-config)
        base_config=$2
        shift 2
        ;;
      --runtime-root)
        runtime_root=$2
        shift 2
        ;;
      --run-name)
        run_name=$2
        shift 2
        ;;
      --count)
        instance_count=$2
        shift 2
        ;;
      --regions)
        regions_csv=$2
        shift 2
        ;;
      --http-port-base)
        http_port_base=$2
        shift 2
        ;;
      --socks-port-base)
        socks_port_base=$2
        shift 2
        ;;
      --wait-seconds)
        wait_seconds=$2
        shift 2
        ;;
      --startup-grace-seconds)
        startup_grace_seconds=$2
        shift 2
        ;;
      --keep-running)
        KEEP_RUNNING=1
        shift
        ;;
      --help)
        usage
        return 0
        ;;
      *)
        err "unknown run option: $1"
        usage >&2
        return "$EXIT_USAGE"
        ;;
    esac
  done

  if [ ! -f "$base_config" ]; then
    err "base config not found: $base_config"
    return "$EXIT_USAGE"
  fi

  require_positive_integer "instance count" "$instance_count" || return "$EXIT_USAGE"
  require_positive_integer "HTTP port base" "$http_port_base" || return "$EXIT_USAGE"
  require_positive_integer "SOCKS port base" "$socks_port_base" || return "$EXIT_USAGE"
  require_non_negative_integer "wait seconds" "$wait_seconds" || return "$EXIT_USAGE"
  require_non_negative_integer "startup grace seconds" "$startup_grace_seconds" || return "$EXIT_USAGE"

  if [ "$download_if_missing" -eq 1 ] || [ "$download_url_requested" -eq 1 ]; then
    download_disabled
    return "$EXIT_DOWNLOAD_FAILED"
  fi

  if ! binary_path=$(resolve_binary "$binary_path" "$runtime_root"); then
    err "unable to locate psiphon-tunnel-core-x86_64"
    return "$EXIT_BINARY_NOT_FOUND"
  fi

  mkdir -p "$runtime_root/runs"

  if [ -z "$run_name" ]; then
    run_name="run-$(date +%Y%m%d-%H%M%S)-${instance_count}i"
  fi

  local run_dir="$runtime_root/runs/$run_name"
  local instances_dir="$run_dir/instances"
  local summary_path="$run_dir/summary.tsv"
  local metrics_start_path="$run_dir/metrics-start.tsv"
  local metrics_final_path="$run_dir/metrics-final.tsv"
  local cgroup_start_path="$run_dir/cgroup-start.snapshot"
  local cgroup_final_path="$run_dir/cgroup-final.snapshot"
  local manifest_path="$run_dir/run.env"
  local regions_path="$run_dir/regions.txt"
  local i instance_id instance_dir config_path data_dir notices_path stdout_path stderr_path pid_path
  local http_port socks_port remote_filename instance_region pid alive_count=0

  if [ -e "$run_dir" ]; then
    err "run directory already exists: $run_dir"
    return "$EXIT_VALIDATION_FAILED"
  fi

  mkdir -p "$instances_dir"

  build_region_list "$instance_count" "$regions_csv" "$regions_path" || return "$EXIT_USAGE"

  cat > "$manifest_path" <<EOF
RUN_DIR=$run_dir
SUMMARY_PATH=$summary_path
METRICS_START_PATH=$metrics_start_path
METRICS_FINAL_PATH=$metrics_final_path
CGROUP_START_PATH=$cgroup_start_path
CGROUP_FINAL_PATH=$cgroup_final_path
BINARY_PATH=$binary_path
BASE_CONFIG=$base_config
INSTANCE_COUNT=$instance_count
REGIONS=$(paste -sd, "$regions_path")
EOF

  printf 'instance\tpid\tregion\thttp_port\tsocks_port\trunning_after_startup\thttp_notice\tsocks_notice\ttunnels_notice\tconfig_path\tdata_dir\tnotices_path\tstdout_path\tstderr_path\n' > "$summary_path"

  log "starting $instance_count instance(s) using $binary_path"
  log "artifacts will be written to $run_dir"

  for ((i = 1; i <= instance_count; i++)); do
    instance_id=$(printf 'instance-%03d' "$i")
    instance_dir="$instances_dir/$instance_id"
    config_path="$instance_dir/config.json"
    data_dir="$instance_dir/data"
    notices_path="$instance_dir/notices.jsonl"
    stdout_path="$instance_dir/stdout.log"
    stderr_path="$instance_dir/stderr.log"
    pid_path="$instance_dir/pid"
    http_port=$((http_port_base + i - 1))
    socks_port=$((socks_port_base + i - 1))
    remote_filename="remote_server_list_${instance_id}"
    instance_region=$(sed -n "${i}p" "$regions_path")

    mkdir -p "$instance_dir" "$data_dir"
    render_instance_config "$base_config" "$config_path" "$http_port" "$socks_port" "$remote_filename" "$instance_region"

    "$binary_path" \
      -config "$config_path" \
      -dataRootDirectory "$data_dir" \
      -notices "$notices_path" \
      >"$stdout_path" \
      2>"$stderr_path" &
    pid=$!

    RUN_PIDS+=("$pid")
    printf '%s\n' "$pid" > "$pid_path"
  done

  sleep "$startup_grace_seconds"
  snapshot_cgroup_state "$cgroup_start_path"
  collect_metrics "$metrics_start_path" "${RUN_PIDS[@]}"

  for ((i = 1; i <= instance_count; i++)); do
    instance_id=$(printf 'instance-%03d' "$i")
    instance_dir="$instances_dir/$instance_id"
    config_path="$instance_dir/config.json"
    data_dir="$instance_dir/data"
    notices_path="$instance_dir/notices.jsonl"
    stdout_path="$instance_dir/stdout.log"
    stderr_path="$instance_dir/stderr.log"
    pid_path="$instance_dir/pid"
    pid=$(cat "$pid_path")
    instance_region=$(sed -n "${i}p" "$regions_path")
    http_port=$((http_port_base + i - 1))
    socks_port=$((socks_port_base + i - 1))

    if kill -0 "$pid" 2>/dev/null; then
      alive_count=$((alive_count + 1))
      printf '%s\t%s\t%s\t%s\t%s\tyes\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
        "$instance_id" \
        "$pid" \
        "$instance_region" \
        "$http_port" \
        "$socks_port" \
        "$(notice_flag "$notices_path" 'ListeningHttpProxyPort')" \
        "$(notice_flag "$notices_path" 'ListeningSocksProxyPort')" \
        "$(notice_flag "$notices_path" 'Tunnels')" \
        "$config_path" \
        "$data_dir" \
        "$notices_path" \
        "$stdout_path" \
        "$stderr_path" >> "$summary_path"
    else
      printf '%s\t%s\t%s\t%s\t%s\tno\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
        "$instance_id" \
        "$pid" \
        "$instance_region" \
        "$http_port" \
        "$socks_port" \
        "$(notice_flag "$notices_path" 'ListeningHttpProxyPort')" \
        "$(notice_flag "$notices_path" 'ListeningSocksProxyPort')" \
        "$(notice_flag "$notices_path" 'Tunnels')" \
        "$config_path" \
        "$data_dir" \
        "$notices_path" \
        "$stdout_path" \
        "$stderr_path" >> "$summary_path"
    fi
  done

  if [ "$wait_seconds" -gt 0 ]; then
    sleep "$wait_seconds"
  fi

  snapshot_cgroup_state "$cgroup_final_path"
  collect_metrics "$metrics_final_path" "${RUN_PIDS[@]}"

  log "summary: $summary_path"
  log "metrics: $metrics_final_path"
  log "cgroup snapshots: $cgroup_start_path $cgroup_final_path"

  if [ "$alive_count" -ne "$instance_count" ]; then
    err "$alive_count of $instance_count instance(s) remained alive through startup"
    return "$EXIT_INSTANCE_FAILED"
  fi

  log "all $instance_count instance(s) remained alive through startup"
  log "network/tunnel success is reported separately via notices and is not required for harness success"
}

main() {
  local command=${1:-}

  if [ -z "$command" ]; then
    usage >&2
    exit "$EXIT_USAGE"
  fi

  shift
  trap 'cleanup "$?"' EXIT INT TERM

  case "$command" in
    locate-binary)
      local binary_path=
      local runtime_root=$DEFAULT_RUNTIME_ROOT

      while [ "$#" -gt 0 ]; do
        case "$1" in
          --binary)
            binary_path=$2
            shift 2
            ;;
          --runtime-root)
            runtime_root=$2
            shift 2
            ;;
          --help)
            usage
            return 0
            ;;
          *)
            err "unknown locate-binary option: $1"
            return "$EXIT_USAGE"
            ;;
        esac
      done

      if resolve_binary "$binary_path" "$runtime_root"; then
        return 0
      fi

      err "unable to locate psiphon-tunnel-core-x86_64"
      return "$EXIT_BINARY_NOT_FOUND"
      ;;
    download-binary)
      while [ "$#" -gt 0 ]; do
        case "$1" in
          --output)
            shift 2
            ;;
          --url)
            shift 2
            ;;
          --help)
            usage
            return 0
            ;;
          *)
            err "unknown download-binary option: $1"
            return "$EXIT_USAGE"
            ;;
        esac
      done

      download_disabled
      return "$EXIT_DOWNLOAD_FAILED"
      ;;
    run)
      run_instances "$@"
      ;;
    --help|-h|help)
      usage
      ;;
    *)
      err "unknown command: $command"
      usage >&2
      return "$EXIT_USAGE"
      ;;
  esac
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
