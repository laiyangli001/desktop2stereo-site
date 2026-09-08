#!/usr/bin/env bash
set -Eeuo pipefail

REQUEST_FILE="${1:?request file is required}"
SHA="$(tr -d '[:space:]' < "$REQUEST_FILE")"
rm -f -- "$REQUEST_FILE"
exec /usr/local/sbin/desktop2stereo-docker-update "$SHA"
