#!/usr/bin/env bash
# test-workspace.sh — Smoke-test every workspace POC endpoint.
#
# Usage:
#   ./scripts/test-workspace.sh <TOKEN>
#   ACCESS_TOKEN=<TOKEN> ./scripts/test-workspace.sh
#
# The token can come from get-token.sh:
#   TOKEN=$(./scripts/get-token.sh CLIENT_ID CLIENT_SECRET)
#   ./scripts/test-workspace.sh "$TOKEN"
#
# All requests target http://localhost:8083 by default.
# Override with BASE_URL env var.

set -euo pipefail

# ── Token resolution ──────────────────────────────────────────────────────────
TOKEN="${1:-${ACCESS_TOKEN:-}}"

if [[ -z "$TOKEN" ]]; then
  echo "Usage: $0 <TOKEN>" >&2
  echo "  or:  ACCESS_TOKEN=<TOKEN> $0" >&2
  echo "" >&2
  echo "Obtain a token first:" >&2
  echo "  TOKEN=\$(./scripts/get-token.sh CLIENT_ID CLIENT_SECRET)" >&2
  exit 1
fi

# ── Config ────────────────────────────────────────────────────────────────────
BASE_URL="${BASE_URL:-http://localhost:8083}"

# ── Colour helpers ────────────────────────────────────────────────────────────
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

PASS=0
FAIL=0

# ── Test runner ───────────────────────────────────────────────────────────────
# run_test <label> <method> <url> [extra curl args...]
#
# Shows:
#   green  — 2xx response
#   yellow — 4xx response (auth/permission failure, expected in some cases)
#   red    — 5xx response or curl failure
#
# Always prints HTTP status code and the full response body.
run_test() {
  local label="$1"
  local method="$2"
  local url="$3"
  shift 3
  local extra_args=("$@")

  local tmp_body
  tmp_body=$(mktemp)
  # Ensure temp file is removed even if the script exits early.
  # shellcheck disable=SC2064
  trap "rm -f $tmp_body" EXIT

  local actual_status
  actual_status=$(curl --silent \
    --write-out "%{http_code}" \
    --output "$tmp_body" \
    --request "$method" \
    "${extra_args[@]}" \
    "$url" 2>/dev/null) || actual_status="000"

  local body
  body=$(cat "$tmp_body" 2>/dev/null || echo "")
  rm -f "$tmp_body"

  # Colour by HTTP status class.
  local color
  if [[ "$actual_status" =~ ^2 ]]; then
    color="$GREEN"
    (( PASS++ )) || true
  elif [[ "$actual_status" =~ ^4 ]]; then
    color="$YELLOW"
    (( FAIL++ )) || true
  else
    color="$RED"
    (( FAIL++ )) || true
  fi

  printf "  ${color}[HTTP %s]${RESET} %s\n" "$actual_status" "$label"
  if [[ -n "$body" ]]; then
    # Indent the body for readability; limit to 300 chars to avoid flooding.
    printf "           %s\n" "$(echo "$body" | head -c 300)"
  fi
  echo
}

AUTH_HEADER="Authorization: Bearer ${TOKEN}"

# ── Test suite ────────────────────────────────────────────────────────────────
echo
printf "${BOLD}${CYAN}Workspace POC — endpoint smoke tests${RESET}\n"
printf "${CYAN}Target: %s${RESET}\n" "$BASE_URL"
printf "${CYAN}Legend: ${GREEN}2xx${RESET}${CYAN} = success  ${YELLOW}4xx${RESET}${CYAN} = auth/permission  ${RED}5xx/000${RESET}${CYAN} = error${RESET}\n\n"

printf "${BOLD}Public endpoints${RESET}\n"
run_test "GET /health (no auth)" \
  GET "${BASE_URL}/health"

printf "${BOLD}Authenticated endpoints${RESET}\n"
run_test "GET /api/me" \
  GET "${BASE_URL}/api/me" \
  --header "$AUTH_HEADER"

printf "${BOLD}Docs service${RESET}\n"
run_test "GET  /api/docs  (requires scope docs:read OR role viewer/editor/owner)" \
  GET "${BASE_URL}/api/docs" \
  --header "$AUTH_HEADER"

run_test "POST /api/docs  (requires scope docs:write OR role editor/owner)" \
  POST "${BASE_URL}/api/docs" \
  --header "$AUTH_HEADER" \
  --header "Content-Type: application/json" \
  --data '{"title":"Test Document from smoke test"}'

run_test "DELETE /api/docs (requires role owner — strict, no scope fallback)" \
  DELETE "${BASE_URL}/api/docs?id=doc-001" \
  --header "$AUTH_HEADER"

printf "${BOLD}Drive service${RESET}\n"
run_test "GET  /api/drive         (requires scope drive:read OR role viewer/editor/owner)" \
  GET "${BASE_URL}/api/drive" \
  --header "$AUTH_HEADER"

run_test "POST /api/drive/upload  (requires scope drive:write OR role editor/owner)" \
  POST "${BASE_URL}/api/drive/upload" \
  --header "$AUTH_HEADER" \
  --header "Content-Type: application/json" \
  --data '{"name":"smoke-test.txt","mime_type":"text/plain"}'

printf "${BOLD}Admin service${RESET}\n"
run_test "GET  /api/admin/users   (requires role admin — strict, no scope fallback)" \
  GET "${BASE_URL}/api/admin/users" \
  --header "$AUTH_HEADER"

# ── Summary ───────────────────────────────────────────────────────────────────
TOTAL=$(( PASS + FAIL ))
printf "${BOLD}Results: %d/%d responded (2xx)${RESET}\n" "$PASS" "$TOTAL"
echo
echo "Note: 4xx responses may be expected if the token lacks the required"
echo "      role or scope for that endpoint (e.g. admin, owner).  Check"
echo "      the Zitadel console to assign roles to your service account."
