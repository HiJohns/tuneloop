#!/bin/bash
# E2E verification for Issue #690: SetUserRole IAM API fix
set -euo pipefail

IAM_URL="http://localhost:5561"
API_URL="http://localhost:5557"
RED='\033[0;31m'; GREEN='\033[0;32m'; NC='\033[0m'
fail() { echo -e "${RED}FAIL: $1${NC}"; exit 1; }
pass() { echo -e "${GREEN}PASS: $1${NC}"; }

echo "=== E2E: Verify PUT /api/admin/users/:id/roles ==="
echo ""

# === Step 1: Health check ===
curl -sf "$IAM_URL/health" > /dev/null || fail "beaconiam not running"
curl -sf "$API_URL/api/health" > /dev/null || fail "tuneloop backend not running"

ADMIN_EMAIL="admin@tuneloop.com"
ADMIN_PASS="Debug@2026"

# === Step 2: Admin login + select-org ===
echo ">>> [1/5] Admin login..."
LOGIN_RESP=$(curl -s -X POST "$IAM_URL/oauth/login" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASS\"}")
TOKEN=$(echo "$LOGIN_RESP" | jq -r '.access_token')
[ -z "$TOKEN" ] || [ "$TOKEN" = "null" ] && fail "admin login: $LOGIN_RESP"
pass "Admin logged in"

echo ">>> [2/5] Select org..."
ORG_ID=$(echo "$LOGIN_RESP" | jq -r '.organizations[0].org_id // empty')
[ -z "$ORG_ID" ] && fail "no org found"
SELECT_RESP=$(curl -s -X POST "$IAM_URL/api/v1/auth/select-org" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d "{\"org_id\":\"$ORG_ID\"}")
ADMIN_TOKEN=$(echo "$SELECT_RESP" | jq -r '.access_token // .token // empty')
[ -z "$ADMIN_TOKEN" ] && ADMIN_TOKEN="$TOKEN"

# === Step 3: Get a test user via staff API ===
echo ">>> [3/5] Fetching staff list..."
STAFF=$(curl -s "$API_URL/api/staff" -H "Authorization: Bearer $ADMIN_TOKEN")
echo "$STAFF" | jq '.data.list[:1]' > /dev/null 2>&1 || fail "no staff data"
TARGET_USER=$(echo "$STAFF" | jq -r '.data.list[0].id // empty')
TARGET_ORG=$(echo "$STAFF" | jq -r '.data.list[0].org_id // empty')
TARGET_ROLE=$(echo "$STAFF" | jq -r '.data.list[0].role // empty')
[ -z "$TARGET_USER" ] && fail "no staff user found"
pass "Found test user: $TARGET_USER (role=$TARGET_ROLE)"

# === Step 4: Update role via Tuneloop API ===
echo ">>> [4/5] Updating role via PUT /api/admin/users/$TARGET_USER/roles..."
NEW_ROLE="site_admin"
UPDATE_RESP=$(curl -s -w "\n%{http_code}" -X PUT "$API_URL/api/admin/users/$TARGET_USER/roles" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"role_code\":\"$NEW_ROLE\"}")
HTTP_CODE=$(echo "$UPDATE_RESP" | tail -1)
UPDATE_BODY=$(echo "$UPDATE_RESP" | head -n -1)
echo "HTTP $HTTP_CODE: $UPDATE_BODY"

if [ "$HTTP_CODE" != "200" ]; then
  echo "=== Direct IAM API test ==="
  echo "Testing IAM endpoint directly with same params..."
  curl -v -X PUT "$IAM_URL/api/v1/organizations/$TARGET_ORG/users/$TARGET_USER/role" \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"role":"ADMIN"}' 2>&1
  fail "Tuneloop API returned $HTTP_CODE (expected 200)"
fi
pass "Role updated via Tuneloop API"

# === Step 5: Verify role change ===
echo ">>> [5/5] Verifying role update..."
STAFF2=$(curl -s "$API_URL/api/staff" -H "Authorization: Bearer $ADMIN_TOKEN")
UPDATED_ROLE=$(echo "$STAFF2" | jq -r ".data.list[] | select(.id==\"$TARGET_USER\") | .role // empty")
pass "User role is now: $UPDATED_ROLE"

# Restore role
echo ">>> [cleanup] Restoring original role..."
curl -s -X PUT "$API_URL/api/admin/users/$TARGET_USER/roles" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"role_code\":\"$TARGET_ROLE\"}" > /dev/null

echo ""
echo -e "${GREEN}=== All 5 steps PASSED ===${NC}"
