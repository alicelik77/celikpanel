#!/usr/bin/env bash
# The Go member service (internal/membership, cmd/membership) and the payment
# package (internal/payment): their own tests, the PHP comparison tests when PHP 8.3
# is present (CELIKPANEL_MEMBERSHIP_PHP=php), and portal-membership's HTTP flow test
# run unchanged against the Go server. Throwaway keys only; nothing leaves the
# loopback.
#
# Go uye hizmeti ve odeme paketi: kendi testleri, PHP varsa PHP ile karsilastirma
# testleri ve portal-membership HTTP akis testinin Go sunucusuna degistirilmeden
# kosulmasi. Yalniz atilacak anahtarlar; hicbir istek yerel makineden cikmaz.
set -euo pipefail
repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo"
work=$(mktemp -d /tmp/celikpanel-membership-go.XXXXXXXX)
server_pid=
cleanup() {
    if [[ -n "$server_pid" ]]; then kill "$server_pid" 2>/dev/null || true; wait "$server_pid" 2>/dev/null || true; fi
    case "$work" in /tmp/celikpanel-membership-go.*) rm -rf -- "$work" ;; esac
}
trap cleanup EXIT

if [[ -n "${CELIKPANEL_MEMBERSHIP_PHP:-}" ]]; then
    printf 'PHP comparison tests: on (%s)\n' "$("$CELIKPANEL_MEMBERSHIP_PHP" -r 'echo PHP_VERSION;')"
    # Required from here: a comparison test that would skip fails instead, whatever its name.
    export CELIKPANEL_MEMBERSHIP_REQUIRE_PHP=1
elif [[ "${CELIKPANEL_MEMBERSHIP_REQUIRE_PHP:-}" == 1 ]]; then
    printf '%s\n' 'The PHP comparison is required but CELIKPANEL_MEMBERSHIP_PHP is not set.' >&2
    exit 1
else
    printf '%s\n' 'PHP comparison tests: off (set CELIKPANEL_MEMBERSHIP_PHP=php to run them)'
fi
go test -count=1 -v ./internal/membership ./cmd/membership ./internal/payment ./internal/licensing 2>&1 \
    | tee "$work/go-test.log" | { grep -E '^(--- FAIL|FAIL|ok|PASS|panic|\s+[a-z_]+\.go:[0-9]+:)|^--- PASS: TestPHP|SKIP: TestPHP' || true; }
grep -qE '^(--- FAIL|FAIL)' "$work/go-test.log" && exit 1
if [[ -n "${CELIKPANEL_MEMBERSHIP_PHP:-}" ]] && grep -qE -- '--- SKIP: TestPHP' "$work/go-test.log"; then
    printf '%s\n' 'A PHP comparison test was skipped although PHP was given.' >&2
    exit 1
fi

go build -o "$work/membership" ./cmd/membership
CELIKPANEL_MEMBERSHIP_PREPARE="$work/http" go test -count=1 -run '^TestPrepareHTTPFixture$' ./internal/membership >/dev/null
CELIKPANEL_MEMBERSHIP_PRIVATE="$work/http" "$work/membership" serve -listen 127.0.0.1:8379 \
    -test-signing-public-key "$(cat "$work/http.public-key")" >"$work/server.log" 2>&1 &
server_pid=$!
for attempt in {1..50}; do
    kill -0 "$server_pid" || { cat "$work/server.log"; exit 1; }
    if curl -fsS http://127.0.0.1:8379/account/ >/dev/null 2>&1; then break; fi
    sleep 0.1
done
python3 portal-membership/tests/http_flow_test.py "$work/http"
