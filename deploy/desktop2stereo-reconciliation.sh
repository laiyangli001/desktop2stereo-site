#!/usr/bin/env bash
set -Eeuo pipefail

APP_ROOT="${D2S_RECONCILIATION_ROOT:-/opt/desktop2stereo-site}"
REPORT_ROOT="${D2S_RECONCILIATION_REPORT_ROOT:-$APP_ROOT/reconciliation-reports}"
BASE_URL="${D2S_RECONCILIATION_BASE_URL:-https://100393.com}"
ADMIN_TOKEN="${D2S_ADMIN_TOKEN:-}"
REPORT_DATE="$(date -u +%Y%m%d)"
REPORT_PATH="$REPORT_ROOT/reconciliation-$REPORT_DATE.json"
RESPONSE_PATH="$REPORT_ROOT/.reconciliation-$REPORT_DATE.response.json"
REPORT_TMP_PATH="$REPORT_PATH.tmp"
LOCK_PATH="$REPORT_ROOT/.reconciliation.lock"

if [[ -z "$ADMIN_TOKEN" ]]; then
  echo "D2S_ADMIN_TOKEN is required" >&2
  exit 2
fi
if [[ "$BASE_URL" != https://* ]]; then
  echo "reconciliation requires an HTTPS base URL" >&2
  exit 2
fi
for command_name in curl date mkdir python3; do
  command -v "$command_name" >/dev/null || {
    echo "required command is missing: $command_name" >&2
    exit 3
  }
done

mkdir -p "$REPORT_ROOT"
if ! mkdir "$LOCK_PATH" 2>/dev/null; then
  echo "another reconciliation run is already active" >&2
  exit 4
fi
cleanup() {
  rm -f -- "$RESPONSE_PATH" "$REPORT_TMP_PATH"
  rmdir -- "$LOCK_PATH" 2>/dev/null || true
}
trap cleanup EXIT

if [[ -f "$REPORT_PATH" ]]; then
  mismatch_count="$(python3 - "$REPORT_PATH" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    payload = json.load(handle)
data = payload.get("data")
if not payload.get("success") or not isinstance(data, dict):
    raise SystemExit("existing reconciliation report is invalid")
mismatches = data.get("mismatches", [])
if not isinstance(mismatches, list):
    raise SystemExit("existing reconciliation report has invalid mismatches")
print(len(mismatches))
PY
)"
  echo "reconciliation report already exists: $REPORT_PATH"
  echo "reconciliation mismatches: $mismatch_count"
  if [[ "$mismatch_count" != "0" ]]; then
    exit 10
  fi
  exit 0
fi

if ! curl --fail --silent --show-error --connect-timeout 10 --max-time 60 \
  --header "Authorization: Bearer $ADMIN_TOKEN" \
  --output "$RESPONSE_PATH" \
  "$BASE_URL/api/v1/admin/reconciliation"; then
  echo "reconciliation endpoint request failed" >&2
  exit 5
fi

mismatch_count="$(python3 - "$RESPONSE_PATH" "$REPORT_TMP_PATH" <<'PY'
import json
import sys

response_path, report_tmp_path = sys.argv[1:]
with open(response_path, encoding="utf-8") as handle:
    payload = json.load(handle)
data = payload.get("data")
if not payload.get("success") or not isinstance(data, dict):
    raise SystemExit("reconciliation endpoint returned an unsuccessful response")
mismatches = data.get("mismatches", [])
if not isinstance(mismatches, list):
    raise SystemExit("reconciliation endpoint returned invalid mismatches")
with open(report_tmp_path, "w", encoding="utf-8") as handle:
    json.dump(payload, handle, ensure_ascii=False, indent=2)
    handle.write("\n")
print(len(mismatches))
PY
)"
mv -f -- "$REPORT_TMP_PATH" "$REPORT_PATH"
echo "reconciliation report saved: $REPORT_PATH"
echo "reconciliation mismatches: $mismatch_count"
if [[ "$mismatch_count" != "0" ]]; then
  exit 10
fi
