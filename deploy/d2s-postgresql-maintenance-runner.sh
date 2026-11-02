#!/bin/sh
set -eu

: "${PGHOST:?PGHOST is required}"
: "${PGUSER:?PGUSER is required}"
: "${PGPASSWORD:?PGPASSWORD is required}"
: "${PGDATABASE:?PGDATABASE is required}"
: "${D2S_PSQL_SCRIPT:?D2S_PSQL_SCRIPT is required}"

psql -v ON_ERROR_STOP=1 -h "$PGHOST" -U "$PGUSER" -d "$PGDATABASE" -f "$D2S_PSQL_SCRIPT"
if [ -n "${D2S_PSQL_SCRIPT_SECONDARY:-}" ]; then
    psql -v ON_ERROR_STOP=1 -h "$PGHOST" -U "$PGUSER" -d "$PGDATABASE" -f "$D2S_PSQL_SCRIPT_SECONDARY"
fi
