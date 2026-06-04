#!/bin/bash
set -euo pipefail

# push_seafile.sh — Upload test.zip to Seafile.
#
# Runs on the local workstation (NOT the build server).
# SCPs test.zip from the build server, then uploads via Seafile REST API.
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

# Accept optional -l/--local flag to skip scp (use local ~/test.zip)
SKIP_SCP=false
if [ "${1:-}" = "-l" ] || [ "${1:-}" = "--local" ]; then
  SKIP_SCP=true
  shift
fi

echo "=== Step 1: Get test.zip ==="
if [ "$SKIP_SCP" = true ] || [ -f "${HOME}/test.zip" ]; then
  echo "  Using local ~/test.zip"
else
  scp opencode:~/test.zip ~/Downloads/test.zip
  echo "  Downloaded to ~/Downloads/test.zip"
fi

SRC="${HOME}/test.zip"
[ -f "${HOME}/Downloads/test.zip" ] && SRC="${HOME}/Downloads/test.zip"
[ "$SKIP_SCP" = true ] && SRC="${HOME}/test.zip"

echo "=== Step 2: Authenticate with Seafile ==="
AUTH_RESP=$(curl -sS -X POST "${SEAFILE_SERVER_URL}/api2/auth-token/" \
  -d "username=${SEAFILE_USERNAME}&password=${SEAFILE_PASSWORD}")

# Handle both JSON {"token":"..."} and plain-text token responses
if echo "$AUTH_RESP" | grep -q '"token"'; then
  TOKEN=$(echo "$AUTH_RESP" | sed -n 's/.*"token"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
else
  TOKEN=$(echo "$AUTH_RESP" | tr -d '"')
fi

if [ -z "$TOKEN" ] || [ "$TOKEN" = "null" ]; then
  echo "ERROR: Seafile authentication failed"
  echo "  Response: $AUTH_RESP"
  exit 1
fi
echo "  Token obtained"

echo "=== Step 3: Upload to ${SEAFILE_PATH} ==="

TARGET_FILE="${SEAFILE_PATH}/test.zip"

# Clean up existing test.zip before upload (overwrite share link changes, accepted)
echo "  Cleaning old test.zip in ${SEAFILE_PATH}..."
DIR_RESP=$(curl -sS -H "Authorization: Token ${TOKEN}" \
  "${SEAFILE_SERVER_URL}/api2/repos/${SEAFILE_REPO_ID}/dir/?p=${SEAFILE_PATH}")

EXISTING=$(echo "$DIR_RESP" | python3 -c "
import sys,json,urllib.parse
for f in json.load(sys.stdin):
    n = f['name']
    if (n == 'test.zip' or n.startswith('test (') and n.endswith('.zip')):
        encoded = urllib.parse.quote(n)
        sys.stdout.write(encoded + '|' + n + '\n')
" 2>/dev/null)

if [ -n "$EXISTING" ]; then
  while IFS='|' read -r encoded name; do
    echo "    Deleting ${name}..."
    curl -sS -X DELETE -H "Authorization: Token ${TOKEN}" \
      "${SEAFILE_SERVER_URL}/api2/repos/${SEAFILE_REPO_ID}/file/?p=${SEAFILE_PATH}/${encoded}" > /dev/null 2>&1
  done <<< "$EXISTING"
fi

echo "  Uploading test.zip..."
UPLOAD_RESP=$(curl -sS -H "Authorization: Token ${TOKEN}" \
  "${SEAFILE_SERVER_URL}/api2/repos/${SEAFILE_REPO_ID}/upload-link/?p=${SEAFILE_PATH}")

if echo "$UPLOAD_RESP" | grep -q '^"http'; then
  UPLOAD_LINK=$(echo "$UPLOAD_RESP" | sed 's/^"//;s/"$//')
elif echo "$UPLOAD_RESP" | grep -q '^http'; then
  UPLOAD_LINK="$UPLOAD_RESP"
elif echo "$UPLOAD_RESP" | grep -q '"upload_link"'; then
  UPLOAD_LINK=$(echo "$UPLOAD_RESP" | sed -n 's/.*"upload_link"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
else
  echo "ERROR: Failed to get upload link"
  echo "  Response: $UPLOAD_RESP"
  exit 1
fi

if [ -z "$UPLOAD_LINK" ]; then
  echo "ERROR: Empty upload link"
  exit 1
fi

echo "  Upload URL: ${UPLOAD_LINK:0:80}..."

curl -sS -H "Authorization: Token ${TOKEN}" \
  -F "file=@${SRC}" \
  -F "filename=test.zip" \
  -F "parent_dir=${SEAFILE_PATH}" \
  "$UPLOAD_LINK"

echo ""
echo "=== Done ==="
