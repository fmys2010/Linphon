#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LINPH_BIN="$REPO_ROOT/tools/psiphon-mg/bin/linph"

if command -v linph >/dev/null 2>&1; then
  if [[ "$(id -u)" -ne 0 ]]; then
    exec sudo linph uninstall "$@"
  fi
  exec linph uninstall "$@"
fi

mkdir -p "$(dirname "$LINPH_BIN")"
(
  cd "$REPO_ROOT/tools/psiphon-mg"
  go build -o ../../tools/psiphon-mg/bin/linph ./cmd/linph
)

if [[ "$(id -u)" -ne 0 ]]; then
  exec sudo env PSIPHON_MG_REPO_ROOT="$REPO_ROOT" "$LINPH_BIN" uninstall "$@"
fi

exec env PSIPHON_MG_REPO_ROOT="$REPO_ROOT" "$LINPH_BIN" uninstall "$@"
