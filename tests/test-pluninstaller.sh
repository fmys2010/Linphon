#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
UNINSTALLER="$REPO_ROOT/pluninstaller"
TEST_ROOT="$REPO_ROOT/.work/test-pluninstaller"
FAKE_BIN="$TEST_ROOT/bin"
RM_LOG="$TEST_ROOT/rm.log"

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
mkdir -p "$FAKE_BIN"

cat > "$FAKE_BIN/rm" <<EOF
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "\$*" >> "$RM_LOG"
EOF
chmod +x "$FAKE_BIN/rm"

PATH="$FAKE_BIN:$PATH" bash "$UNINSTALLER"

assert_eq 2 "$(wc -l < "$RM_LOG" | tr -d '[:space:]')" 'rm call count'
assert_eq '-rf -- /etc/psiphon' "$(sed -n '1p' "$RM_LOG")" 'config dir removal contract'
assert_eq '-f -- /usr/bin/psiphon' "$(sed -n '2p' "$RM_LOG")" 'launcher removal contract'

printf '[test] pluninstaller contract verification passed\n'
