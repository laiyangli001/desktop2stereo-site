#!/usr/bin/env bash
set -Eeuo pipefail

REPOSITORY="https://github.com/laiyangli001/desktop2stereo-site.git"
BRANCH="main"
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
for command_name in git docker; do
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
git clone --filter=blob:none --no-checkout --branch "$BRANCH" --single-branch "$REPOSITORY" "$RELEASE"
git -C "$RELEASE" checkout --detach "$SHA"

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
