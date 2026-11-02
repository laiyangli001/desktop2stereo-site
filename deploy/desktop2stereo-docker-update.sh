#!/usr/bin/env bash
set -Eeuo pipefail

REPOSITORY="${2:-${D2S_UPDATE_REPOSITORY:-laiyangli001/desktop2stereo-site}}"
BRANCH="${3:-${D2S_UPDATE_BRANCH:-main}}"
APP_ROOT="${D2S_DOCKER_APP_ROOT:-/opt/desktop2stereo-site}"
RELEASE_ROOT="${D2S_DOCKER_RELEASE_ROOT:-/opt/desktop2stereo-releases}"
BACKUP_SCRIPT="${D2S_UPDATE_BACKUP_SCRIPT:-/usr/local/sbin/desktop2stereo-db-backup}"
UPDATE_SCRIPT_PATH="${D2S_DOCKER_UPDATE_SCRIPT:-/usr/local/sbin/desktop2stereo-docker-update}"
BUILD_GOPROXY="${D2S_BUILD_GOPROXY:-https://goproxy.cn,direct}"
BUILD_GOSUMDB="${D2S_BUILD_GOSUMDB:-off}"
BUILD_TIMEOUT_SECONDS="${D2S_BUILD_TIMEOUT_SECONDS:-900}"
DOWNLOAD_TIMEOUT_SECONDS="${D2S_DOWNLOAD_TIMEOUT_SECONDS:-1800}"
SHA="${1:-}"
STATUS_FILE="${D2S_UPDATE_STATUS_FILE:-$APP_ROOT/update-requests/status.json}"
SOURCE_FILE="${D2S_UPDATE_SOURCE_FILE:-$APP_ROOT/update-requests/source}"
STATUS_URL="${D2S_DOCKER_STATUS_URL:-http://127.0.0.1:3000/api/status}"
CURRENT_PHASE="starting"
DOWNLOAD_BYTES=""
DOWNLOAD_TOTAL_BYTES=""
DOWNLOAD_SPEED_BPS=""

if [[ -z "${2:-}" && -f "$SOURCE_FILE" ]]; then
  mapfile -t SOURCE_FIELDS < "$SOURCE_FILE"
  REPOSITORY="${SOURCE_FIELDS[0]//[[:space:]]/}"
  BRANCH="${SOURCE_FIELDS[1]//[[:space:]]/}"
  rm -f -- "$SOURCE_FILE"
fi

write_status() {
  local state="$1" phase="$2" message="$3" error_message="${4:-}"
  local temporary="${STATUS_FILE}.tmp"
  mkdir -p "$(dirname "$STATUS_FILE")"
  STATUS_STATE="$state" STATUS_PHASE="$phase" STATUS_MESSAGE="$message" \
    STATUS_ERROR="$error_message" STATUS_SHA="$SHA" STATUS_UPDATED_AT="$(date -Is)" \
    STATUS_DOWNLOAD_BYTES="$DOWNLOAD_BYTES" STATUS_DOWNLOAD_TOTAL_BYTES="$DOWNLOAD_TOTAL_BYTES" \
    STATUS_DOWNLOAD_SPEED_BPS="$DOWNLOAD_SPEED_BPS" \
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
if os.environ["STATUS_PHASE"] == "download":
    for field, variable in (
        ("download_bytes", "STATUS_DOWNLOAD_BYTES"),
        ("download_total_bytes", "STATUS_DOWNLOAD_TOTAL_BYTES"),
        ("download_speed_bps", "STATUS_DOWNLOAD_SPEED_BPS"),
    ):
        value = os.environ[variable]
        if value.isdigit():
            payload[field] = int(value)
with open(sys.argv[1], "w", encoding="utf-8") as handle:
    json.dump(payload, handle, ensure_ascii=False)
    handle.write("\n")
PY
  mv -f -- "$temporary" "$STATUS_FILE"
}

read_runtime_version() {
  local response
  response="$(curl --fail --silent --show-error --max-time 5 "$STATUS_URL")" || return 1
  printf '%s' "$response" | python3 -c '
import json
import sys

try:
    payload = json.load(sys.stdin)
except (json.JSONDecodeError, UnicodeDecodeError):
    sys.exit(1)
if not isinstance(payload, dict) or payload.get("success") is not True:
    sys.exit(1)
data = payload.get("data")
version = data.get("version") if isinstance(data, dict) else None
if not isinstance(version, str) or not version:
    sys.exit(1)
sys.stdout.write(version)
'
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
if [[ ! "$REPOSITORY" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ || ! "$BRANCH" =~ ^[A-Za-z0-9._/-]+$ || "$BRANCH" == /* || "$BRANCH" == */ || "$BRANCH" == *..* ]]; then
  echo "invalid update source" >&2
  exit 2
fi
if [[ ! -x "$BACKUP_SCRIPT" ]]; then
  echo "database backup script is missing: $BACKUP_SCRIPT" >&2
  exit 3
fi
for command_name in awk cmp curl docker python3 stat tar timeout; do
  command -v "$command_name" >/dev/null || { echo "required command is missing: $command_name" >&2; exit 4; }
done
if [[ ! "$BUILD_TIMEOUT_SECONDS" =~ ^[1-9][0-9]*$ ]]; then
  echo "D2S_BUILD_TIMEOUT_SECONDS must be a positive integer" >&2
  exit 4
fi
if [[ ! "$DOWNLOAD_TIMEOUT_SECONDS" =~ ^[1-9][0-9]*$ ]]; then
  echo "D2S_DOWNLOAD_TIMEOUT_SECONDS must be a positive integer" >&2
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
  if running_version="$(read_runtime_version 2>/dev/null)" && [[ "$running_version" == "$SHA" ]]; then
    write_status "succeeded" "completed" "该版本已部署，无需重复更新"
    echo "release already prepared and running: $RELEASE"
    exit 0
  fi
  echo "release marker exists but the running version does not match; rebuilding: $RELEASE"
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
DOWNLOAD_HEADERS="$RELEASE_ROOT/.desktop2stereo-$SHA.headers"
rm -rf -- "$EXTRACT_ROOT" "$ARCHIVE"
rm -f -- "$DOWNLOAD_HEADERS"
mkdir -p "$EXTRACT_ROOT"
CURRENT_PHASE="download"
DOWNLOAD_BYTES=0
DOWNLOAD_SPEED_BPS=0
write_status "running" "$CURRENT_PHASE" "正在下载项目代码"
curl --fail --location --retry 3 --connect-timeout 10 --max-time "$DOWNLOAD_TIMEOUT_SECONDS" \
  --silent --show-error --dump-header "$DOWNLOAD_HEADERS" \
  -o "$ARCHIVE" "https://codeload.github.com/$REPOSITORY/tar.gz/$SHA" &
download_pid=$!
last_bytes=0
last_time=$(date +%s)
while kill -0 "$download_pid" 2>/dev/null; do
  sleep 2
  DOWNLOAD_BYTES=$(stat -c %s "$ARCHIVE" 2>/dev/null || printf '0')
  DOWNLOAD_TOTAL_BYTES=$(awk '/^HTTP\// { total="" } tolower($1) == "content-length:" { total=$2 } END { gsub("\r", "", total); if (total ~ /^[0-9]+$/) print total }' "$DOWNLOAD_HEADERS" 2>/dev/null || true)
  now=$(date +%s)
  if (( now > last_time && DOWNLOAD_BYTES >= last_bytes )); then
    DOWNLOAD_SPEED_BPS=$(( (DOWNLOAD_BYTES - last_bytes) / (now - last_time) ))
  else
    DOWNLOAD_SPEED_BPS=0
  fi
  write_status "running" "$CURRENT_PHASE" "正在下载项目代码"
  last_bytes=$DOWNLOAD_BYTES
  last_time=$now
done
download_exit=0
wait "$download_pid" || download_exit=$?
DOWNLOAD_BYTES=$(stat -c %s "$ARCHIVE" 2>/dev/null || printf '0')
write_status "running" "$CURRENT_PHASE" "正在下载项目代码"
if (( download_exit != 0 )); then
  write_status "failed" "$CURRENT_PHASE" "更新失败" "下载项目代码失败，curl 退出代码 ${download_exit}"
  exit "$download_exit"
fi
rm -f -- "$DOWNLOAD_HEADERS"
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
CURRENT_PHASE="audit_ownership"
write_status "running" "$CURRENT_PHASE" "正在配置应用数据库身份"
docker compose --env-file "$APP_ROOT/.env" -p desktop2stereo-site -f "$RELEASE/docker-compose.yml" --profile maintenance run --rm --no-deps audit-ownership
CURRENT_PHASE="audit_migration"
write_status "running" "$CURRENT_PHASE" "正在迁移审计表"
docker compose --env-file "$APP_ROOT/.env" -p desktop2stereo-site -f "$RELEASE/docker-compose.yml" --profile maintenance run --rm --no-deps audit-migrate
CURRENT_PHASE="audit_permissions"
write_status "running" "$CURRENT_PHASE" "正在校验审计表数据库权限"
docker compose --env-file "$APP_ROOT/.env" -p desktop2stereo-site -f "$RELEASE/docker-compose.yml" --profile maintenance run --rm --no-deps audit-permissions
CURRENT_PHASE="restart"
write_status "running" "$CURRENT_PHASE" "正在重启应用容器"
docker compose --env-file "$APP_ROOT/.env" -p desktop2stereo-site -f "$RELEASE/docker-compose.yml" up -d --no-deps new-api

CURRENT_PHASE="health"
write_status "running" "$CURRENT_PHASE" "正在执行健康检查和运行版本校验"
HEALTHY_CONTAINER_SEEN=0
VERSION_CHECK_FAILURE="API status did not confirm target commit $SHA"
for _ in $(seq 1 30); do
  status="$(docker inspect -f '{{.State.Health.Status}}' new-api 2>/dev/null || true)"
  if [[ "$status" == "healthy" ]]; then
    HEALTHY_CONTAINER_SEEN=1
    running_version="$(read_runtime_version 2>/dev/null || true)"
    if [[ "$running_version" == "$SHA" ]]; then
      if [[ -f "$RELEASE/deploy/desktop2stereo-docker-update.sh" ]]; then
        install -o root -g root -m 0750 "$RELEASE/deploy/desktop2stereo-docker-update.sh" "$UPDATE_SCRIPT_PATH"
        if ! cmp -s "$RELEASE/deploy/desktop2stereo-docker-update.sh" "$UPDATE_SCRIPT_PATH"; then
          echo "installed Docker update script does not match release" >&2
          exit 9
        fi
      fi
      if [[ -f "$RELEASE/deploy/desktop2stereo-db-backup-docker.sh" ]]; then
        install -o root -g root -m 0750 "$RELEASE/deploy/desktop2stereo-db-backup-docker.sh" "$BACKUP_SCRIPT"
        if ! cmp -s "$RELEASE/deploy/desktop2stereo-db-backup-docker.sh" "$BACKUP_SCRIPT"; then
          echo "installed database backup script does not match release" >&2
          exit 9
        fi
      fi
      touch "$RELEASE_MARKER"
      write_status "succeeded" "completed" "更新完成，运行版本 SHA 与目标提交一致"
      echo "Docker update succeeded: $SHA"
      exit 0
    fi
  fi
  sleep 2
done

CURRENT_PHASE="rollback"
if [[ "$HEALTHY_CONTAINER_SEEN" == "1" ]]; then
  echo "runtime version check failed; restoring previous image" >&2
  write_status "running" "$CURRENT_PHASE" "运行版本 SHA 校验失败，正在恢复旧版本" "$VERSION_CHECK_FAILURE"
else
  echo "health check failed; restoring previous image" >&2
  write_status "running" "$CURRENT_PHASE" "健康检查失败，正在恢复旧版本" "新版本健康检查未通过"
fi
docker tag "new-api-desktop2stereo-site:pre-$SHA" new-api-desktop2stereo-site:local
docker compose --env-file "$APP_ROOT/.env" -p desktop2stereo-site -f "$RELEASE/docker-compose.yml" up -d --no-deps new-api || true
if [[ "$HEALTHY_CONTAINER_SEEN" == "1" ]]; then
  write_status "failed" "rollback" "运行版本 SHA 校验失败，已尝试恢复旧版本" "$VERSION_CHECK_FAILURE"
else
  write_status "failed" "rollback" "健康检查失败，已尝试恢复旧版本" "新版本健康检查未通过"
fi
exit 8
