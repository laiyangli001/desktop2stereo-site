#!/usr/bin/env bash
set -Eeuo pipefail

REPOSITORY="laiyangli001/desktop2stereo-site"
APP_ROOT="${D2S_DOCKER_APP_ROOT:-/opt/desktop2stereo-site}"
RELEASE_ROOT="${D2S_DOCKER_RELEASE_ROOT:-/opt/desktop2stereo-releases}"
BACKUP_SCRIPT="${D2S_UPDATE_BACKUP_SCRIPT:-/usr/local/sbin/desktop2stereo-db-backup}"
SHA="${1:-}"

if [[ ! "$SHA" =~ ^[0-9a-fA-F]{40}$ ]]; then
  echo "invalid commit SHA" >&2
  exit 2
fi
if [[ ! -x "$BACKUP_SCRIPT" ]]; then
  echo "database backup script is missing: $BACKUP_SCRIPT" >&2
  exit 3
fi
for command_name in curl docker tar; do
  command -v "$command_name" >/dev/null || { echo "required command is missing: $command_name" >&2; exit 4; }
done

mkdir -p "$RELEASE_ROOT" "$APP_ROOT/update-requests"
exec > >(tee -a "$APP_ROOT/logs/update-$SHA.log") 2>&1
LOCK="$APP_ROOT/.update.lock"
if ! mkdir "$LOCK" 2>/dev/null; then
  echo "another Docker update is already running" >&2
  exit 5
fi
cleanup() { rm -rf -- "$LOCK"; }
trap cleanup EXIT

RELEASE="$RELEASE_ROOT/$SHA"
if [[ -e "$RELEASE" ]]; then
  echo "release already prepared: $RELEASE"
  exit 0
fi

"$BACKUP_SCRIPT" "$APP_ROOT/backups" "$SHA"
ARCHIVE="$RELEASE_ROOT/.desktop2stereo-$SHA.tar.gz"
EXTRACT_ROOT="$RELEASE_ROOT/.desktop2stereo-$SHA"
rm -rf -- "$EXTRACT_ROOT" "$ARCHIVE"
mkdir -p "$EXTRACT_ROOT"
curl --fail --location --retry 3 --connect-timeout 10 --max-time 300 \
  -o "$ARCHIVE" "https://codeload.github.com/$REPOSITORY/tar.gz/$SHA"
tar -xzf "$ARCHIVE" -C "$EXTRACT_ROOT"
SOURCE_DIR="$(find "$EXTRACT_ROOT" -mindepth 1 -maxdepth 1 -type d -print -quit)"
if [[ -z "$SOURCE_DIR" ]]; then
  echo "GitHub commit archive has no source directory" >&2
  exit 7
fi
mv "$SOURCE_DIR" "$RELEASE"
rm -rf -- "$EXTRACT_ROOT" "$ARCHIVE"

if [[ -f "$APP_ROOT/Dockerfile" ]]; then
  cp "$APP_ROOT/Dockerfile" "$RELEASE/Dockerfile"
fi
ln -s "$APP_ROOT/.env" "$RELEASE/.env"
ln -s "$APP_ROOT/data" "$RELEASE/data"
ln -s "$APP_ROOT/logs" "$RELEASE/logs"
ln -s "$APP_ROOT/update-requests" "$RELEASE/update-requests"

docker image tag new-api-desktop2stereo-site:local "new-api-desktop2stereo-site:pre-$SHA"
docker compose --env-file "$APP_ROOT/.env" -p desktop2stereo-site -f "$RELEASE/docker-compose.yml" build new-api
docker compose --env-file "$APP_ROOT/.env" -p desktop2stereo-site -f "$RELEASE/docker-compose.yml" up -d --no-deps new-api

for _ in $(seq 1 30); do
  status="$(docker inspect -f '{{.State.Health.Status}}' new-api 2>/dev/null || true)"
  if [[ "$status" == "healthy" ]]; then
    echo "Docker update succeeded: $SHA"
    exit 0
  fi
  sleep 2
done

echo "health check failed; restoring previous image" >&2
docker tag "new-api-desktop2stereo-site:pre-$SHA" new-api-desktop2stereo-site:local
docker compose --env-file "$APP_ROOT/.env" -p desktop2stereo-site -f "$RELEASE/docker-compose.yml" up -d --no-deps new-api || true
exit 8
