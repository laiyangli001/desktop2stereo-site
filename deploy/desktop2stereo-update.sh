#!/usr/bin/env bash
set -Eeuo pipefail

# This script is intentionally fixed to the Desktop2Stereo repository.
# It never copies shared data into a release directory.
REPOSITORY="${2:-${D2S_UPDATE_REPOSITORY:-laiyangli001/desktop2stereo-site}}"
BRANCH="${3:-${D2S_UPDATE_BRANCH:-main}}"
ROOT="${D2S_UPDATE_ROOT:-/opt/desktop2stereo}"
RELEASES="$ROOT/releases"
SHARED="$ROOT/shared"
CURRENT="$ROOT/current"
BACKUP_SCRIPT="${D2S_UPDATE_BACKUP_SCRIPT:-/usr/local/sbin/desktop2stereo-db-backup}"
RESTART_MODE="${D2S_UPDATE_RESTART_MODE:-manual}"
SERVICE_NAME="${D2S_UPDATE_SERVICE:-desktop2stereo}"
HEALTH_URL="${D2S_UPDATE_HEALTH_URL:-http://127.0.0.1:3000/api/status}"
SHA="${1:-}"

if [[ ! "$SHA" =~ ^[0-9a-fA-F]{40}$ ]]; then
  echo "invalid commit SHA" >&2
  exit 2
fi
if [[ ! "$REPOSITORY" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ || ! "$BRANCH" =~ ^[A-Za-z0-9._/-]+$ || "$BRANCH" == /* || "$BRANCH" == */ || "$BRANCH" == *..* ]]; then
  echo "invalid update source" >&2
  exit 2
fi
if [[ ! -d "$SHARED" || ! -f "$SHARED/.env" ]]; then
  echo "shared runtime configuration is missing: $SHARED/.env" >&2
  exit 3
fi
if [[ ! -x "$BACKUP_SCRIPT" ]]; then
  echo "database backup script is missing or not executable: $BACKUP_SCRIPT" >&2
  exit 4
fi
for command_name in git go npm; do
  command -v "$command_name" >/dev/null || { echo "required command is missing: $command_name" >&2; exit 5; }
done

mkdir -p "$RELEASES" "$SHARED/backups" "$SHARED/logs"
exec > >(tee -a "$SHARED/logs/update-$SHA.log") 2>&1
LOCK="$ROOT/.update.lock"
if ! mkdir "$LOCK" 2>/dev/null; then
  echo "another update is already running" >&2
  exit 6
fi
cleanup() { rm -rf -- "$LOCK"; }
trap cleanup EXIT

RELEASE="$RELEASES/$SHA"
if [[ -e "$RELEASE" ]]; then
  echo "release already prepared: $RELEASE"
  exit 0
fi

"$BACKUP_SCRIPT" "$SHARED/backups" "$SHA"

git clone --filter=blob:none --no-checkout --branch "$BRANCH" --single-branch "https://github.com/$REPOSITORY.git" "$RELEASE"
git -C "$RELEASE" checkout --detach "$SHA"
ln -s "$SHARED/.env" "$RELEASE/.env"
if [[ -d "$SHARED/uploads" ]]; then
  ln -s "$SHARED/uploads" "$RELEASE/uploads"
fi

pushd "$RELEASE/web" >/dev/null
npm ci
npm run build
popd >/dev/null
go build -o "$RELEASE/desktop2stereo-site.new" "$RELEASE"
mv "$RELEASE/desktop2stereo-site.new" "$RELEASE/desktop2stereo-site"

OLD_TARGET=""
if [[ -L "$CURRENT" ]]; then
  OLD_TARGET="$(readlink "$CURRENT")"
fi
NEXT_LINK="$ROOT/.current.next"
rm -f -- "$NEXT_LINK"
ln -s "$RELEASE" "$NEXT_LINK"
mv -Tf "$NEXT_LINK" "$CURRENT"

restart_service() {
  case "$RESTART_MODE" in
    systemd) systemctl restart "$SERVICE_NAME" ;;
    supervisor) supervisorctl restart "$SERVICE_NAME" ;;
    manual) echo "restart required: $SERVICE_NAME" ;;
    *) echo "unsupported D2S_UPDATE_RESTART_MODE: $RESTART_MODE" >&2; return 1 ;;
  esac
}

if [[ "$RESTART_MODE" != "manual" ]]; then
  if ! restart_service; then
    echo "service restart failed; rolling back" >&2
    if [[ -n "$OLD_TARGET" ]]; then
      ROLLBACK_LINK="$ROOT/.current.rollback"
      rm -f -- "$ROLLBACK_LINK"
      ln -s "$OLD_TARGET" "$ROLLBACK_LINK"
      mv -Tf "$ROLLBACK_LINK" "$CURRENT"
      restart_service || true
    fi
    exit 8
  fi
  if command -v curl >/dev/null; then
    for _ in $(seq 1 30); do
      if curl --fail --silent --show-error --max-time 3 "$HEALTH_URL" >/dev/null; then
        echo "update succeeded: $SHA"
        exit 0
      fi
      sleep 2
    done
    echo "health check failed; rolling back" >&2
    if [[ -n "$OLD_TARGET" ]]; then
      ROLLBACK_LINK="$ROOT/.current.rollback"
      rm -f -- "$ROLLBACK_LINK"
      ln -s "$OLD_TARGET" "$ROLLBACK_LINK"
      mv -Tf "$ROLLBACK_LINK" "$CURRENT"
      restart_service || true
    fi
    exit 8
  fi
fi

echo "release prepared: $SHA"
