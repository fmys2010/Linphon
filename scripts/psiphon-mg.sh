#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)

GO_BINARY=${PSIPHON_MG_GO_BINARY:-$REPO_ROOT/tools/psiphon-mg/bin/psiphon-mg}
if [ -x "$GO_BINARY" ]; then
  export PSIPHON_MG_REPO_ROOT="$REPO_ROOT"
  exec "$GO_BINARY" "$@"
fi

printf '[psiphon-mg] ERROR: Go binary not found: %s\n' "$GO_BINARY" >&2
printf '[psiphon-mg] ERROR: build it with: (cd tools/psiphon-mg && go build -o ../../tools/psiphon-mg/bin/psiphon-mg ./cmd/psiphon-mg)\n' >&2
exit 127
