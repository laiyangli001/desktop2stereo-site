#!/usr/bin/env bash
set -Eeuo pipefail

BACKUP_DIR="${1:?backup directory is required}"
SHA="${2:?commit SHA is required}"
POSTGRES_USER="${D2S_POSTGRES_USER:-d2s}"
POSTGRES_DB="${D2S_POSTGRES_DB:-new-api}"

mkdir -p "$BACKUP_DIR"
timestamp="$(date +%Y%m%d-%H%M%S)"
temporary="$BACKUP_DIR/.pre-$SHA-$timestamp.dump.tmp"
target="$BACKUP_DIR/pre-$SHA-$timestamp.dump"

if ! docker exec postgres pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc > "$temporary"; then
  rm -f "$temporary"
  exit 1
fi
if [[ "$(stat -c %s "$temporary")" -le 1000 ]]; then
  rm -f "$temporary"
  echo "database backup is unexpectedly small" >&2
  exit 1
fi
mv "$temporary" "$target"
sha256sum "$target" > "$target.sha256"
echo "database backup created: $target"
