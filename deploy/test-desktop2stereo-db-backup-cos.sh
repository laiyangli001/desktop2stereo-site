#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKUP_SCRIPT="$SCRIPT_DIR/desktop2stereo-db-backup-docker.sh"
DOWNLOAD_SCRIPT="$SCRIPT_DIR/desktop2stereo-db-download-cos.sh"
TEST_ROOT="$(mktemp -d)"
MOCK_BIN="$TEST_ROOT/bin"
MOCK_COS_ROOT="$TEST_ROOT/cos"
mkdir -p "$MOCK_BIN" "$MOCK_COS_ROOT"
cleanup() {
  rm -rf -- "$TEST_ROOT"
}
trap cleanup EXIT

cat > "$MOCK_BIN/docker" <<'SH'
#!/usr/bin/env bash
set -Eeuo pipefail
if [[ -n "${DOCKER_MARKER:-}" ]]; then
  touch "$DOCKER_MARKER"
fi
if [[ "$1" == exec && "$2" == postgres && "$3" == pg_dump ]]; then
  dd if=/dev/zero bs=2048 count=1 2>/dev/null
  exit 0
fi
echo "unexpected docker mock call" >&2
exit 1
SH

cat > "$MOCK_BIN/coscli" <<'SH'
#!/usr/bin/env bash
set -Eeuo pipefail
COS_ROOT="${MOCK_COS_ROOT:?MOCK_COS_ROOT is required}"
if [[ -n "${COSCLI_MARKER:-}" ]]; then
  touch "$COSCLI_MARKER"
fi
COMMAND="$1"
shift
case "$COMMAND" in
  cp)
    SOURCE="$1"
    DESTINATION="$2"
    shift 2
    OPTIONS=" $* "
    if [[ "$OPTIONS" != *" --endpoint cos.ap-guangzhou.myqcloud.com "* ]]; then
      echo "COS command must specify the bucket region endpoint" >&2
      exit 7
    fi
    if [[ "$SOURCE" == cos://* ]]; then
      if [[ "$OPTIONS" != *" --disable-checksum=false "* ]]; then
        echo "download must verify full-object CRC64" >&2
        exit 2
      fi
      SOURCE_PATH="$COS_ROOT/${SOURCE#cos://}"
      [[ -f "$SOURCE_PATH" ]] || exit 3
      mkdir -p "$(dirname "$DESTINATION")"
      cp -- "$SOURCE_PATH" "$DESTINATION"
    elif [[ "$DESTINATION" == cos://* ]]; then
      if [[ "$OPTIONS" != *" --disable-checksum=false "* || "$OPTIONS" != *" --encryption-type SSE-COS "* || "$OPTIONS" != *" --server-side-encryption AES256 "* ]]; then
        echo "upload must enforce SSE-COS and full-object CRC64" >&2
        exit 4
      fi
      DESTINATION_PATH="$COS_ROOT/${DESTINATION#cos://}"
      mkdir -p "$(dirname "$DESTINATION_PATH")"
      cp -- "$SOURCE" "$DESTINATION_PATH"
    else
      echo "unexpected coscli cp direction" >&2
      exit 5
    fi
    ;;
  stat)
    if [[ "$*" != *"--endpoint cos.ap-guangzhou.myqcloud.com"* ]]; then
      echo "COS stat must specify the bucket region endpoint" >&2
      exit 8
    fi
    OBJECT_PATH="$COS_ROOT/${1#cos://}"
    [[ -f "$OBJECT_PATH" ]]
    ;;
  *)
    echo "unexpected coscli command: $COMMAND" >&2
    exit 6
    ;;
esac
SH

chmod +x "$MOCK_BIN/docker" "$MOCK_BIN/coscli"
export PATH="$MOCK_BIN:$PATH"
export MOCK_COS_ROOT

SHA="0123456789abcdef0123456789abcdef01234567"
COS_URI="cos://d2s-backup-1250000000/recovery"
BACKUP_DIR="$TEST_ROOT/backups"
RESTORE_DIR="$TEST_ROOT/restored"


D2S_BACKUP_COS_URI="$COS_URI" \
D2S_BACKUP_SOURCE_REGION=ap-shanghai \
D2S_BACKUP_COS_REGION=ap-guangzhou \
D2S_BACKUP_COSCLI="$MOCK_BIN/coscli" \
  "$BACKUP_SCRIPT" "$BACKUP_DIR" "$SHA" > "$TEST_ROOT/backup.log"

BACKUP_FILE="$(find "$BACKUP_DIR" -maxdepth 1 -type f -name "pre-$SHA-*.dump" -print -quit)"
[[ -n "$BACKUP_FILE" ]]
BACKUP_NAME="$(basename "$BACKUP_FILE")"
(
  cd "$BACKUP_DIR"
  sha256sum --check --status "$BACKUP_NAME.sha256"
)
if [[ "$(uname -s)" != MINGW* ]]; then
  [[ "$(stat -c %a "$BACKUP_DIR")" == 700 ]]
fi
[[ -f "$MOCK_COS_ROOT/d2s-backup-1250000000/recovery/$BACKUP_NAME" ]]
[[ -f "$MOCK_COS_ROOT/d2s-backup-1250000000/recovery/$BACKUP_NAME.sha256" ]]

LOCAL_ONLY_DIR="$TEST_ROOT/local-only"
env -u D2S_BACKUP_COS_URI -u D2S_BACKUP_COS_REGION -u D2S_BACKUP_COSCLI \
  "$BACKUP_SCRIPT" "$LOCAL_ONLY_DIR" "$SHA" > "$TEST_ROOT/local-only.log"
LOCAL_ONLY_FILE="$(find "$LOCAL_ONLY_DIR" -maxdepth 1 -type f -name "pre-$SHA-*.dump" -print -quit)"
[[ -n "$LOCAL_ONLY_FILE" ]]
(
  cd "$LOCAL_ONLY_DIR"
  sha256sum --check --status "$(basename "$LOCAL_ONLY_FILE").sha256"
)

REMOTE_OBJECT="$COS_URI/$(basename "$BACKUP_FILE")"
D2S_BACKUP_COSCLI="$MOCK_BIN/coscli" \
D2S_BACKUP_COS_REGION=ap-guangzhou \
  "$DOWNLOAD_SCRIPT" "$REMOTE_OBJECT" "$RESTORE_DIR" > "$TEST_ROOT/download.log"
cmp -- "$BACKUP_FILE" "$RESTORE_DIR/$BACKUP_NAME"
cmp -- "$BACKUP_FILE.sha256" "$RESTORE_DIR/$BACKUP_NAME.sha256"

printf 'corrupt' >> "$MOCK_COS_ROOT/d2s-backup-1250000000/recovery/$BACKUP_NAME"
CORRUPT_DIR="$TEST_ROOT/corrupt"
if D2S_BACKUP_COSCLI="$MOCK_BIN/coscli" \
  D2S_BACKUP_COS_REGION=ap-guangzhou \
  "$DOWNLOAD_SCRIPT" "$REMOTE_OBJECT" "$CORRUPT_DIR" > "$TEST_ROOT/corrupt.log" 2>&1; then
  echo "corrupt remote backup was accepted" >&2
  exit 1
fi
[[ ! -e "$CORRUPT_DIR/$BACKUP_NAME" ]]

MISSING_REGION_MARKER="$TEST_ROOT/missing-region-coscli-was-called"
if env -u D2S_BACKUP_COS_REGION \
  D2S_BACKUP_COSCLI="$MOCK_BIN/coscli" \
  COSCLI_MARKER="$MISSING_REGION_MARKER" \
  "$DOWNLOAD_SCRIPT" "$REMOTE_OBJECT" "$TEST_ROOT/missing-region" > "$TEST_ROOT/missing-region.log" 2>&1; then
  echo "COS restore download accepted a missing bucket region" >&2
  exit 1
fi
[[ ! -e "$MISSING_REGION_MARKER" ]]

REGION_MARKER="$TEST_ROOT/same-region-docker-was-called"
if D2S_BACKUP_COS_URI="$COS_URI" \
  D2S_BACKUP_SOURCE_REGION=ap-shanghai \
  D2S_BACKUP_COS_REGION=ap-shanghai \
  D2S_BACKUP_COSCLI="$MOCK_BIN/coscli" \
  DOCKER_MARKER="$REGION_MARKER" \
  "$BACKUP_SCRIPT" "$TEST_ROOT/same-region" "$SHA" > "$TEST_ROOT/same-region.log" 2>&1; then
  echo "same-region COS backup configuration was accepted" >&2
  exit 1
fi
[[ ! -e "$REGION_MARKER" ]]

echo "COS backup upload, endpoint, missing-region guard, region guard, encryption, portable manifest, restore download, and SHA-256 checks passed"
