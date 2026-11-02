#!/usr/bin/env bash
set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
: "${PGHOST:?PGHOST is required}"
: "${PGPORT:?PGPORT is required}"
: "${PGUSER:?PGUSER is required}"
: "${PGPASSWORD:?PGPASSWORD is required}"
: "${PGDATABASE:?PGDATABASE is required}"

psql_cmd=(psql -X -v ON_ERROR_STOP=1 -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE")
"${psql_cmd[@]}" -f "$REPO_ROOT/deploy/d2s-postgresql-provision-roles.sql"
"${psql_cmd[@]}" -f "$REPO_ROOT/deploy/d2s-postgresql-app-ownership.sql"
"${psql_cmd[@]}" -f "$REPO_ROOT/deploy/d2s-audit-permissions.postgresql.sql"

"${psql_cmd[@]}" -Atqc "
SELECT CASE WHEN count(*) = 3 THEN 'ok' ELSE 'invalid' END
FROM pg_roles
WHERE rolname IN ('d2s_app', 'd2s_migrator', 'd2s_audit_cleanup')
  AND NOT rolsuper AND NOT rolcreatedb AND NOT rolcreaterole AND NOT rolinherit;
" | grep -Fxq ok

assert_role_statement_denied() {
  local role="$1"
  local statement="$2"
  if "${psql_cmd[@]}" -c "SET ROLE $role; $statement" >/dev/null 2>&1; then
    echo "unexpected permission: $role may execute $statement" >&2
    return 1
  fi
}

assert_role_statement_denied d2s_app "UPDATE public.d2s_user_action_logs SET result_status = result_status WHERE false"
assert_role_statement_denied d2s_app "DELETE FROM public.d2s_user_action_logs WHERE false"
assert_role_statement_denied d2s_app "TRUNCATE public.d2s_user_action_logs"
assert_role_statement_denied d2s_audit_cleanup "UPDATE public.d2s_user_action_logs SET result_status = result_status WHERE false"
assert_role_statement_denied d2s_audit_cleanup "INSERT INTO public.d2s_user_action_logs DEFAULT VALUES"
assert_role_statement_denied d2s_audit_cleanup "UPDATE public.d2s_audit_retention_runs SET result = result WHERE false"
assert_role_statement_denied d2s_audit_cleanup "DELETE FROM public.d2s_audit_retention_runs WHERE false"

echo "PostgreSQL application, migration, and cleanup role checks passed."
