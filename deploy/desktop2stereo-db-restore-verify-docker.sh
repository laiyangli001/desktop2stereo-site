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

restored_counts="$(docker exec "$RECOVERY_CONTAINER" psql -Atq \
  -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
  -c "SELECT 'users=' || (SELECT count(*) FROM users)
      UNION ALL SELECT 'profiles=' || (SELECT count(*) FROM d2s_user_profiles)
      UNION ALL SELECT 'licenses=' || (SELECT count(*) FROM d2s_licenses)
      UNION ALL SELECT 'orders=' || (SELECT count(*) FROM d2s_orders)
      UNION ALL SELECT 'payment_events=' || (SELECT count(*) FROM d2s_payment_events)
      UNION ALL SELECT 'balance_accounts=' || (SELECT count(*) FROM d2s_balance_accounts)
      UNION ALL SELECT 'balance_transactions=' || (SELECT count(*) FROM d2s_balance_transactions)
      UNION ALL SELECT 'license_events=' || (SELECT count(*) FROM d2s_license_events)
      UNION ALL SELECT 'signing_keys=' || (SELECT count(*) FROM d2s_signing_keys)")"

integrity_report="$(docker exec -i "$RECOVERY_CONTAINER" psql -Atq \
  -U "$POSTGRES_USER" -d "$POSTGRES_DB" <<'SQL'
SELECT 'orphan_profiles=' || count(*) FROM d2s_user_profiles p LEFT JOIN users u ON u.id = p.user_id WHERE u.id IS NULL
UNION ALL SELECT 'orphan_licenses=' || count(*) FROM d2s_licenses l LEFT JOIN users u ON u.id = l.user_id WHERE u.id IS NULL
UNION ALL SELECT 'orphan_license_source_orders=' || count(*) FROM d2s_licenses l LEFT JOIN d2s_orders o ON o.id = l.source_order_id WHERE l.source_order_id <> '' AND o.id IS NULL
UNION ALL SELECT 'orphan_bindings=' || count(*) FROM d2s_device_bindings b LEFT JOIN d2s_licenses l ON l.id = b.license_id WHERE l.id IS NULL
UNION ALL SELECT 'orphan_orders=' || count(*) FROM d2s_orders o LEFT JOIN users u ON u.id = o.user_id WHERE u.id IS NULL
UNION ALL SELECT 'orphan_payment_events=' || count(*) FROM d2s_payment_events e LEFT JOIN d2s_orders o ON o.id = e.order_id WHERE o.id IS NULL
UNION ALL SELECT 'orphan_license_events=' || count(*) FROM d2s_license_events e LEFT JOIN d2s_licenses l ON l.id = e.license_id WHERE l.id IS NULL
UNION ALL SELECT 'orphan_license_event_users=' || count(*) FROM d2s_license_events e LEFT JOIN users u ON u.id = e.user_id WHERE u.id IS NULL
UNION ALL SELECT 'orphan_license_event_orders=' || count(*) FROM d2s_license_events e LEFT JOIN d2s_orders o ON o.id = e.order_id WHERE e.order_id <> '' AND o.id IS NULL
UNION ALL SELECT 'orphan_balance_accounts=' || count(*) FROM d2s_balance_accounts a LEFT JOIN users u ON u.id = a.user_id WHERE u.id IS NULL
UNION ALL SELECT 'orphan_balance_transactions=' || count(*) FROM d2s_balance_transactions t LEFT JOIN d2s_balance_accounts a ON a.id = t.account_id WHERE a.id IS NULL
UNION ALL SELECT 'orphan_balance_transaction_users=' || count(*) FROM d2s_balance_transactions t LEFT JOIN users u ON u.id = t.user_id WHERE u.id IS NULL
UNION ALL SELECT 'orphan_balance_transaction_orders=' || count(*) FROM d2s_balance_transactions t LEFT JOIN d2s_orders o ON o.id = t.order_id WHERE t.order_id <> '' AND o.id IS NULL
UNION ALL SELECT 'orphan_inviter_rewards=' || count(*) FROM d2s_invite_rewards r LEFT JOIN users u ON u.id = r.inviter_user_id WHERE u.id IS NULL
UNION ALL SELECT 'orphan_invitee_rewards=' || count(*) FROM d2s_invite_rewards r LEFT JOIN users u ON u.id = r.invitee_user_id WHERE u.id IS NULL
UNION ALL SELECT 'orphan_withdrawals=' || count(*) FROM d2s_withdrawal_requests w LEFT JOIN users u ON u.id = w.user_id WHERE u.id IS NULL
UNION ALL SELECT 'orphan_offline_entitlements=' || count(*) FROM d2s_offline_entitlements e LEFT JOIN d2s_licenses l ON l.id = e.license_id WHERE l.id IS NULL
UNION ALL SELECT 'orphan_online_leases=' || count(*) FROM d2s_online_leases e LEFT JOIN d2s_licenses l ON l.id = e.license_id WHERE l.id IS NULL
UNION ALL SELECT 'orphan_manual_unbinds=' || count(*) FROM d2s_manual_unbind_requests r LEFT JOIN d2s_licenses l ON l.id = r.license_id WHERE l.id IS NULL
UNION ALL SELECT 'orphan_paid_revoke_quotas=' || count(*) FROM d2s_paid_revoke_quotas q LEFT JOIN d2s_licenses l ON l.id = q.license_id WHERE l.id IS NULL;
SQL
)"

while IFS='=' read -r check_name check_count; do
  if [[ ! "$check_count" =~ ^[0-9]+$ ]] || (( check_count != 0 )); then
    echo "restore integrity check failed: $check_name=$check_count" >&2
    exit 7
  fi
done <<< "$integrity_report"

echo "isolated PostgreSQL restore verification passed: $restored_tables D2S table(s) restored"
echo "restored critical row counts: ${restored_counts//$'\n'/, }"
echo "restore referential integrity checks passed: ${integrity_report//$'\n'/, }"
