#!/usr/bin/env bash
set -euo pipefail

BOOTSTRAP_SOURCE_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BOOTSTRAP_TEMP_ROOT=""
LINPHON_BOOTSTRAP_ARCHIVE_URL="${LINPHON_BOOTSTRAP_ARCHIVE_URL:-https://github.com/fmys2010/Linphon/archive/refs/heads/main.tar.gz}"

bootstrap_repo_root() {
  local candidate="$1"
  local temp_root=""

  if [[ -f "$candidate/tools/psiphon-mg/go.mod" ]]; then
    printf '%s\n' "$candidate"
    return 0
  fi

  if ! command -v curl >/dev/null 2>&1; then
    printf 'curl is required to bootstrap Linphon from a remote script\n' >&2
    return 1
  fi
  if ! command -v tar >/dev/null 2>&1; then
    printf 'tar is required to bootstrap Linphon from a remote script\n' >&2
    return 1
  fi

  temp_root="$(mktemp -d "${TMPDIR:-/tmp}/linphon-bootstrap.XXXXXX")"
  BOOTSTRAP_TEMP_ROOT="$temp_root"
  printf 'downloaded install.sh without repository sources; fetching Linphon source archive\n' >&2
  if ! curl -fsSL "$LINPHON_BOOTSTRAP_ARCHIVE_URL" | tar -xz -C "$temp_root" --strip-components=1; then
    rm -rf "$temp_root"
    BOOTSTRAP_TEMP_ROOT=""
    return 1
  fi
  if [[ ! -f "$temp_root/tools/psiphon-mg/go.mod" ]]; then
    printf 'downloaded Linphon source archive is missing tools/psiphon-mg/go.mod\n' >&2
    return 1
  fi
  printf '%s\n' "$temp_root"
}

cleanup_bootstrap_temp_root() {
  if [[ -n "$BOOTSTRAP_TEMP_ROOT" ]]; then
    rm -rf "$BOOTSTRAP_TEMP_ROOT"
  fi
}

trap cleanup_bootstrap_temp_root EXIT

DEFAULT_INSTALL_BIN_DIR="/usr/local/bin"
DEFAULT_INSTALL_CONFIG_DIR="/etc/psiphon"
REPO_ROOT=""
LINPH_BIN=""
DEFAULT_BASE_CONFIG=""
DEFAULT_RUNTIME_ROOT=""
DEFAULT_SUPPORTED_REGIONS="AT,BE,BG,CA,CH,CZ,DE,DK,EE,ES,FI,FR,GB,HU,IE,IN,IT,JP,LV,NL,NO,PL,RO,RS,SE,SG,SK,US"
DEFAULT_BOOTSTRAP_VERSION_URL="https://raw.githubusercontent.com/fmys2010/Linphon/main/tools/psiphon-mg/internal/mg/linph.go"
BOOTSTRAP_VERSION_URL="${LINPHON_BOOTSTRAP_VERSION_URL:-$DEFAULT_BOOTSTRAP_VERSION_URL}"
INSTALLED_VERSION_FILENAME=".linph-version"
INSTALLED_SLOT_HARD_LIMIT=28
INSTALLED_SLOT_FALLBACK_CAP=1
INSTALLED_SLOT_MEMORY_SCALE_DIV=100
UNLIMITED_BYTES_THRESHOLD=1125899906842624

SCRIPT_LANG="en"
SCRIPT_FORCE_UNLOCK=0
EFFECTIVE_MEMORY_MIB=0
EFFECTIVE_MEMORY_SOURCE="fallback"
EFFECTIVE_SLOT_CAP="$INSTALLED_SLOT_FALLBACK_CAP"
SERVICE_WARNING_STATE=""
SUPPORTED_REGIONS_CSV=""
FORCE_INSTALL_FLAG=0
DERIVED_HTTP_PORTS=()
DERIVED_SOCKS_PORTS=()
DERIVED_SLOT_LINES=()
PRECHECK_CONFLICTS=()
COLLECTED_START_HTTP_PORT=""
COLLECTED_START_SOCKS_PORT=""

initialize_repo_paths() {
  if [[ -n "$REPO_ROOT" ]]; then
    return 0
  fi
  REPO_ROOT="$(bootstrap_repo_root "$BOOTSTRAP_SOURCE_ROOT")"
  LINPH_BIN="$REPO_ROOT/tools/psiphon-mg/bin/linph"
  DEFAULT_BASE_CONFIG="$REPO_ROOT/psiphon.config"
  DEFAULT_RUNTIME_ROOT="$REPO_ROOT/.work/psiphon-harness"
}

lang_printf() {
  local english="$1"
  local chinese="$2"
  shift 2
  if [[ "$SCRIPT_LANG" == "zh" ]]; then
    printf "$chinese" "$@"
  else
    printf "$english" "$@"
  fi
}

lang_eprintf() {
  local english="$1"
  local chinese="$2"
  shift 2
  if [[ "$SCRIPT_LANG" == "zh" ]]; then
    printf "$chinese" "$@" >&2
  else
    printf "$english" "$@" >&2
  fi
}

read_lang_prompt() {
  local __outvar="$1"
  local english="$2"
  local chinese="$3"
  shift 3
  local prompt=""
  if [[ "$SCRIPT_LANG" == "zh" ]]; then
    printf -v prompt "$chinese" "$@"
  else
    printf -v prompt "$english" "$@"
  fi
  read_with_prompt "$__outvar" "$prompt"
}

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
  lang_printf "Supported regions: %s\n" "支持的地区：%s\n" "$SUPPORTED_REGIONS_CSV"
}

ensure_command() {
  local name="$1"
  if ! command_exists "$name"; then
    lang_eprintf "required command not found: %s\n" "缺少必需命令：%s\n" "$name"
    exit 1
  fi
}

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

can_run_privileged_command() {
  [[ "$(id -u)" -eq 0 ]] || command_exists sudo
}

detect_go_package_manager() {
  local manager=""
  for manager in apt-get dnf yum apk pacman zypper; do
    if command_exists "$manager"; then
      printf '%s\n' "$manager"
      return 0
    fi
  done
  return 1
}

go_package_name_for_manager() {
  case "$1" in
    apt-get)
      printf '%s\n' 'golang-go'
      ;;
    dnf|yum)
      printf '%s\n' 'golang'
      ;;
    apk|pacman|zypper)
      printf '%s\n' 'go'
      ;;
    *)
      return 1
      ;;
  esac
}

describe_go_install_command() {
  local manager="$1"
  local package="$2"
  local prefix=""

  if [[ "$(id -u)" -ne 0 ]]; then
    prefix='sudo '
  fi

  case "$manager" in
    apt-get)
      printf '%sapt-get update && %sapt-get install -y %s\n' "$prefix" "$prefix" "$package"
      ;;
    dnf)
      printf '%sdnf install -y %s\n' "$prefix" "$package"
      ;;
    yum)
      printf '%syum install -y %s\n' "$prefix" "$package"
      ;;
    apk)
      printf '%sapk add %s\n' "$prefix" "$package"
      ;;
    pacman)
      printf '%spacman -Sy --noconfirm %s\n' "$prefix" "$package"
      ;;
    zypper)
      printf '%szypper --non-interactive install %s\n' "$prefix" "$package"
      ;;
    *)
      return 1
      ;;
  esac
}

path_requires_privilege() {
  local path="$1"
  local parent=""

  if [[ -d "$path" ]]; then
    [[ -w "$path" ]] && return 1
    return 0
  fi

  parent="$(dirname "$path")"
  while [[ ! -e "$parent" && "$parent" != "/" ]]; do
    parent="$(dirname "$parent")"
  done

  [[ -d "$parent" && -w "$parent" ]] && return 1
  return 0
}

run_path_command() {
  local target_path="$1"
  shift

  if path_requires_privilege "$target_path"; then
    run_privileged_command "$@"
    return $?
  fi

  "$@"
}

run_privileged_command() {
  if [[ "$(id -u)" -eq 0 ]]; then
    "$@"
    return $?
  fi

  sudo "$@"
}

run_go_auto_install() {
  local manager="$1"
  local package="$2"

  case "$manager" in
    apt-get)
      run_privileged_command env DEBIAN_FRONTEND=noninteractive apt-get update && run_privileged_command env DEBIAN_FRONTEND=noninteractive apt-get install -yq "$package"
      ;;
    dnf)
      run_privileged_command dnf install -y "$package"
      ;;
    yum)
      run_privileged_command yum install -y "$package"
      ;;
    apk)
      run_privileged_command apk add "$package"
      ;;
    pacman)
      run_privileged_command pacman -Sy --noconfirm "$package"
      ;;
    zypper)
      run_privileged_command zypper --non-interactive install "$package"
      ;;
    *)
      return 1
      ;;
  esac
}

extract_linphon_version() {
  local input="$1"
  local version=""
  if [[ "$input" =~ LinphonVersion[[:space:]]*=[[:space:]]*\"([^\"]+)\" ]]; then
    version="${BASH_REMATCH[1]}"
  elif [[ "$input" =~ Linphon[[:space:]]+([0-9]+(\.[0-9]+)*) ]]; then
    version="${BASH_REMATCH[1]}"
  elif [[ "$input" =~ ^[[:space:]]*([0-9]+(\.[0-9]+)*)[[:space:]]*$ ]]; then
    version="${BASH_REMATCH[1]}"
  fi
  printf '%s\n' "$version"
}

write_installed_linph_version() {
  local install_bin_dir="$1"
  local version="$2"
  [[ -n "$version" ]] || return 0
  run_path_command "$install_bin_dir" sh -c 'printf "%s\n" "$1" > "$2"' sh "$version" "$install_bin_dir/$INSTALLED_VERSION_FILENAME"
}

installed_linph_version() {
  local install_bin_dir="$1"
  local version_path="$install_bin_dir/$INSTALLED_VERSION_FILENAME"
  if [[ ! -x "$install_bin_dir/linph" ]]; then
    return 1
  fi
  if [[ ! -r "$version_path" ]]; then
    return 1
  fi
  extract_linphon_version "$(<"$version_path")"
}

source_linphon_version() {
  initialize_repo_paths
  local path="$REPO_ROOT/tools/psiphon-mg/internal/mg/linph.go"
  if [[ ! -r "$path" ]]; then
    return 1
  fi
  extract_linphon_version "$(<"$path")"
}

latest_linphon_version() {
  local data=""
  if [[ "$BOOTSTRAP_VERSION_URL" == file://* ]]; then
    local path="${BOOTSTRAP_VERSION_URL#file://}"
    [[ -r "$path" ]] || return 1
    data="$(<"$path")"
  else
    data="$(curl --connect-timeout 5 --max-time 15 -fsSL "$BOOTSTRAP_VERSION_URL")" || return 1
  fi
  extract_linphon_version "$data"
}

bootstrap_install_state() {
  local install_bin_dir="$1"
  local installed_version=""
  local latest_version=""
  if ! installed_version="$(installed_linph_version "$install_bin_dir")" || [[ -z "$installed_version" ]]; then
    if [[ -x "$install_bin_dir/linph" ]]; then
      printf '%s\n' 'update'
      return 0
    fi
    printf '%s\n' 'missing'
    return 0
  fi
  if latest_version="$(latest_linphon_version 2>/dev/null)" && [[ -n "$latest_version" ]]; then
    if [[ "$installed_version" == "$latest_version" ]]; then
      printf '%s\n' 'current'
    else
      printf '%s\n' 'update'
    fi
    return 0
  fi
  local source_version=""
  if source_version="$(source_linphon_version)" && [[ -n "$source_version" && "$installed_version" == "$source_version" ]]; then
    printf '%s\n' 'current'
    return 0
  fi
  printf '%s\n' 'update'
}

parse_bootstrap_install_bin_dir() {
  local install_bin_dir="$DEFAULT_INSTALL_BIN_DIR"
  local arg=""

  while [[ $# -gt 0 ]]; do
    arg="$1"
    case "$arg" in
      --install-bin-dir)
        if [[ $# -lt 2 ]]; then
          return 64
        fi
        install_bin_dir="$2"
        shift 2
        ;;
      --help|-h)
        return 65
        ;;
      *)
        return 66
        ;;
    esac
  done
  printf '%s\n' "$install_bin_dir"
}

maybe_exit_if_bootstrap_current() {
  local install_bin_dir=""
  local install_state=""

  if ! install_bin_dir="$(parse_bootstrap_install_bin_dir "$@")"; then
    return 0
  fi
  install_state="$(bootstrap_install_state "$install_bin_dir")"
  if [[ "$install_state" == 'current' ]]; then
    printf '已是最新版本\n'
    exit 0
  fi
}

preflight_build_dependencies() {
  local allow_auto_install="${1:-0}"
  local require_privileged_install="${2:-1}"
  local prompt_auto_install="${3:-1}"
  local reply=""
  local manager=""
  local package=""
  local install_command=""

  if ! command_exists go; then
    if [[ "$allow_auto_install" -eq 1 ]]; then
      if ! can_run_privileged_command; then
        lang_eprintf \
          'Go is required to build linph, and automatic installation needs root or sudo. Please install Go manually and rerun.\n' \
          '构建 linph 需要 go，而自动安装需要 root 或 sudo。请先手动安装 Go 后重试。\n'
        exit 1
      fi

      if manager="$(detect_go_package_manager 2>/dev/null)" && package="$(go_package_name_for_manager "$manager" 2>/dev/null)"; then
        install_command="$(describe_go_install_command "$manager" "$package")"
        if [[ "$prompt_auto_install" -eq 1 && -t 0 && -t 1 ]]; then
          read_lang_prompt reply \
            'Go is required to build linph. Install it now with `%s`? [Y/n] ' \
            '构建 linph 需要 go。是否现在用 `%s` 自动安装？[Y/n] ' \
            "$install_command"
        else
          reply="yes"
          lang_printf \
            'Go is required to build linph; installing it with `%s`.\n' \
            '构建 linph 需要 go；现在使用 `%s` 自动安装。\n' \
            "$install_command"
        fi
        case "$reply" in
          ""|y|Y|yes|YES|Yes)
            if ! run_go_auto_install "$manager" "$package"; then
              lang_eprintf \
                'Automatic Go installation failed. Please install Go manually and rerun.\n' \
                'Go 自动安装失败。请先手动安装 Go 后重试。\n'
              exit 1
            fi
            hash -r
            if ! command_exists go; then
              lang_eprintf \
                'Go is still not available after automatic installation. Please check PATH and rerun.\n' \
                '自动安装后仍未找到 Go。请检查 PATH 后重试。\n'
              exit 1
            fi
            ;;
          *)
            lang_eprintf \
              'Go is required to build linph. Please install it manually and rerun.\n' \
              '构建 linph 需要 go。请先手动安装后重试。\n'
            exit 1
            ;;
        esac
      else
        lang_eprintf \
          'Go is required to build linph. Automatic installation is not supported on this system, so please install Go manually and rerun.\n' \
          '构建 linph 需要 go。当前系统不支持脚本自动安装，请先手动安装 Go 后重试。\n'
        exit 1
      fi
    else
      lang_eprintf "required command not found: %s\n" "缺少必需命令：%s\n" 'go'
      lang_eprintf \
        'install.sh builds linph locally, so please install Go first and rerun.\n' \
        'install.sh 会在本地构建 linph，请先安装 Go 后重试。\n'
      exit 1
    fi
  fi

  if [[ "$require_privileged_install" -ne 0 && "$(id -u)" -ne 0 ]] && ! command_exists sudo; then
    lang_eprintf \
      'Non-root install requires sudo to write into %s and %s. Please install sudo or rerun as root.\n' \
      '非 root 安装需要 sudo 才能写入 %s 和 %s。请安装 sudo，或改为 root 身份重试。\n' \
      "$DEFAULT_INSTALL_BIN_DIR" "$DEFAULT_INSTALL_CONFIG_DIR"
    exit 1
  fi
}

cancel_install() {
  lang_printf "\ninstall cancelled\n" "\n已取消安装\n"
  exit 0
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

choose_language() {
  local reply=""
  while true; do
    read_with_prompt reply "english/中文 [english]: "
    reply="${reply//[[:space:]]/}"
    case "$reply" in
      ""|english|English|ENGLISH|en|EN|1)
        SCRIPT_LANG="en"
        return 0
        ;;
      中文|zh|ZH|cn|CN|2)
        SCRIPT_LANG="zh"
        return 0
        ;;
    esac
    printf 'Please enter english or 中文. / 请输入 english 或 中文。\n' >&2
  done
}

describe_memory_source() {
  case "$EFFECTIVE_MEMORY_SOURCE" in
    cgroup)
      lang_printf 'cgroup limit' 'cgroup 限制'
      ;;
    host)
      lang_printf 'host memory' '主机内存'
      ;;
    *)
      lang_printf 'safe fallback' '安全兜底'
      ;;
  esac
}

compute_slot_cap_from_memory_mib() {
  local total_mib="$1"
  local cap=0
  if [[ -z "$total_mib" || "$total_mib" -le 0 ]]; then
    printf '%s\n' "$INSTALLED_SLOT_FALLBACK_CAP"
    return 0
  fi
  cap=$(( total_mib / INSTALLED_SLOT_MEMORY_SCALE_DIV ))
  if (( cap < INSTALLED_SLOT_FALLBACK_CAP )); then
    cap=$INSTALLED_SLOT_FALLBACK_CAP
  fi
  if (( cap > INSTALLED_SLOT_HARD_LIMIT )); then
    cap=$INSTALLED_SLOT_HARD_LIMIT
  fi
  printf '%s\n' "$cap"
}

read_proc_meminfo_mib() {
  local key=""
  local value=""
  local unit=""
  if [[ ! -r /proc/meminfo ]]; then
    return 1
  fi
  while read -r key value unit; do
    if [[ "$key" == "MemTotal:" && "$value" =~ ^[0-9]+$ ]]; then
      printf '%s\n' "$(( value / 1024 ))"
      return 0
    fi
  done < /proc/meminfo
  return 1
}

read_cgroup_limit_bytes() {
  local path=""
  local raw=""
  for path in \
    /sys/fs/cgroup/memory.max \
    /sys/fs/cgroup/memory/memory.limit_in_bytes \
    /sys/fs/cgroup/memory.limit_in_bytes
  do
    [[ -r "$path" ]] || continue
    if ! IFS= read -r raw < "$path"; then
      continue
    fi
    raw="${raw//$'\r'/}"
    raw="${raw//[[:space:]]/}"
    if [[ -z "$raw" || "$raw" == "max" ]]; then
      continue
    fi
    if [[ "$raw" =~ ^[0-9]+$ ]]; then
      if (( raw == 0 || raw >= UNLIMITED_BYTES_THRESHOLD )); then
        continue
      fi
      printf '%s\n' "$raw"
      return 0
    fi
  done
  return 1
}

detect_effective_memory() {
  local host_mib=0
  local cgroup_bytes=""
  local cgroup_mib=0

  if host_mib="$(read_proc_meminfo_mib 2>/dev/null)"; then
    :
  else
    host_mib=0
  fi

  if cgroup_bytes="$(read_cgroup_limit_bytes 2>/dev/null)"; then
    cgroup_mib=$(( cgroup_bytes / 1024 / 1024 ))
    if (( cgroup_mib == 0 )); then
      cgroup_mib=1
    fi
  else
    cgroup_mib=0
  fi

  if (( cgroup_mib > 0 && host_mib > 0 && cgroup_mib < host_mib )); then
    EFFECTIVE_MEMORY_MIB="$cgroup_mib"
    EFFECTIVE_MEMORY_SOURCE="cgroup"
  elif (( host_mib > 0 )); then
    EFFECTIVE_MEMORY_MIB="$host_mib"
    EFFECTIVE_MEMORY_SOURCE="host"
  elif (( cgroup_mib > 0 )); then
    EFFECTIVE_MEMORY_MIB="$cgroup_mib"
    EFFECTIVE_MEMORY_SOURCE="cgroup"
  else
    EFFECTIVE_MEMORY_MIB=0
    EFFECTIVE_MEMORY_SOURCE="fallback"
  fi

  EFFECTIVE_SLOT_CAP="$(compute_slot_cap_from_memory_mib "$EFFECTIVE_MEMORY_MIB")"
  if (( SCRIPT_FORCE_UNLOCK != 0 )); then
    EFFECTIVE_SLOT_CAP="$INSTALLED_SLOT_HARD_LIMIT"
  fi
}

print_slot_cap_guidance() {
  if (( SCRIPT_FORCE_UNLOCK != 0 )); then
    lang_printf \
      'Memory-based slot cap is bypassed with --fk. Up to %s slots are unlocked.\n' \
      '已使用 --fk 绕过内存上限，最多解锁到 %s 个槽位。\n' \
      "$INSTALLED_SLOT_HARD_LIMIT"
    return 0
  fi

  if (( EFFECTIVE_MEMORY_MIB > 0 )); then
    local source_label=""
    source_label="$(describe_memory_source)"
    lang_printf \
      'Detected %s MiB effective memory from %s. Current slot cap: %s. Use --fk to unlock up to %s.\n' \
      '检测到有效内存 %s MiB（来源：%s）。当前槽位上限：%s。可用 --fk 解锁到 %s。\n' \
      "$EFFECTIVE_MEMORY_MIB" "$source_label" "$EFFECTIVE_SLOT_CAP" "$INSTALLED_SLOT_HARD_LIMIT"
    return 0
  fi

  lang_printf \
    'Could not detect effective memory. Using safe slot cap %s. Use --fk to unlock up to %s.\n' \
    '未能检测到有效内存，当前使用安全上限 %s。可用 --fk 解锁到 %s。\n' \
    "$EFFECTIVE_SLOT_CAP" "$INSTALLED_SLOT_HARD_LIMIT"
}

choose_install_mode() {
  local __outvar="$1"
  local choice=""

  while true; do
    read_lang_prompt choice \
      'Install mode: [1] single-port, [2] multi-port [1]: ' \
      '安装模式：[1] 单端口，[2] 多端口 [1]： '
    case "$choice" in
      ""|1|single|single-port)
        printf -v "$__outvar" '%s' 'single'
        return 0
        ;;
      2|multi|multi-port)
        printf -v "$__outvar" '%s' 'multi'
        return 0
        ;;
    esac
    lang_eprintf \
      'please choose 1 for single-port or 2 for multi-port\n' \
      '请输入 1（单端口）或 2（多端口）\n'
  done
}

prompt_positive_int() {
  local __outvar="$1"
  local english_prompt="$2"
  local chinese_prompt="$3"
  shift 3
  local input=""

  while true; do
    read_lang_prompt input "$english_prompt" "$chinese_prompt" "$@"
    input="${input//[[:space:]]/}"
    case "$input" in
      *[!0-9]*|"")
        lang_eprintf 'please enter a positive integer\n' '请输入正整数\n'
        ;;
      *)
        if [[ "$input" -gt 0 ]]; then
          printf -v "$__outvar" '%s' "$input"
          return 0
        fi
        lang_eprintf 'please enter a positive integer\n' '请输入正整数\n'
        ;;
    esac
  done
}

prompt_port() {
  local __outvar="$1"
  local english_prompt="$2"
  local chinese_prompt="$3"
  shift 3
  local port=""

  while true; do
    prompt_positive_int port "$english_prompt" "$chinese_prompt" "$@"
    if (( port >= 1 && port <= 65535 )); then
      printf -v "$__outvar" '%s' "$port"
      return 0
    fi
    lang_eprintf 'port must be between 1 and 65535\n' '端口必须在 1 到 65535 之间\n'
  done
}

prompt_slot_count() {
  local __outvar="$1"
  local limit="$2"
  local count=""

  while true; do
    prompt_positive_int count \
      'Enter slot count (1-%s): ' \
      '请输入槽位数量（1-%s）： ' \
      "$limit"
    if (( count <= limit )); then
      printf -v "$__outvar" '%s' "$count"
      return 0
    fi
    if (( SCRIPT_FORCE_UNLOCK != 0 )); then
      lang_eprintf 'slot count must be %s or less\n' '槽位数量不能超过 %s\n' "$limit"
    else
      lang_eprintf \
        'slot count exceeds the current memory-based cap %s (use --fk to unlock up to %s)\n' \
        '槽位数量超过当前基于内存的上限 %s（可用 --fk 解锁到 %s）\n' \
        "$limit" "$INSTALLED_SLOT_HARD_LIMIT"
    fi
  done
}

resolve_plan_port() {
  local candidate="$1"
  local used_name="$2"
  local -n used_ref="$used_name"

  while (( candidate <= 65535 )); do
    if [[ -z "${used_ref[$candidate]+x}" ]]; then
      used_ref[$candidate]=1
      printf '%s\n' "$candidate"
      return 0
    fi
    ((candidate++))
  done
  return 1
}

derive_port_plan() {
  local slot_count="$1"
  local http_base="$2"
  local socks_base="$3"
  local -A used_ports=()
  local index=0
  local resolved_http=""
  local resolved_socks=""

  DERIVED_HTTP_PORTS=()
  DERIVED_SOCKS_PORTS=()

  for ((index=0; index<slot_count; index++)); do
    resolved_http="$(resolve_plan_port $((http_base + index)) used_ports)" || return 1
    resolved_socks="$(resolve_plan_port $((socks_base + index)) used_ports)" || return 1
    DERIVED_HTTP_PORTS+=("$resolved_http")
    DERIVED_SOCKS_PORTS+=("$resolved_socks")
  done
  return 0
}

render_slot_plan() {
  local slot_count="$1"
  local regions_csv="$2"
  local -a regions=()
  local index=0
  DERIVED_SLOT_LINES=()
  IFS=',' read -r -a regions <<< "$regions_csv"
  for ((index=0; index<slot_count; index++)); do
    DERIVED_SLOT_LINES+=("slot-$(printf '%03d' $((index + 1))) region=${regions[$index]} http=${DERIVED_HTTP_PORTS[$index]} socks=${DERIVED_SOCKS_PORTS[$index]}")
  done
}

prompt_regions_csv() {
  local __outvar="$1"
  local slot_count="$2"
  local english_prompt="$3"
  local chinese_prompt="$4"
  local input=""
  local -a selected=()
  local -a raw_parts=()
  local region=""
  local part=""
  local invalid=0

  while true; do
    print_supported_regions
    read_lang_prompt input "$english_prompt" "$chinese_prompt"

    selected=()
    raw_parts=()
    invalid=0
    IFS=',' read -r -a raw_parts <<< "$input"
    for part in "${raw_parts[@]}"; do
      region="$(normalize_region_token "$part")"
      [[ -z "$region" ]] && continue
      if ! region_is_supported "$region"; then
        lang_eprintf 'unsupported region: %s\n' '不支持的地区：%s\n' "$region"
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
      lang_eprintf 'please enter at least %s supported region(s)\n' '请至少输入 %s 个受支持地区\n' "$slot_count"
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
    printf -v "$__outvar" '%s' "$joined"
    return 0
  done
}

collect_ports_for_slots() {
  local slot_count="$1"
  local -n __http_outvar="$2"
  local -n __socks_outvar="$3"
  local http_port=""
  local socks_port=""

  while true; do
    prompt_port http_port 'Enter starting HTTP port: ' '请输入起始 HTTP 端口： '
    prompt_port socks_port 'Enter starting SOCKS5 port: ' '请输入起始 SOCKS5 端口： '
    if derive_port_plan "$slot_count" "$http_port" "$socks_port"; then
      __http_outvar="$http_port"
      __socks_outvar="$socks_port"
      COLLECTED_START_HTTP_PORT="$http_port"
      COLLECTED_START_SOCKS_PORT="$socks_port"
      return 0
    fi
    lang_eprintf \
      'the chosen starting ports leave no valid room for %s slot(s); please choose smaller ports\n' \
      '当前起始端口无法为 %s 个槽位推导出有效端口，请改用更小的端口\n' \
      "$slot_count"
  done
}

resolve_default_binary() {
  local candidate
  for candidate in \
    "$REPO_ROOT/psiphon-tunnel-core-x86_64" \
    "$REPO_ROOT/archive/psiphon-tunnel-core-x86_64" \
    "$DEFAULT_RUNTIME_ROOT/bin/psiphon-tunnel-core-x86_64"
  do
    if [[ -f "$candidate" && ! -L "$candidate" ]]; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done
  return 1
}

prompt_for_existing_file() {
  local __outvar="$1"
  local english_prompt="$2"
  local chinese_prompt="$3"
  local default_value="${4-}"
  local input=""

  while true; do
    if [[ -n "$default_value" ]]; then
      read_lang_prompt input "$english_prompt [%s]: " "$chinese_prompt [%s]： " "$default_value"
      input="${input:-$default_value}"
    else
      read_lang_prompt input "$english_prompt: " "$chinese_prompt： "
    fi

    if [[ -e "$input" ]]; then
      if [[ -f "$input" && ! -L "$input" ]]; then
        printf -v "$__outvar" '%s' "$input"
        return 0
      fi
      lang_eprintf 'file must be a regular file: %s\n' '文件必须是普通文件：%s\n' "$input"
      continue
    fi

    lang_eprintf 'file not found: %s\n' '文件不存在：%s\n' "$input"
  done
}

confirm_install() {
  local reply=""
  read_lang_prompt reply 'Continue install? [Y/n] ' '继续安装？[Y/n] '
  case "$reply" in
    ""|y|Y|yes|YES|Yes)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

collect_unmanaged_conflicts() {
  PRECHECK_CONFLICTS=()
  local manifest_path="$DEFAULT_INSTALL_BIN_DIR/linph-install-manifest.json"
  if [[ -f "$manifest_path" ]]; then
    return 1
  fi

  local targets=(
    "$DEFAULT_INSTALL_BIN_DIR/linph"
    "$DEFAULT_INSTALL_BIN_DIR/psiphon"
    "$DEFAULT_INSTALL_BIN_DIR/plinstaller2"
    "$DEFAULT_INSTALL_BIN_DIR/pluninstaller"
    "$DEFAULT_INSTALL_CONFIG_DIR/psiphon-tunnel-core-x86_64"
    "$DEFAULT_INSTALL_CONFIG_DIR/psiphon.config"
  )
  local path=""
  for path in "${targets[@]}"; do
    if [[ -e "$path" ]]; then
      PRECHECK_CONFLICTS+=("$path")
    fi
  done
  [[ "${#PRECHECK_CONFLICTS[@]}" -gt 0 ]]
}

maybe_enable_force_for_conflicts() {
  local reply=""
  FORCE_INSTALL_FLAG=0
  if ! collect_unmanaged_conflicts; then
    return 0
  fi

  lang_printf 'Existing install targets were found before install:\n' '安装前检测到已有安装目标：\n'
  local path=""
  for path in "${PRECHECK_CONFLICTS[@]}"; do
    printf '  - %s\n' "$path"
  done
  read_lang_prompt reply \
    'Enable --force to overwrite these unmanaged paths? [y/N] ' \
    '是否启用 --force 覆盖这些未托管路径？[y/N] '
  case "$reply" in
    y|Y|yes|YES|Yes)
      FORCE_INSTALL_FLAG=1
      ;;
    *)
      lang_eprintf \
        'install may still fail later on unmanaged-path checks if these files are not from linph\n' \
        '如果这些文件不是 linph 管理的，后续安装仍可能在未托管路径检查阶段失败\n'
      ;;
  esac
}

detect_legacy_psiphon_service() {
  SERVICE_WARNING_STATE=""
  if ! command -v systemctl >/dev/null 2>&1; then
    return 1
  fi
  if systemctl is-active --quiet psiphon.service >/dev/null 2>&1; then
    SERVICE_WARNING_STATE="active"
    return 0
  fi
  if systemctl is-enabled psiphon.service >/dev/null 2>&1; then
    SERVICE_WARNING_STATE="enabled"
    return 0
  fi
  return 1
}

print_service_warning() {
  case "$SERVICE_WARNING_STATE" in
    active)
      lang_printf \
        'Warning: detected active psiphon.service. Stop/disable it first if install or startup conflicts occur.\n' \
        '警告：检测到正在运行的 psiphon.service。如遇安装或启动冲突，请先停止/禁用它。\n'
      ;;
    enabled)
      lang_printf \
        'Warning: detected enabled psiphon.service. It may conflict with installed linph startup.\n' \
        '警告：检测到已启用的 psiphon.service，它可能与 linph 已安装实例启动冲突。\n'
      ;;
  esac
}


print_bootstrap_help() {
  cat <<'EOF'
Usage:
  bash ./install.sh [--install-bin-dir PATH]
  bash ./install.sh --legacy-full-install [install options]
  bash ./install.sh --help

Default behavior bootstraps only the linph command. It builds linph from this
checkout, installs it into the selected bin directory, and prints the next step:
  linph install

If Go is missing, the bootstrap path attempts to install the distro Go package
first, for example golang-go on Debian/Ubuntu apt systems.
When linph is already installed, running this script checks for updates first;
if no newer version is available it prints 已是最新版本 and exits.

Options:
  --install-bin-dir PATH      Install linph here (default: /usr/local/bin).
  --legacy-full-install       Run the previous full interactive/system install flow.
  --help                      Show this message.

The curl-pipe/bootstrap path is a convenience path, not the cryptographically
trusted install path. Use reviewed local sources or a verified release package
when authenticity matters.
EOF
}

collect_restart_schedule_arg() {
  local __outvar="$1"
  local reply=""
  local hours=""

  if [[ ! -t 0 || ! -t 1 ]]; then
    printf -v "$__outvar" '%s' '0'
    return 0
  fi

  while true; do
    read_lang_prompt reply \
      'Enable periodic restart to refresh IP? [y/N] ' \
      '是否启用定期重启以重新获取 IP？[y/N] '
    case "$reply" in
      y|Y|yes|YES|Yes)
        while true; do
          read_lang_prompt hours \
            'Restart interval in hours (1-168): ' \
            '重启间隔，单位小时（1-168）： '
          hours="${hours//[[:space:]]/}"
          if [[ "$hours" =~ ^[0-9]+$ ]] && (( hours >= 1 && hours <= 168 )); then
            printf -v "$__outvar" '%s' "$hours"
            return 0
          fi
          lang_eprintf 'hours must be between 1 and 168\n' '小时数必须在 1 到 168 之间\n'
        done
        ;;
      ""|n|N|no|NO|No)
        printf -v "$__outvar" '%s' '0'
        return 0
        ;;
    esac
    lang_eprintf 'please answer y or n\n' '请输入 y 或 n\n'
  done
}

install_linph_bootstrap() {
  local install_bin_dir="$DEFAULT_INSTALL_BIN_DIR"
  local arg=""
  local install_state=""
  local source_version=""

  while [[ $# -gt 0 ]]; do
    arg="$1"
    case "$arg" in
      --install-bin-dir)
        if [[ $# -lt 2 ]]; then
          printf '%s\n' '--install-bin-dir requires a value' >&2
          return 64
        fi
        install_bin_dir="$2"
        shift 2
        ;;
      --help|-h)
        print_bootstrap_help
        return 0
        ;;
    *)
      printf 'unknown bootstrap option: %s\n' "$arg" >&2
      print_bootstrap_help >&2
      return 64
      ;;
    esac
  done

  install_state="$(bootstrap_install_state "$install_bin_dir")"
  if [[ "$install_state" == 'current' ]]; then
    printf '已是最新版本\n'
    return 0
  fi
  if [[ "$install_state" == 'update' ]]; then
    printf 'detected installed linph update; updating %s/linph\n' "$install_bin_dir"
  fi

  initialize_repo_paths

  if path_requires_privilege "$install_bin_dir"; then
    preflight_build_dependencies 1 1 0
  else
    preflight_build_dependencies 1 0 0
  fi
  build_linph
  source_version="$(source_linphon_version || true)"
  run_path_command "$install_bin_dir" mkdir -p "$install_bin_dir"
  run_path_command "$install_bin_dir" cp "$LINPH_BIN" "$install_bin_dir/linph"
  run_path_command "$install_bin_dir" chmod 0755 "$install_bin_dir/linph"
  write_installed_linph_version "$install_bin_dir" "$source_version"

  printf 'installed linph to %s\n' "$install_bin_dir/linph"
  printf 'next step: linph install\n'
  printf 'provider setup: linph psi set [options]\n'
}

build_linph() {
  initialize_repo_paths
  ensure_command go
  mkdir -p "$(dirname "$LINPH_BIN")"
  (
    cd "$REPO_ROOT/tools/psiphon-mg"
    go build -o ../../tools/psiphon-mg/bin/linph ./cmd/linph
  )
}

run_install() {
  local -a install_args=("$@")

  initialize_repo_paths

  if [[ "$(id -u)" -ne 0 ]]; then
    ensure_command sudo
    exec sudo env PSIPHON_MG_REPO_ROOT="$REPO_ROOT" "$LINPH_BIN" install "${install_args[@]}"
  fi

  exec env PSIPHON_MG_REPO_ROOT="$REPO_ROOT" "$LINPH_BIN" install "${install_args[@]}"
}

run_legacy_install() {
  local -a install_args=("$@")

  initialize_repo_paths

  install_args+=(--start)

  if [[ "$(id -u)" -ne 0 ]]; then
    ensure_command sudo
    exec sudo env PSIPHON_MG_REPO_ROOT="$REPO_ROOT" "$LINPH_BIN" install "${install_args[@]}"
  fi

  exec env PSIPHON_MG_REPO_ROOT="$REPO_ROOT" "$LINPH_BIN" install "${install_args[@]}"
}

print_slot_plan_summary() {
  local line=""
  for line in "${DERIVED_SLOT_LINES[@]}"; do
    printf '  %s\n' "$line"
  done
}

run_interactive_install() {
  local force_unlock="${1:-0}"
  local detected_binary=""
  local binary_path=""
  local config_path="$DEFAULT_BASE_CONFIG"
  local install_reply=""
  local install_mode=""
  local slot_count="1"
  local http_port=""
  local socks_port=""
  local regions_csv=""
  local restart_every_hours="0"
  local -a install_args=()

  initialize_repo_paths

  SCRIPT_FORCE_UNLOCK="$force_unlock"
  SUPPORTED_REGIONS_CSV="$(load_supported_regions_csv)"
  choose_language
  detect_effective_memory
  detected_binary="$(resolve_default_binary || true)"

  lang_printf 'Linphon interactive install\n' 'Linphon 交互式安装\n'
  read_lang_prompt install_reply 'Install Linphon now? [Y/n] ' '现在安装 Linphon 吗？[Y/n] '
  case "$install_reply" in
    ""|y|Y|yes|YES|Yes)
      ;;
    *)
      cancel_install
      ;;
  esac

  preflight_build_dependencies 1
  print_slot_cap_guidance
  choose_install_mode install_mode

  if [[ "$install_mode" == 'single' ]]; then
    slot_count=1
    collect_ports_for_slots "$slot_count" http_port socks_port
    prompt_regions_csv regions_csv "$slot_count" 'Enter region code' '请输入地区代码'
  else
    prompt_slot_count slot_count "$EFFECTIVE_SLOT_CAP"
    collect_ports_for_slots "$slot_count" http_port socks_port
    prompt_regions_csv regions_csv "$slot_count" 'Enter comma-separated regions' '请输入逗号分隔的地区列表'
  fi
  http_port="$COLLECTED_START_HTTP_PORT"
  socks_port="$COLLECTED_START_SOCKS_PORT"
  render_slot_plan "$slot_count" "$regions_csv"
  collect_restart_schedule_arg restart_every_hours

  if [[ -n "$detected_binary" ]]; then
    lang_printf 'Detected tunnel-core: %s\n' '检测到 tunnel-core：%s\n' "$detected_binary"
    prompt_for_existing_file binary_path 'Tunnel-core path (press Enter to use detected value)' 'Tunnel-core 路径（直接回车使用检测值）' "$detected_binary"
  else
    lang_printf 'No reviewed tunnel-core was found in the default repo locations.\n' '默认仓库位置未找到已审核的 tunnel-core。\n'
    prompt_for_existing_file binary_path 'Enter tunnel-core path' '请输入 tunnel-core 路径'
  fi

  if [[ ! -f "$config_path" ]]; then
    lang_eprintf 'Default base config not found: %s\n' '默认基础配置不存在：%s\n' "$config_path"
    prompt_for_existing_file config_path 'Enter base config path' '请输入基础配置路径'
  fi

  maybe_enable_force_for_conflicts
  detect_legacy_psiphon_service || true

  lang_printf '\nInstall summary:\n' '\n安装摘要：\n'
  lang_printf '  mode              -> %s\n' '  模式              -> %s\n' "$install_mode"
  lang_printf '  slot count        -> %s\n' '  槽位数量          -> %s\n' "$slot_count"
  lang_printf '  start HTTP port   -> %s\n' '  起始 HTTP 端口    -> %s\n' "$http_port"
  lang_printf '  start SOCKS port  -> %s\n' '  起始 SOCKS 端口   -> %s\n' "$socks_port"
  lang_printf '  regions           -> %s\n' '  地区              -> %s\n' "$regions_csv"
  lang_printf '  slot cap          -> %s\n' '  槽位上限          -> %s\n' "$EFFECTIVE_SLOT_CAP"
  if (( EFFECTIVE_MEMORY_MIB > 0 )); then
    lang_printf '  effective memory  -> %s MiB (%s)\n' '  有效内存          -> %s MiB（%s）\n' "$EFFECTIVE_MEMORY_MIB" "$(describe_memory_source)"
  else
    lang_printf '  effective memory  -> fallback\n' '  有效内存          -> 兜底值\n'
  fi
  lang_printf '  --fk override     -> %s\n' '  --fk 解锁         -> %s\n' "$([[ "$SCRIPT_FORCE_UNLOCK" -eq 0 ]] && printf 'off' || printf 'on')"
  lang_printf '  --force overwrite -> %s\n' '  --force 覆盖      -> %s\n' "$([[ "$FORCE_INSTALL_FLAG" -eq 0 ]] && printf 'off' || printf 'on')"
  if (( restart_every_hours > 0 )); then
    lang_printf '  periodic restart  -> every %s hour(s)\n' '  定期重启          -> 每 %s 小时\n' "$restart_every_hours"
  else
    lang_printf '  periodic restart  -> disabled\n' '  定期重启          -> 不启用\n'
  fi
  lang_printf '  linph and aliases -> %s\n' '  linph 与别名      -> %s\n' "$DEFAULT_INSTALL_BIN_DIR"
  lang_printf '  psiphon assets    -> %s\n' '  psiphon 资源      -> %s\n' "$DEFAULT_INSTALL_CONFIG_DIR"
  lang_printf '  tunnel-core       -> %s\n' '  tunnel-core       -> %s\n' "$binary_path"
  lang_printf '  base config       -> %s\n' '  基础配置          -> %s\n' "$config_path"
  lang_printf '  derived slots:\n' '  推导后的槽位：\n'
  print_slot_plan_summary
  print_service_warning

  if ! confirm_install; then
    cancel_install
  fi

  build_linph
  install_args+=(--base-config "$config_path")
  install_args+=(--installed-slot-count "$slot_count")
  install_args+=(--installed-http-port "$http_port")
  install_args+=(--installed-socks-port "$socks_port")
  install_args+=(--installed-regions "$regions_csv")
  install_args+=(--restart-every-hours "$restart_every_hours")
  install_args+=(--binary "$binary_path")
  if (( SCRIPT_FORCE_UNLOCK != 0 )); then
    install_args+=(--fk)
  fi
  if (( FORCE_INSTALL_FLAG != 0 )); then
    install_args+=(--force)
  fi
  run_legacy_install "${install_args[@]}"
}

run_install_help() {
  initialize_repo_paths
  preflight_build_dependencies 0 0
  build_linph
  exec env PSIPHON_MG_REPO_ROOT="$REPO_ROOT" "$LINPH_BIN" install "$@"
}

main() {
  local interactive_force_unlock=0
  local legacy_full_install=0
  local bootstrap_args=()
  local arg=""

  for arg in "$@"; do
    case "$arg" in
      --legacy-full-install)
        legacy_full_install=1
        ;;
      *)
        bootstrap_args+=("$arg")
        ;;
    esac
  done
  set -- "${bootstrap_args[@]}"

  if [[ "$legacy_full_install" -eq 1 ]]; then
    if [[ $# -eq 1 && ( "$1" == '--help' || "$1" == '-h' ) ]]; then
      run_install_help "$@"
      return 0
    fi

    if [[ $# -eq 1 && "$1" == '--fk' && -t 0 && -t 1 ]]; then
      interactive_force_unlock=1
      set --
    fi

    if [[ $# -eq 0 && -t 0 && -t 1 ]]; then
      run_interactive_install "$interactive_force_unlock"
    fi

    preflight_build_dependencies 0 1
    build_linph
    run_legacy_install "$@"
    return 0
  fi

  if [[ $# -eq 1 && ( "$1" == '--help' || "$1" == '-h' ) ]]; then
    print_bootstrap_help
    exit 0
  fi

  maybe_exit_if_bootstrap_current "$@"
  install_linph_bootstrap "$@"
}

main "$@"
