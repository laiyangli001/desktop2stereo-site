#!/usr/bin/env bash
set -Eeuo pipefail

BACKUP_DIR="${1:?backup directory is required}"
SHA="${2:?commit SHA is required}"
POSTGRES_USER="${D2S_POSTGRES_USER:-d2s}"
POSTGRES_DB="${D2S_POSTGRES_DB:-new-api}"
COS_URI="${D2S_BACKUP_COS_URI:-}"
COS_SOURCE_REGION="${D2S_BACKUP_SOURCE_REGION:-ap-shanghai}"
COS_REGION="${D2S_BACKUP_COS_REGION:-}"
COS_ENDPOINT=""
COSCLI="${D2S_BACKUP_COSCLI:-/usr/local/bin/coscli}"

if [[ ! "$SHA" =~ ^[[:xdigit:]]{40}$ ]]; then
  echo "commit SHA must contain exactly 40 hexadecimal characters" >&2
  exit 2
fi

if [[ -n "$COS_URI" ]]; then
  if [[ "$COS_URI" != cos://*/* || "$COS_URI" == */ ]]; then
    echo "D2S_BACKUP_COS_URI must be a cos:// bucket/prefix without a trailing slash" >&2
    exit 2
  fi
  if [[ -z "$COS_REGION" || "$COS_REGION" == "$COS_SOURCE_REGION" ]]; then
    echo "D2S_BACKUP_COS_REGION is required and must differ from D2S_BACKUP_SOURCE_REGION" >&2
    exit 2
  fi
  if [[ ! "$COS_REGION" =~ ^[a-z]+-[a-z0-9-]+$ || ! "$COS_SOURCE_REGION" =~ ^[a-z]+-[a-z0-9-]+$ ]]; then
    echo "COS region values must be Tencent Cloud region identifiers" >&2
    exit 2
  fi
  COS_ENDPOINT="cos.$COS_REGION.myqcloud.com"
  if [[ ! -x "$COSCLI" ]]; then
    echo "COSCLI is missing or not executable: $COSCLI" >&2
    exit 2
  fi
fi

umask 077
mkdir -p "$BACKUP_DIR"
chmod 700 "$BACKUP_DIR"
timestamp="$(date -u +%Y%m%d-%H%M%S)"
temporary="$BACKUP_DIR/.pre-$SHA-$timestamp.dump.tmp"
target="$BACKUP_DIR/pre-$SHA-$timestamp.dump"

if ! docker exec postgres pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc > "$temporary"; then
  rm -f -- "$temporary"
  exit 1
fi
if [[ "$(stat -c %s "$temporary")" -le 1000 ]]; then
  rm -f -- "$temporary"
  echo "database backup is unexpectedly small" >&2
  exit 1
fi
mv -- "$temporary" "$target"
(
  cd "$BACKUP_DIR"
  sha256sum "$(basename "$target")" > "$(basename "$target").sha256"
)
echo "database backup created: $target"

if [[ -n "$COS_URI" ]]; then
  object_prefix="${COS_URI%/}/$(basename "$target")"
  "$COSCLI" cp "$target" "$object_prefix" --endpoint "$COS_ENDPOINT" --disable-checksum=false --encryption-type SSE-COS --server-side-encryption AES256
  "$COSCLI" cp "$target.sha256" "$object_prefix.sha256" --endpoint "$COS_ENDPOINT" --disable-checksum=false --encryption-type SSE-COS --server-side-encryption AES256
  "$COSCLI" stat "$object_prefix" --endpoint "$COS_ENDPOINT" >/dev/null
  "$COSCLI" stat "$object_prefix.sha256" --endpoint "$COS_ENDPOINT" >/dev/null
  echo "database backup copied to COS: $object_prefix (region: $COS_REGION)"
else
  echo "COS backup upload is not configured"
fi
