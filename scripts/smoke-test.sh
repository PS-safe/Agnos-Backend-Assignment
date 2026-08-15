#!/usr/bin/env sh
set -eu

compose_project="agnos-smoke-test-$$"
smoke_port="${SMOKE_APP_PORT:-18080}"
response_dir="$(mktemp -d)"

export COMPOSE_PROJECT_NAME="$compose_project"
export APP_PORT="$smoke_port"
export POSTGRES_DB="agnos"
export POSTGRES_USER="agnos"
export POSTGRES_PASSWORD="${SMOKE_POSTGRES_PASSWORD:-smoke-only-database-password}"
export JWT_SECRET="${SMOKE_JWT_SECRET:-smoke-only-jwt-secret-containing-at-least-32-characters}"

cleanup() {
    exit_code=$?
    trap - EXIT INT TERM
    if [ "$exit_code" -ne 0 ]; then
        docker compose logs --no-color || true
    fi
    docker compose down -v --remove-orphans >/dev/null 2>&1 || true
    rm -rf "$response_dir"
    exit "$exit_code"
}
trap cleanup EXIT INT TERM

fail() {
    echo "smoke test failed: $1" >&2
    if [ -f "$response_dir/response.json" ]; then
        cat "$response_dir/response.json" >&2
        echo >&2
    fi
    exit 1
}

request_status() {
    curl -sS -o "$response_dir/response.json" -w '%{http_code}' "$@"
}

expect_status() {
    label=$1
    expected=$2
    shift 2
    actual="$(request_status "$@")"
    if [ "$actual" != "$expected" ]; then
        fail "$label expected HTTP $expected but received $actual"
    fi
    echo "$label -> HTTP $expected"
}

docker compose up -d --build

attempt=0
until curl -fsS "http://localhost:$smoke_port/health" >/dev/null 2>&1; do
    attempt=$((attempt + 1))
    if [ "$attempt" -ge 60 ]; then
        fail "service did not become healthy within 60 seconds"
    fi
    sleep 1
done

curl -fsS -D "$response_dir/headers.txt" -o "$response_dir/health.json" \
    "http://localhost:$smoke_port/health"
grep -qi '^X-Request-ID:' "$response_dir/headers.txt" || fail "health response is missing X-Request-ID"
grep -qi '^Cache-Control: no-store' "$response_dir/headers.txt" || fail "health response is missing Cache-Control: no-store"
echo "health and response headers -> verified"

expect_status "search without token" 401 \
    "http://localhost:$smoke_port/patient/search?first_name=Smoke"

expect_status "staff creation" 201 \
    -X POST -H 'Content-Type: application/json' \
    -d '{"username":"smoke.tester","password":"Smoke-only-pass-2026!","hospital":"hospital-a"}' \
    "http://localhost:$smoke_port/staff/create"

login_status="$(request_status \
    -X POST -H 'Content-Type: application/json' \
    -d '{"username":"smoke.tester","password":"Smoke-only-pass-2026!","hospital":"hospital-a"}' \
    "http://localhost:$smoke_port/staff/login")"
[ "$login_status" = "200" ] || fail "staff login expected HTTP 200 but received $login_status"

access_token="$(sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p' "$response_dir/response.json")"
[ -n "$access_token" ] || fail "login response did not contain an access token"
echo "staff login and JWT issuance -> verified"

expect_status "empty search rejection" 400 \
    -H "Authorization: Bearer $access_token" \
    "http://localhost:$smoke_port/patient/search"

docker compose exec -T postgres psql \
    -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 \
    -c "INSERT INTO patients (hospital_id, first_name_en, last_name_en, date_of_birth, patient_hn, national_id, gender, source_system, source_updated_at) SELECT id, 'Smoke', 'Tester', DATE '1990-01-02', 'SMOKE-HN-001', '7777777777777', 'F', 'compose-smoke-test', now() FROM hospitals WHERE code = 'hospital-a';" \
    >/dev/null

expect_status "authenticated patient search" 200 \
    -H "Authorization: Bearer $access_token" \
    "http://localhost:$smoke_port/patient/search?first_name=Smoke&last_name=Tester"
grep -q '"patient_hn":"SMOKE-HN-001"' "$response_dir/response.json" \
    || fail "patient search did not return the synthetic hospital-scoped record"

echo "Docker Compose smoke test passed on port $smoke_port"
