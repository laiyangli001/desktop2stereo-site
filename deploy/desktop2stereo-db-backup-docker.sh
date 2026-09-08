#!/usr/bin/env bash
set -Eeuo pipefail

BACKUP_DIR="${1:?backup directory is required}"
SHA="${2:?commit SHA is required}"
POSTGRES_USER="${D2S_POSTGRES_USER:-d2s}"
POSTGRES_DB="${D2S_POSTGRES_DB:-new-api}"
COS_URI="${D2S_BACKUP_COS_URI:-}"
COSCLI="${D2S_BACKUP_COSCLI:-/usr/local/bin/coscli}"

if [[ -n "$COS_URI" ]]; then
  if [[ "$COS_URI" != cos://* || "$COS_URI" == */ ]]; then
    echo "D2S_BACKUP_COS_URI must be a non-empty cos:// prefix without a trailing slash" >&2
    exit 2
  fi
  if [[ ! -x "$COSCLI" ]]; then
    echo "COSCLI is missing or not executable: $COSCLI" >&2
    exit 2
  fi
fi

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

if [[ -n "$COS_URI" ]]; then
  object_prefix="${COS_URI%/}/$(basename "$target")"
  "$COSCLI" cp "$target" "$object_prefix"
  "$COSCLI" cp "$target.sha256" "$object_prefix.sha256"
  "$COSCLI" stat "$object_prefix" >/dev/null
  "$COSCLI" stat "$object_prefix.sha256" >/dev/null
  echo "database backup copied to COS: $object_prefix"
else
  echo "COS backup upload is not configured"
fi
