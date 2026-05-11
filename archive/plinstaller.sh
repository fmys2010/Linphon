#!/usr/bin/env bash
set -euo pipefail

EXIT_DOWNLOAD_DISABLED=66

printf '%s\n' 'Automatic remote download/install is disabled until executable authenticity verification exists.' >&2
printf '%s\n' 'Use reviewed local archive artifacts instead, such as archive/psiphon-tunnel-core-x86_64 with archive/psiphon.sh and ../psiphon.config.' >&2
exit "$EXIT_DOWNLOAD_DISABLED"
