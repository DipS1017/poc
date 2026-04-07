#!/usr/bin/env bash
# get-token.sh — Obtain a client_credentials access token from Zitadel.
#
# Usage:
#   ./scripts/get-token.sh CLIENT_ID CLIENT_SECRET [DOMAIN]
#
# Arguments:
#   CLIENT_ID      OAuth2 client ID   (required)
#   CLIENT_SECRET  OAuth2 client secret (required)
#   DOMAIN         Zitadel base URL   (optional, default: http://localhost:8090)
#
# Output:
#   Prints the raw access_token string to stdout.
#   Diagnostic messages are written to stderr.
#
# Example:
#   TOKEN=$(./scripts/get-token.sh "my-client-id" "my-secret")
#   TOKEN=$(./scripts/get-token.sh "my-client-id" "my-secret" "http://zitadel.example.com")
#   echo "Token: $TOKEN"
#
# No external tools required — uses only curl, grep, and sed.

set -euo pipefail

# ── Usage help ────────────────────────────────────────────────────────────────
if [[ $# -lt 2 ]]; then
  echo "Usage: $0 CLIENT_ID CLIENT_SECRET [DOMAIN]" >&2
  echo "" >&2
  echo "Arguments:" >&2
  echo "  CLIENT_ID      OAuth2 client ID (required)" >&2
  echo "  CLIENT_SECRET  OAuth2 client secret (required)" >&2
  echo "  DOMAIN         Zitadel base URL (default: http://localhost:8090)" >&2
  echo "" >&2
  echo "Example:" >&2
  echo "  TOKEN=\$($0 \"my-client-id\" \"my-secret\")" >&2
  echo "  TOKEN=\$($0 \"my-client-id\" \"my-secret\" \"http://zitadel.example.com\")" >&2
  exit 1
fi

# ── Positional arguments ──────────────────────────────────────────────────────
CLIENT_ID="$1"
CLIENT_SECRET="$2"
DOMAIN="${3:-http://localhost:8090}"

# Strip trailing slash from DOMAIN if present.
DOMAIN="${DOMAIN%/}"

# ── Dependencies check ────────────────────────────────────────────────────────
for cmd in curl grep sed; do
  if ! command -v "$cmd" &>/dev/null; then
    echo "Error: '$cmd' is required but not installed." >&2
    exit 1
  fi
done

# ── Token request ─────────────────────────────────────────────────────────────
TOKEN_URL="${DOMAIN}/oauth/v2/token"

echo "Requesting token from: ${TOKEN_URL}" >&2
echo "Client ID:             ${CLIENT_ID}" >&2

RESPONSE=$(curl --silent --show-error \
  --request POST "${TOKEN_URL}" \
  --header "Content-Type: application/x-www-form-urlencoded" \
  --user "${CLIENT_ID}:${CLIENT_SECRET}" \
  --data-urlencode "grant_type=client_credentials")

# ── Extract access_token without jq ──────────────────────────────────────────
# Matches: "access_token":"<value>" — handles both compact and spaced JSON.
ACCESS_TOKEN=$(echo "$RESPONSE" \
  | grep -o '"access_token"\s*:\s*"[^"]*"' \
  | sed 's/.*"access_token"\s*:\s*"\([^"]*\)".*/\1/')

if [[ -z "$ACCESS_TOKEN" ]]; then
  echo "Error: no access_token found in response." >&2
  echo "Response body:" >&2
  echo "$RESPONSE" >&2
  exit 1
fi

echo "$ACCESS_TOKEN"
