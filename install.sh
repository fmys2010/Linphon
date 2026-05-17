#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LINPH_BIN="$REPO_ROOT/tools/psiphon-mg/bin/linph"
DEFAULT_BASE_CONFIG="$REPO_ROOT/psiphon.config"
DEFAULT_INSTALL_BIN_DIR="/usr/local/bin"
DEFAULT_INSTALL_CONFIG_DIR="/etc/psiphon"
DEFAULT_RUNTIME_ROOT="$REPO_ROOT/.work/psiphon-harness"
DEFAULT_SUPPORTED_REGIONS="AT,BE,BG,CA,CH,CZ,DE,DK,EE,ES,FI,FR,GB,HU,IE,IN,IT,JP,LV,NL,NO,PL,RO,RS,SE,SG,SK,US"

load_supported_regions_csv() {
  local path="$REPO_ROOT/regions.txt"
  local -a regions=()
  local line=""

  if [[ -f "$path" ]]; then
    while IFS= read -r line; do
      line="${line//$'\r'/}"
      line="${line#"${line%%[![:space:]]*}"}"
      line="${line%"${line##*[![:space:]]}"}"
      [[ -z "$line" || "$line" == \#* ]] && continue
      regions+=("${line^^}")
    done < "$path"
  fi

  if [[ "${#regions[@]}" -eq 0 ]]; then
    printf '%s\n' "$DEFAULT_SUPPORTED_REGIONS"
    return 0
  fi

  local joined=""
  local region
  for region in "${regions[@]}"; do
    if [[ -z "$joined" ]]; then
      joined="$region"
    else
      joined+=",$region"
    fi
  done
  printf '%s\n' "$joined"
}

SUPPORTED_REGIONS_CSV="$(load_supported_regions_csv)"

normalize_region_token() {
  local value="$1"
  value="${value//[[:space:]]/}"
  printf '%s\n' "${value^^}"
}

region_is_supported() {
  local want
  want="$(normalize_region_token "$1")"
  local IFS=,
  local region
  for region in $SUPPORTED_REGIONS_CSV; do
    [[ "$want" == "$region" ]] && return 0
  done
  return 1
}

print_supported_regions() {
  printf 'Supported regions: %s\n' "$SUPPORTED_REGIONS_CSV"
}

ensure_command() {
  local name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    printf 'required command not found: %s\n' "$name" >&2
    exit 1
  fi
}

cancel_install() {
  printf '\ninstall cancelled\n'
  exit 0
}

choose_install_mode() {
  local __outvar="$1"
  local choice=""

  while true; do
    read_with_prompt choice "Install mode: [1] single-port, [2] multi-port [1]: "
    case "$choice" in
      ""|1|single|single-port)
        printf -v "$__outvar" '%s' "single"
        return 0
        ;;
      2|multi|multi-port)
        printf -v "$__outvar" '%s' "multi"
        return 0
        ;;
    esac
    printf 'please choose 1 for single-port or 2 for multi-port\n' >&2
  done
}

prompt_positive_int() {
  local __outvar="$1"
  local prompt="$2"
  local input=""

  while true; do
    read_with_prompt input "$prompt"
    input="${input//[[:space:]]/}"
    case "$input" in
      *[!0-9]*|"")
        printf 'please enter a positive integer\n' >&2
        ;;
      *)
        if [[ "$input" -gt 0 ]]; then
          printf -v "$__outvar" '%s' "$input"
          return 0
        fi
        printf 'please enter a positive integer\n' >&2
        ;;
    esac
  done
}

prompt_regions_csv() {
  local __outvar="$1"
  local slot_count="$2"
  local prompt="$3"
  local input=""
  local -a selected=()
  local -a raw_parts=()
  local region=""
  local part=""
  local invalid=0

  while true; do
    print_supported_regions
    read_with_prompt input "$prompt"

    selected=()
    raw_parts=()
    invalid=0
    IFS=',' read -r -a raw_parts <<< "$input"
    for part in "${raw_parts[@]}"; do
      region="$(normalize_region_token "$part")"
      [[ -z "$region" ]] && continue
      if ! region_is_supported "$region"; then
        printf 'unsupported region: %s\n' "$region" >&2
        invalid=1
        break
      fi
      selected+=("$region")
      if [[ "${#selected[@]}" -eq "$slot_count" ]]; then
        break
      fi
    done

    if [[ "$invalid" -ne 0 ]]; then
      continue
    fi
    if [[ "${#selected[@]}" -lt "$slot_count" ]]; then
      printf 'please enter at least %s supported region(s)\n' "$slot_count" >&2
      continue
    fi

    local joined=""
    for region in "${selected[@]}"; do
      if [[ -z "$joined" ]]; then
        joined="$region"
      else
        joined+=",$region"
      fi
    done
    if [[ -n "$joined" ]]; then
      printf -v "$__outvar" '%s' "$joined"
      return 0
    fi
  done
}

read_with_prompt() {
  local __outvar="$1"
  local prompt="$2"
  local read_value=""

  if ! read -r -p "$prompt" read_value; then
    cancel_install
  fi

  read_value="${read_value//$'\r'/}"
  printf -v "$__outvar" '%s' "$read_value"
}

build_linph() {
  ensure_command go
  mkdir -p "$(dirname "$LINPH_BIN")"
  (
    cd "$REPO_ROOT/tools/psiphon-mg"
    go build -o ../../tools/psiphon-mg/bin/linph ./cmd/linph
  )
}

run_install() {
  local -a install_args=("$@")

  if [[ "$(id -u)" -ne 0 ]]; then
    ensure_command sudo
    exec sudo env PSIPHON_MG_REPO_ROOT="$REPO_ROOT" "$LINPH_BIN" install "${install_args[@]}"
  fi

  exec env PSIPHON_MG_REPO_ROOT="$REPO_ROOT" "$LINPH_BIN" install "${install_args[@]}"
}

resolve_default_binary() {
  local candidate
  for candidate in \
    "$REPO_ROOT/psiphon-tunnel-core-x86_64" \
    "$REPO_ROOT/archive/psiphon-tunnel-core-x86_64" \
    "$DEFAULT_RUNTIME_ROOT/bin/psiphon-tunnel-core-x86_64"
  do
    if [[ -f "$candidate" ]]; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done

  return 1
}

prompt_for_existing_file() {
  local __outvar="$1"
  local prompt="$2"
  local default_value="${3-}"
  local input=""

  while true; do
    if [[ -n "$default_value" ]]; then
      read_with_prompt input "$prompt [$default_value]: "
      input="${input:-$default_value}"
    else
      read_with_prompt input "$prompt: "
    fi

    if [[ -f "$input" ]]; then
      printf -v "$__outvar" '%s' "$input"
      return 0
    fi

    printf 'file not found: %s\n' "$input" >&2
  done
}

confirm_install() {
  local reply=""
  read_with_prompt reply "Continue install? [Y/n] "
  case "$reply" in
    ""|y|Y|yes|YES|Yes)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

run_interactive_install() {
  local detected_binary=""
  local binary_path=""
  local config_path="$DEFAULT_BASE_CONFIG"
  local install_reply=""
  local install_mode=""
  local slot_count="1"
  local http_port=""
  local socks_port=""
  local regions_csv=""
  local -a install_args=()

  detected_binary="$(resolve_default_binary || true)"

  printf 'Linphon interactive install\n'
  read_with_prompt install_reply "Install Linphon now? [Y/n] "
  case "$install_reply" in
    ""|y|Y|yes|YES|Yes)
      ;;
    *)
      cancel_install
      ;;
  esac

  choose_install_mode install_mode

  if [[ "$install_mode" == "single" ]]; then
    slot_count=1
    prompt_positive_int http_port "Enter HTTP port: "
    prompt_positive_int socks_port "Enter SOCKS5 port: "
    prompt_regions_csv regions_csv "$slot_count" "Enter region code: "
  else
    prompt_positive_int slot_count "Enter slot count (1-5): "
    while [[ "$slot_count" -gt 5 ]]; do
      printf 'slot count must be 5 or less\n' >&2
      prompt_positive_int slot_count "Enter slot count (1-5): "
    done
    prompt_positive_int http_port "Enter starting HTTP port: "
    prompt_positive_int socks_port "Enter starting SOCKS5 port: "
    prompt_regions_csv regions_csv "$slot_count" "Enter comma-separated regions: "
  fi

  if [[ -n "$detected_binary" ]]; then
    printf 'Detected tunnel-core: %s\n' "$detected_binary"
    prompt_for_existing_file binary_path "Tunnel-core path (press Enter to use detected value)" "$detected_binary"
  else
    printf 'No reviewed tunnel-core was found in the default repo locations.\n'
    prompt_for_existing_file binary_path "Enter tunnel-core path"
  fi

  if [[ ! -f "$config_path" ]]; then
    printf 'Default base config not found: %s\n' "$config_path" >&2
    prompt_for_existing_file config_path "Enter base config path"
  fi

  printf '\nInstall summary:\n'
  printf '  mode            -> %s\n' "$install_mode"
  printf '  slot count      -> %s\n' "$slot_count"
  printf '  start HTTP port -> %s\n' "$http_port"
  printf '  start SOCKS port-> %s\n' "$socks_port"
  printf '  regions         -> %s\n' "$regions_csv"
  printf '  linph and aliases -> %s\n' "$DEFAULT_INSTALL_BIN_DIR"
  printf '  psiphon assets    -> %s\n' "$DEFAULT_INSTALL_CONFIG_DIR"
  printf '  tunnel-core       -> %s\n' "$binary_path"
  printf '  base config       -> %s\n' "$config_path"

  if ! confirm_install; then
    cancel_install
  fi

  build_linph
  install_args+=(--base-config "$config_path")
  install_args+=(--installed-slot-count "$slot_count")
  install_args+=(--installed-http-port "$http_port")
  install_args+=(--installed-socks-port "$socks_port")
  install_args+=(--installed-regions "$regions_csv")
  install_args+=(--binary "$binary_path")
  run_install "${install_args[@]}"
}

run_install_help() {
  build_linph
  exec env PSIPHON_MG_REPO_ROOT="$REPO_ROOT" "$LINPH_BIN" install "$@"
}

main() {
  if [[ $# -eq 1 && ( "$1" == "--help" || "$1" == "-h" ) ]]; then
    run_install_help "$@"
  fi

  if [[ $# -eq 0 && -t 0 && -t 1 ]]; then
    run_interactive_install
  fi

  build_linph
  run_install "$@"
}

main "$@"
