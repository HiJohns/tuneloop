#!/bin/bash
# E2E verification script for CreateMerchant role initialization (#663)
# Usage: ./scripts/verify_roles_e2e.sh
# Prerequisites: backend running with #663 code, beaconiam running
# IMPORTANT: Restart backend after deploying #663 code before running this

set -e

BEACONIAM="${BEACONIAM_INTERNAL_URL:-http://localhost:5561}"
NAMESPACE="${IAM_NAMESPACE:-tuneloop_debug}"
BACKEND_PORT="${TUNELOOP_WWW_PORT:-5557}"

echo "=== Step 1: Get Service Token ==="
TOKEN=$(curl -s -X POST "$BEACONIAM/api/v1/auth/token" \
  -H "Content-Type: application/json" \
  -d "{\"grant_type\":\"client_credentials\",\"client_id\":\"$NAMESPACE\",\"client_secret\":\"$IAM_SECRET\"}" | jq -r '.access_token')
echo "OK ✓"

echo ""
echo "=== Step 2: Get Namespace ID ==="
NS_ID=$(curl -s "$BEACONIAM/api/v1/namespaces/$NAMESPACE" \
  -H "Authorization: Bearer $TOKEN" | jq -r '.id')
echo "OK ✓ namespace=$NS_ID"

echo ""
echo "=== Step 3: List Role Templates from IAM ==="
TEMPLATES=$(curl -s "$BEACONIAM/api/v1/namespaces/$NS_ID/role-templates" \
  -H "Authorization: Bearer $TOKEN")
echo "$TEMPLATES" | jq '[.[] | {code, sys_perm, cus_perm}]'
COUNT=$(echo "$TEMPLATES" | jq 'length')
echo ""
echo "Result: $COUNT role templates found"
echo "Note: cus_perm=null means role template cus_perm not synced yet"
echo "EXPECT after #663 restart: cus_perm will be set for all templates"

echo ""
echo "=== Step 4: Check Existing Merchant's Local Roles ==="
echo "Checking merchant 'cadenza' (org_id=9a8d2fca-e18b-4797-b142-c9d06aeb81a0)..."
docker exec jobmaster-postgres psql -U tuneloop_user -d tuneloop_debug -t -c \
  "SELECT code, name FROM roles WHERE tenant_id = '9a8d2fca-e18b-4797-b142-c9d06aeb81a0' ORDER BY code;" 2>/dev/null || echo "(DB check requires #663 code deployed + IAM user token for CreateMerchant)"

echo ""
echo "=== Step 5: Verify Backend Startup Logs (manual check) ==="
echo "Check for log entries:"
echo "  '[Bootstrap] Created role template ...'"
echo "  '[Bootstrap] Synced sys_perm for role ...'"
echo "  '[Bootstrap] Synced cus_perm for role ...'"

echo ""
echo "=== Step 6: Check Frontend Menu (manual check) ==="
echo "Login as haidian_admin@tuneloop.com and verify:"
echo "  1. 系统管理 → 权限管理 菜单可见"
echo "  2. 角色管理 Tab 显示 4 个系统角色 (owner/admin/staff/worker)"
echo "  3. 每个角色有 id（可编辑）"

echo ""
echo "=== PREREQUISITE ==="
echo "Full E2E requires backend restart with #663 code deployed."
echo "Steps:"
echo "  1. pkill -f 'go run main.go' (manual)"
echo "  2. cd backend && go run main.go & (manual)"
echo "  3. Run this script again"
echo ""

echo "=== PARTIAL CHECK COMPLETE ==="
