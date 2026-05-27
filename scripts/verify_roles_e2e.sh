#!/bin/bash
# E2E verification script for CreateMerchant role initialization (#663)
# Usage: ./scripts/verify_roles_e2e.sh
# Prerequisites: backend running, beaconiam running

set -e

BEACONIAM="${BEACONIAM_INTERNAL_URL:-http://localhost:5561}"
NAMESPACE="${IAM_NAMESPACE:-tuneloop_debug}"
BACKEND_PORT="${TUNELOOP_WWW_PORT:-5557}"

echo "=== Step 1: Get Service Token ==="
TOKEN=$(curl -s -X POST "$BEACONIAM/api/v1/auth/token" \
  -H "Content-Type: application/json" \
  -d "{\"grant_type\":\"client_credentials\",\"client_id\":\"$NAMESPACE\",\"client_secret\":\"$IAM_SECRET\"}" | jq -r '.access_token')
echo "OK: token=${TOKEN:0:20}..."

echo ""
echo "=== Step 2: Get Namespace ID ==="
NS_ID=$(curl -s "$BEACONIAM/api/v1/namespaces/current" \
  -H "Authorization: Bearer $TOKEN" | jq -r '.id')
echo "OK: namespace=$NS_ID"

echo ""
echo "=== Step 3: Create Test User ==="
TEST_EMAIL="e2e_roles_$(date +%s)@tuneloop.com"
USER_ID=$(curl -s -X POST "$BEACONIAM/api/v1/users" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"e2e_roles\",\"name\":\"E2E Test\",\"email\":\"$TEST_EMAIL\",\"callback_url\":\"http://localhost:$BACKEND_PORT\"}" | jq -r '.id // .user_id')
echo "OK: user=$USER_ID"

echo ""
echo "=== Step 4: Create Merchant via Tuneloop API ==="
MERCHANT_RESP=$(curl -s -X POST "http://localhost:$BACKEND_PORT/api/merchants" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"E2E Test Merchant $(date +%s)\",\"admin_uid\":\"$USER_ID\",\"phone\":\"13800000000\"}")
IAM_ORG_ID=$(echo "$MERCHANT_RESP" | jq -r '.data.iam_org_id')
echo "OK: org=$IAM_ORG_ID"

echo ""
echo "=== Step 5: Verify Local DB Roles ==="
docker exec jobmaster-postgres psql -U tuneloop_user -d tuneloop_debug -t -c \
  "SELECT code, name FROM roles WHERE tenant_id = '$IAM_ORG_ID' ORDER BY code;" 2>/dev/null
echo "(EXPECT: 4 rows: owner, admin, staff, worker)"

echo ""
echo "=== Step 6: Verify IAM Role Templates ==="
curl -s "$BEACONIAM/api/v1/namespaces/$NS_ID/role-templates" \
  -H "Authorization: Bearer $TOKEN" | jq '[.[] | {code, name}]'
echo "(EXPECT: merchant_admin with sys_perm + cus_perm)"

echo ""
echo "=== Step 7: Login as Admin User, Verify JWT ==="
ADMIN_TOKEN=$(curl -s -X POST "$BEACONIAM/api/v1/auth/token" \
  -H "Content-Type: application/json" \
  -d "{\"grant_type\":\"password\",\"username\":\"$TEST_EMAIL\",\"password\":\"test\",\"scope\":\"openid\"}" | jq -r '.access_token')
JWT_PAYLOAD=$(echo "$ADMIN_TOKEN" | cut -d. -f2 | base64 -d 2>/dev/null)
echo "$JWT_PAYLOAD" | jq '{sub, tid, roles, sys_perm, cus_perm}'
echo "(EXPECT: sys_perm>0, roles=[\"merchant_admin\"])"

echo ""
echo "=== Step 8: Call Permission Management APIs ==="
curl -s "http://localhost:$BACKEND_PORT/api/admin/roles" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq '{code, count: (.data | length)}'
echo "(EXPECT: 20000, count>=4)"

echo ""
echo "=== ALL CHECKS COMPLETE ==="
