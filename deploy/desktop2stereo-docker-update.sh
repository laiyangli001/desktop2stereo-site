#!/usr/bin/env bash
set -Eeuo pipefail

REPOSITORY="laiyangli001/desktop2stereo-site"
APP_ROOT="${D2S_DOCKER_APP_ROOT:-/opt/desktop2stereo-site}"
RELEASE_ROOT="${D2S_DOCKER_RELEASE_ROOT:-/opt/desktop2stereo-releases}"
BACKUP_SCRIPT="${D2S_UPDATE_BACKUP_SCRIPT:-/usr/local/sbin/desktop2stereo-db-backup}"
UPDATE_SCRIPT_PATH="${D2S_DOCKER_UPDATE_SCRIPT:-/usr/local/sbin/desktop2stereo-docker-update}"
BUILD_GOPROXY="${D2S_BUILD_GOPROXY:-https://goproxy.cn,direct}"
BUILD_GOSUMDB="${D2S_BUILD_GOSUMDB:-off}"
BUILD_TIMEOUT_SECONDS="${D2S_BUILD_TIMEOUT_SECONDS:-900}"
SHA="${1:-}"
STATUS_FILE="${D2S_UPDATE_STATUS_FILE:-$APP_ROOT/update-requests/status.json}"
CURRENT_PHASE="starting"

write_status() {
  local state="$1" phase="$2" message="$3" error_message="${4:-}"
  local temporary="${STATUS_FILE}.tmp"
  mkdir -p "$(dirname "$STATUS_FILE")"
  STATUS_STATE="$state" STATUS_PHASE="$phase" STATUS_MESSAGE="$message" \
    STATUS_ERROR="$error_message" STATUS_SHA="$SHA" STATUS_UPDATED_AT="$(date -Is)" \
    python3 - "$temporary" <<'PY'
import json
import os
import sys

payload = {
    "state": os.environ["STATUS_STATE"],
    "phase": os.environ["STATUS_PHASE"],
    "message": os.environ["STATUS_MESSAGE"],
    "sha": os.environ["STATUS_SHA"],
    "updated_at": os.environ["STATUS_UPDATED_AT"],
}
if os.environ["STATUS_ERROR"]:
    payload["error"] = os.environ["STATUS_ERROR"]
with open(sys.argv[1], "w", encoding="utf-8") as handle:
    json.dump(payload, handle, ensure_ascii=False)
    handle.write("\n")
PY
  mv -f -- "$temporary" "$STATUS_FILE"
}

handle_error() {
  local exit_code=$?
  set +e
  write_status "failed" "$CURRENT_PHASE" "更新失败" "更新脚本在阶段 ${CURRENT_PHASE} 退出，代码 ${exit_code}"
  exit "$exit_code"
}

trap handle_error ERR

if [[ ! "$SHA" =~ ^[0-9a-fA-F]{40}$ ]]; then
  echo "invalid commit SHA" >&2
  exit 2
fi
if [[ ! -x "$BACKUP_SCRIPT" ]]; then
  echo "database backup script is missing: $BACKUP_SCRIPT" >&2
  exit 3
fi
for command_name in curl docker tar timeout; do
  command -v "$command_name" >/dev/null || { echo "required command is missing: $command_name" >&2; exit 4; }
done
if [[ ! "$BUILD_TIMEOUT_SECONDS" =~ ^[1-9][0-9]*$ ]]; then
  echo "D2S_BUILD_TIMEOUT_SECONDS must be a positive integer" >&2
  exit 4
fi

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
RELEASE_MARKER="$RELEASE/.update-complete"
if [[ -f "$RELEASE_MARKER" ]]; then
	write_status "succeeded" "completed" "该版本已部署，无需重复更新"
  echo "release already prepared: $RELEASE"
  exit 0
fi
if [[ -e "$RELEASE" ]]; then
  echo "removing incomplete release: $RELEASE"
  rm -rf -- "$RELEASE"
fi

CURRENT_PHASE="backup"
write_status "running" "$CURRENT_PHASE" "正在备份数据库"
"$BACKUP_SCRIPT" "$APP_ROOT/backups" "$SHA"
ARCHIVE="$RELEASE_ROOT/.desktop2stereo-$SHA.tar.gz"
EXTRACT_ROOT="$RELEASE_ROOT/.desktop2stereo-$SHA"
rm -rf -- "$EXTRACT_ROOT" "$ARCHIVE"
mkdir -p "$EXTRACT_ROOT"
CURRENT_PHASE="download"
write_status "running" "$CURRENT_PHASE" "正在下载项目代码"
curl --fail --location --retry 3 --connect-timeout 10 --max-time 300 \
  -o "$ARCHIVE" "https://codeload.github.com/$REPOSITORY/tar.gz/$SHA"
CURRENT_PHASE="extract"
write_status "running" "$CURRENT_PHASE" "正在解压项目代码"
tar -xzf "$ARCHIVE" -C "$EXTRACT_ROOT"
SOURCE_DIR="$(find "$EXTRACT_ROOT" -mindepth 1 -maxdepth 1 -type d -print -quit)"
if [[ -z "$SOURCE_DIR" ]]; then
  echo "GitHub commit archive has no source directory" >&2
  exit 7
fi
mv "$SOURCE_DIR" "$RELEASE"
rm -rf -- "$EXTRACT_ROOT" "$ARCHIVE"

ln -s "$APP_ROOT/.env" "$RELEASE/.env"
ln -s "$APP_ROOT/data" "$RELEASE/data"
ln -s "$APP_ROOT/logs" "$RELEASE/logs"
ln -s "$APP_ROOT/update-requests" "$RELEASE/update-requests"

docker image tag new-api-desktop2stereo-site:local "new-api-desktop2stereo-site:pre-$SHA"
CURRENT_PHASE="build"
write_status "running" "$CURRENT_PHASE" "正在构建 Docker 镜像"
timeout --foreground --signal=TERM --kill-after=30 "$BUILD_TIMEOUT_SECONDS" \
docker compose --env-file "$APP_ROOT/.env" -p desktop2stereo-site -f "$RELEASE/docker-compose.yml" build \
  --build-arg "D2S_BUILD_VERSION=$SHA" \
  --build-arg "GOPROXY=$BUILD_GOPROXY" \
  --build-arg "GOSUMDB=$BUILD_GOSUMDB" new-api
docker image tag new-api-desktop2stereo-site:local "new-api-desktop2stereo-site:$SHA"
CURRENT_PHASE="restart"
write_status "running" "$CURRENT_PHASE" "正在重启应用容器"
docker compose --env-file "$APP_ROOT/.env" -p desktop2stereo-site -f "$RELEASE/docker-compose.yml" up -d --no-deps new-api

CURRENT_PHASE="health"
write_status "running" "$CURRENT_PHASE" "正在执行健康检查"
for _ in $(seq 1 30); do
  status="$(docker inspect -f '{{.State.Health.Status}}' new-api 2>/dev/null || true)"
  if [[ "$status" == "healthy" ]]; then
    if [[ -f "$RELEASE/deploy/desktop2stereo-docker-update.sh" ]]; then
	  install -o root -g root -m 0750 "$RELEASE/deploy/desktop2stereo-docker-update.sh" "$UPDATE_SCRIPT_PATH"
	fi
	if [[ -f "$RELEASE/deploy/desktop2stereo-db-backup-docker.sh" ]]; then
	  install -o root -g root -m 0750 "$RELEASE/deploy/desktop2stereo-db-backup-docker.sh" "$BACKUP_SCRIPT"
	fi
	touch "$RELEASE_MARKER"
	write_status "succeeded" "completed" "更新完成，应用健康检查通过"
    echo "Docker update succeeded: $SHA"
    exit 0
  fi
  sleep 2
done

echo "health check failed; restoring previous image" >&2
CURRENT_PHASE="rollback"
write_status "running" "$CURRENT_PHASE" "健康检查失败，正在恢复旧版本"
docker tag "new-api-desktop2stereo-site:pre-$SHA" new-api-desktop2stereo-site:local
docker compose --env-file "$APP_ROOT/.env" -p desktop2stereo-site -f "$RELEASE/docker-compose.yml" up -d --no-deps new-api || true
write_status "failed" "rollback" "健康检查失败，已尝试恢复旧版本" "新版本健康检查未通过"
exit 8
