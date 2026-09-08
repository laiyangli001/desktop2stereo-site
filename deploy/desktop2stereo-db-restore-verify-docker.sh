#!/usr/bin/env bash
set -Eeuo pipefail

BACKUP_FILE="${1:?backup file is required}"
RECOVERY_CONTAINER="${D2S_RECOVERY_CONTAINER:-d2s-postgres-recovery}"
POSTGRES_USER="${D2S_RECOVERY_POSTGRES_USER:-d2s}"
POSTGRES_DB="${D2S_RECOVERY_POSTGRES_DB:-new-api}"
POSTGRES_PASSWORD="${D2S_RECOVERY_POSTGRES_PASSWORD:?D2S_RECOVERY_POSTGRES_PASSWORD is required}"

if [[ ! -f "$BACKUP_FILE" ]]; then
  echo "backup file does not exist: $BACKUP_FILE" >&2
  exit 2
fi
if [[ ! -f "$BACKUP_FILE.sha256" ]]; then
  echo "backup checksum file does not exist: $BACKUP_FILE.sha256" >&2
  exit 2
fi
for command_name in docker sha256sum; do
  command -v "$command_name" >/dev/null || {
    echo "required command is missing: $command_name" >&2
    exit 3
  }
done
if docker container inspect "$RECOVERY_CONTAINER" >/dev/null 2>&1; then
  echo "recovery container already exists; remove only this fixed recovery container before retrying: $RECOVERY_CONTAINER" >&2
  exit 4
fi

cleanup() {
  docker rm --force "$RECOVERY_CONTAINER" >/dev/null 2>&1 || true
}
trap cleanup EXIT

sha256sum --check --status "$BACKUP_FILE.sha256"
docker run --detach --name "$RECOVERY_CONTAINER" \
  --tmpfs /var/lib/postgresql/data \
  --env "POSTGRES_USER=$POSTGRES_USER" \
  --env "POSTGRES_DB=$POSTGRES_DB" \
  --env "POSTGRES_PASSWORD=$POSTGRES_PASSWORD" \
  postgres:15 >/dev/null

for _ in $(seq 1 60); do
  if docker exec "$RECOVERY_CONTAINER" pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB" >/dev/null 2>&1; then
    break
  fi
  sleep 2
done
if ! docker exec "$RECOVERY_CONTAINER" pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB" >/dev/null 2>&1; then
  echo "isolated PostgreSQL recovery container did not become ready" >&2
  exit 5
fi

docker exec "$RECOVERY_CONTAINER" mkdir -p /recovery
docker cp "$BACKUP_FILE" "$RECOVERY_CONTAINER:/recovery/backup.dump"
docker exec "$RECOVERY_CONTAINER" pg_restore \
  --exit-on-error --no-owner --no-privileges \
  -U "$POSTGRES_USER" -d "$POSTGRES_DB" /recovery/backup.dump

restored_tables="$(docker exec "$RECOVERY_CONTAINER" psql -Atq \
  -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
  -c "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name LIKE 'd2s_%';")"
if [[ ! "$restored_tables" =~ ^[0-9]+$ ]] || (( restored_tables == 0 )); then
  echo "restored database does not contain D2S tables" >&2
  exit 6
fi

echo "isolated PostgreSQL restore verification passed: $restored_tables D2S table(s) restored"
