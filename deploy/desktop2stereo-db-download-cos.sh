#!/usr/bin/env bash
set -Eeuo pipefail

REMOTE_OBJECT="${1:?COS backup object URI is required}"
BACKUP_DIR="${2:?local backup directory is required}"
COSCLI="${D2S_BACKUP_COSCLI:-/usr/local/bin/coscli}"
COS_REGION="${D2S_BACKUP_COS_REGION:-}"

if [[ ! "$REMOTE_OBJECT" =~ ^cos://[A-Za-z0-9.-]+/.+\.dump$ ]]; then
  echo "backup object must be a COS URI ending in .dump" >&2
  exit 2
fi
FILENAME="${REMOTE_OBJECT##*/}"
if [[ ! "$FILENAME" =~ ^pre-[[:xdigit:]]{40}-[0-9]{8}-[0-9]{6}\.dump$ ]]; then
  echo "backup object filename does not match the generated backup format" >&2
  exit 2
fi
if [[ ! "$COS_REGION" =~ ^[a-z]+-[a-z0-9-]+$ ]]; then
  echo "D2S_BACKUP_COS_REGION must contain the backup bucket region" >&2
  exit 2
fi
COS_ENDPOINT="cos.$COS_REGION.myqcloud.com"
if [[ ! -x "$COSCLI" ]]; then
  echo "COSCLI is missing or not executable: $COSCLI" >&2
  exit 2
fi
for command_name in sha256sum mktemp; do
  command -v "$command_name" >/dev/null || {
    echo "required command is missing: $command_name" >&2
    exit 3
  }
done

umask 077
mkdir -p "$BACKUP_DIR"
chmod 700 "$BACKUP_DIR"
TARGET="$BACKUP_DIR/$FILENAME"
if [[ -e "$TARGET" || -e "$TARGET.sha256" ]]; then
  echo "refusing to overwrite an existing backup: $TARGET" >&2
  exit 4
fi

STAGING_DIR="$(mktemp -d "$BACKUP_DIR/.cos-download.XXXXXX")"
cleanup() {
  if [[ -n "$STAGING_DIR" && -d "$STAGING_DIR" ]]; then
    rm -rf -- "$STAGING_DIR"
  fi
}
trap cleanup EXIT

"$COSCLI" cp "$REMOTE_OBJECT" "$STAGING_DIR/$FILENAME" --endpoint "$COS_ENDPOINT" --disable-checksum=false
"$COSCLI" cp "$REMOTE_OBJECT.sha256" "$STAGING_DIR/$FILENAME.sha256" --endpoint "$COS_ENDPOINT" --disable-checksum=false

if [[ ! -s "$STAGING_DIR/$FILENAME" || ! -s "$STAGING_DIR/$FILENAME.sha256" ]]; then
  echo "downloaded backup or checksum is empty" >&2
  exit 5
fi
mapfile -t MANIFEST_LINES < "$STAGING_DIR/$FILENAME.sha256"
if [[ "${#MANIFEST_LINES[@]}" -ne 1 ]]; then
  echo "backup checksum manifest must contain exactly one line" >&2
  exit 5
fi
read -r EXPECTED_HASH MANIFEST_FILENAME EXTRA <<< "${MANIFEST_LINES[0]}"
MANIFEST_FILENAME="${MANIFEST_FILENAME#\*}"
if [[ ! "$EXPECTED_HASH" =~ ^[[:xdigit:]]{64}$ || "$MANIFEST_FILENAME" != "$FILENAME" || -n "${EXTRA:-}" ]]; then
  echo "backup checksum manifest is invalid" >&2
  exit 5
fi
ACTUAL_HASH="$(sha256sum -- "$STAGING_DIR/$FILENAME" | awk '{print $1}')"
if [[ "$ACTUAL_HASH" != "$EXPECTED_HASH" ]]; then
  echo "downloaded backup SHA-256 does not match its manifest" >&2
  exit 6
fi

mv -- "$STAGING_DIR/$FILENAME" "$TARGET"
mv -- "$STAGING_DIR/$FILENAME.sha256" "$TARGET.sha256"
echo "COS backup downloaded and SHA-256 verified: $TARGET"
