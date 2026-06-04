#!/bin/bash
set -euo pipefail

# push_seafile.sh — SCP the test.zip from the build server and upload to Seafile.
#
# Required environment variables:
#   SEAFILE_SERVER_URL   e.g. https://cloud.seafile.com
#   SEAFILE_USERNAME     Seafile account email or username
#   SEAFILE_PASSWORD     Seafile account password
#   SEAFILE_REPO_ID      UUID of the Seafile library
#   SEAFILE_PATH         Target directory path inside the library, e.g. /deploy
#
# Usage:
#   export SEAFILE_SERVER_URL=...
#   export SEAFILE_USERNAME=...
#   export SEAFILE_PASSWORD=...
#   export SEAFILE_REPO_ID=...
#   export SEAFILE_PATH=/deploy
#   bash push_seafile.sh

for var in SEAFILE_SERVER_URL SEAFILE_USERNAME SEAFILE_PASSWORD SEAFILE_REPO_ID SEAFILE_PATH; do
  if [ -z "${!var:-}" ]; then
    echo "ERROR: $var is not set"
    exit 1
  fi
done

echo "=== Step 1: Download test.zip from build server ==="
scp opencode:~/test.zip ~/Downloads/test.zip
echo "  Downloaded to ~/Downloads/test.zip"

echo "=== Step 2: Authenticate with Seafile ==="
TOKEN=$(curl -sS -X POST "${SEAFILE_SERVER_URL}/api2/auth-token/" \
  -d "username=${SEAFILE_USERNAME}&password=${SEAFILE_PASSWORD}" | tr -d '"')

if [ -z "$TOKEN" ] || [ "$TOKEN" = "null" ]; then
  echo "ERROR: Seafile authentication failed"
  exit 1
fi
echo "  Token obtained"

echo "=== Step 3: Get upload link ==="
UPLOAD_RESP=$(curl -sS -H "Authorization: Token ${TOKEN}" \
  "${SEAFILE_SERVER_URL}/api2/repos/${SEAFILE_REPO_ID}/upload-link/?p=${SEAFILE_PATH}")

# Response is 'https://...upload...' — strip the surrounding quotes
UPLOAD_LINK=$(echo "$UPLOAD_RESP" | sed 's/^"//;s/"$//')

if [ -z "$UPLOAD_LINK" ] || [ "$UPLOAD_LINK" = "null" ]; then
  echo "ERROR: Failed to get upload link from Seafile"
  echo "  Response: $UPLOAD_RESP"
  exit 1
fi
echo "  Upload URL obtained"

echo "=== Step 4: Upload test.zip to Seafile ==="
# Upload using the returned link (multipart form)
curl -sS -H "Authorization: Token ${TOKEN}" \
  -F "file=@${HOME}/Downloads/test.zip" \
  -F "filename=test.zip" \
  -F "parent_dir=${SEAFILE_PATH}" \
  "$UPLOAD_LINK"

echo ""
echo "=== Done ==="
