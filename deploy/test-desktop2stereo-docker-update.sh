#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
UPDATE_SCRIPT="$SCRIPT_DIR/desktop2stereo-docker-update.sh"
TMP_BASE="$(printenv TMPDIR || true)"
if [[ -z "$TMP_BASE" ]]; then
  TMP_BASE="/tmp"
fi
TEST_ROOT="$(mktemp -d "$TMP_BASE/d2s-docker-update-test.XXXXXX")"
case "$TEST_ROOT" in
  "$TMP_BASE"/d2s-docker-update-test.*) ;;
  *) echo "unexpected test root: $TEST_ROOT" >&2; exit 1 ;;
esac

cleanup() {
  case "$TEST_ROOT" in
    "$TMP_BASE"/d2s-docker-update-test.*) rm -rf -- "$TEST_ROOT" ;;
    *) echo "refusing to remove unexpected test root: $TEST_ROOT" >&2 ;;
  esac
}
trap cleanup EXIT

TARGET_SHA="0123456789abcdef0123456789abcdef01234567"
OTHER_SHA="fedcba9876543210fedcba9876543210fedcba98"
MOCK_BIN="$TEST_ROOT/mock-bin"
ARCHIVE_ROOT="$TEST_ROOT/archive-root"
PYTHON_SHIM="$TEST_ROOT/python3-shim.js"
mkdir -p "$MOCK_BIN" "$ARCHIVE_ROOT/release"
printf 'services: {}\n' > "$ARCHIVE_ROOT/release/docker-compose.yml"
tar -czf "$TEST_ROOT/release.tar.gz" -C "$ARCHIVE_ROOT" release

cat > "$PYTHON_SHIM" <<'NODE'
const fs = require("node:fs");
const args = process.argv.slice(2);

if (args[0] === "-c") {
  let input = "";
  process.stdin.setEncoding("utf8");
  process.stdin.on("data", (chunk) => { input += chunk; });
  process.stdin.on("end", () => {
    try {
      const payload = JSON.parse(input);
      const version = payload && payload.success === true && payload.data &&
        typeof payload.data.version === "string" ? payload.data.version : "";
      if (!version) process.exitCode = 1;
      else process.stdout.write(version);
    } catch {
      process.exitCode = 1;
    }
  });
} else if (args[0] === "-" && args[1]) {
  const payload = {
    state: process.env.STATUS_STATE,
    phase: process.env.STATUS_PHASE,
    message: process.env.STATUS_MESSAGE,
    sha: process.env.STATUS_SHA,
    updated_at: process.env.STATUS_UPDATED_AT
  };
  if (process.env.STATUS_ERROR) payload.error = process.env.STATUS_ERROR;
  fs.writeFileSync(args[1], JSON.stringify(payload) + "\n", "utf8");
} else {
  process.exitCode = 2;
}
NODE

cat > "$MOCK_BIN/python3" <<'SH'
#!/usr/bin/env bash
exec node "$D2S_TEST_PYTHON_SHIM" "$@"
SH

cat > "$MOCK_BIN/curl" <<'SH'
#!/usr/bin/env bash
set -Eeuo pipefail
url="$(printf '%s\n' "$@" | tail -n 1)"

if [[ "$url" == https://codeload.github.com/* ]]; then
  output=""
  headers=""
  while (($#)); do
    case "$1" in
      -o) output="$2"; shift 2 ;;
      --dump-header) headers="$2"; shift 2 ;;
      *) shift ;;
    esac
  done
  cp "$D2S_TEST_ARCHIVE" "$output"
  if [[ -n "$headers" ]]; then
    size="$(stat -c %s "$output")"
    printf 'HTTP/1.1 200 OK\r\nContent-Length: %s\r\n\r\n' "$size" > "$headers"
  fi
  count="$(cat "$D2S_TEST_DOWNLOAD_COUNT" 2>/dev/null || printf '0')"
  printf '%s\n' "$((count + 1))" > "$D2S_TEST_DOWNLOAD_COUNT"
  exit 0
fi

case "$D2S_TEST_STATUS_MODE" in
  unavailable) exit 22 ;;
  malformed) printf '{' ;;
  *) printf '{"success":true,"data":{"version":"%s"}}\n' "$D2S_TEST_RUNTIME_VERSION" ;;
esac
SH

cat > "$MOCK_BIN/docker" <<'SH'
#!/usr/bin/env bash
set -Eeuo pipefail
printf '%s\n' "$*" >> "$D2S_TEST_DOCKER_LOG"
if [[ "${D2S_TEST_FAIL_PHASE:-}" == "audit_migration" && "$*" == *"audit-migrate"* ]]; then
  exit 42
fi
if [[ "$1" == "inspect" ]]; then
  printf 'healthy\n'
fi
SH

cat > "$MOCK_BIN/ln" <<'SH'
#!/usr/bin/env bash
exit 0
SH

cat > "$MOCK_BIN/sleep" <<'SH'
#!/usr/bin/env bash
exit 0
SH

chmod +x "$MOCK_BIN/python3" "$MOCK_BIN/curl" "$MOCK_BIN/docker" "$MOCK_BIN/ln" "$MOCK_BIN/sleep"

cat > "$TEST_ROOT/backup" <<'SH'
#!/usr/bin/env bash
exit 0
SH
chmod +x "$TEST_ROOT/backup"

prepare_case() {
  local name="$1"
  local root="$TEST_ROOT/cases/$name"
  mkdir -p "$root/app/logs" "$root/app/update-requests" "$root/releases"
  : > "$root/app/.env"
  : > "$root/docker.log"
  printf '0\n' > "$root/download-count"
}

run_update() {
  local name="$1"
  local runtime_version="$2"
  local status_mode="$3"
  local root="$TEST_ROOT/cases/$name"

  D2S_DOCKER_APP_ROOT="$root/app" \
  D2S_DOCKER_RELEASE_ROOT="$root/releases" \
  D2S_UPDATE_BACKUP_SCRIPT="$TEST_ROOT/backup" \
  D2S_UPDATE_STATUS_FILE="$root/app/update-requests/status.json" \
  D2S_DOCKER_STATUS_URL="http://mock.invalid/api/status" \
  D2S_TEST_ARCHIVE="$TEST_ROOT/release.tar.gz" \
  D2S_TEST_PYTHON_SHIM="$PYTHON_SHIM" \
  D2S_TEST_DOWNLOAD_COUNT="$root/download-count" \
  D2S_TEST_DOCKER_LOG="$root/docker.log" \
  D2S_TEST_RUNTIME_VERSION="$runtime_version" \
  D2S_TEST_STATUS_MODE="$status_mode" \
  D2S_TEST_FAIL_PHASE="${4:-}" \
  PATH="$MOCK_BIN:$PATH" \
    "$UPDATE_SCRIPT" "$TARGET_SHA" test/repository main > "$root/run.log" 2>&1
}

assert_status() {
  local name="$1"
  local expected="$2"
  local file="$TEST_ROOT/cases/$name/app/update-requests/status.json"
  if ! grep -Fq "\"state\":\"$expected\"" "$file" && ! grep -Fq "\"state\": \"$expected\"" "$file"; then
    echo "expected status $expected for $name" >&2
    cat "$file" >&2
    return 1
  fi
}

prepare_case success
if ! run_update success "$TARGET_SHA" ok; then
  cat "$TEST_ROOT/cases/success/run.log" >&2
  exit 1
fi
assert_status success succeeded
[[ -f "$TEST_ROOT/cases/success/releases/$TARGET_SHA/.update-complete" ]]
[[ -f "$TEST_ROOT/cases/success/releases/$TARGET_SHA/docker-compose.yml" ]]
[[ "$(cat "$TEST_ROOT/cases/success/download-count")" == "1" ]]
grep -Fq -- "--profile maintenance run --rm --no-deps audit-ownership" "$TEST_ROOT/cases/success/docker.log"
grep -Fq -- "--profile maintenance run --rm --no-deps audit-migrate" "$TEST_ROOT/cases/success/docker.log"
grep -Fq -- "--profile maintenance run --rm --no-deps audit-permissions" "$TEST_ROOT/cases/success/docker.log"
ownership_line="$(grep -nF -- 'audit-ownership' "$TEST_ROOT/cases/success/docker.log" | head -n 1 | cut -d: -f1)"
migration_line="$(grep -nF -- 'audit-migrate' "$TEST_ROOT/cases/success/docker.log" | head -n 1 | cut -d: -f1)"
permissions_line="$(grep -nF -- 'audit-permissions' "$TEST_ROOT/cases/success/docker.log" | head -n 1 | cut -d: -f1)"
restart_line="$(grep -nF -- 'up -d --no-deps new-api' "$TEST_ROOT/cases/success/docker.log" | head -n 1 | cut -d: -f1)"
(( ownership_line < migration_line && migration_line < permissions_line && permissions_line < restart_line ))

if ! run_update success "$TARGET_SHA" ok; then
  cat "$TEST_ROOT/cases/success/run.log" >&2
  exit 1
fi
assert_status success succeeded
[[ "$(cat "$TEST_ROOT/cases/success/download-count")" == "1" ]]

if run_update success "$OTHER_SHA" ok; then
  echo "stale release marker incorrectly reported success" >&2
  exit 1
else
  exit_code=$?
  [[ "$exit_code" == "8" ]]
fi
assert_status success failed
grep -Fq "tag new-api-desktop2stereo-site:pre-$TARGET_SHA new-api-desktop2stereo-site:local" "$TEST_ROOT/cases/success/docker.log"

prepare_case mismatch
if run_update mismatch "$OTHER_SHA" ok; then
  echo "runtime SHA mismatch incorrectly reported success" >&2
  exit 1
else
  exit_code=$?
  [[ "$exit_code" == "8" ]]
fi
assert_status mismatch failed
grep -Fq "tag new-api-desktop2stereo-site:pre-$TARGET_SHA new-api-desktop2stereo-site:local" "$TEST_ROOT/cases/mismatch/docker.log"

prepare_case unavailable
if run_update unavailable "$TARGET_SHA" unavailable; then
  echo "unavailable status endpoint incorrectly reported success" >&2
  exit 1
else
  exit_code=$?
  [[ "$exit_code" == "8" ]]
fi
assert_status unavailable failed
grep -Fq "tag new-api-desktop2stereo-site:pre-$TARGET_SHA new-api-desktop2stereo-site:local" "$TEST_ROOT/cases/unavailable/docker.log"

prepare_case migration-failure
if run_update migration-failure "$TARGET_SHA" ok audit_migration; then
  echo "audit migration failure incorrectly proceeded with deployment" >&2
  exit 1
else
  exit_code=$?
  [[ "$exit_code" == "42" ]]
fi
assert_status migration-failure failed
if grep -Fq 'up -d --no-deps new-api' "$TEST_ROOT/cases/migration-failure/docker.log"; then
  echo "application was restarted after audit migration failed" >&2
  exit 1
fi

echo "Docker update SHA gate tests passed."
