#!/bin/bash
set -euo pipefail

# migrate.sh — Migrate namespace_admin cus_perm for category:manage + attribute:manage
#
# Adds bit 18 (category:manage) and bit 19 (attribute:manage) to cus_perm
# for all namespace_admin users in IAM.
#
# Usage:
#   ./migrate.sh                    # interactive (prompts before each update)
#   ./migrate.sh --yes              # non-interactive (skip confirmation)
#
# Env vars (or reads from .env in same directory):
#   BEACONIAM_INTERNAL_URL  — IAM internal URL (default: http://localhost:5561)
#   IAM_CLIENT_ID           — IAM client ID (default: tuneloop)
#   IAM_CLIENT_SECRET       — IAM client secret (also reads IAM_SECRET alias)
#   IAM_NAMESPACE           — IAM namespace (default: tuneloop)

DRY_RUN=true
if [[ "${1:-}" == "--yes" ]]; then
  DRY_RUN=false
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ENV_FILE="$SCRIPT_DIR/.env"

# Load .env if it exists
if [[ -f "$ENV_FILE" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a
fi

IAM_URL="${BEACONIAM_INTERNAL_URL:-http://localhost:5561}"
CLIENT_ID="${IAM_CLIENT_ID:-tuneloop}"
CLIENT_SECRET="${IAM_CLIENT_SECRET:-${IAM_SECRET:-}}"
NAMESPACE="${IAM_NAMESPACE:-tuneloop}"

if [[ -z "$CLIENT_SECRET" ]]; then
  echo "ERROR: IAM_CLIENT_SECRET (or IAM_SECRET) is required (set env var or add to .env)"
  exit 1
fi

# Bits for category:manage (18) and attribute:manage (19)
NEW_BITS_MASK=$(( (1 << 18) | (1 << 19) ))  # 786432

echo "============================================"
echo "  Namespace Admin cus_perm Migration"
echo "============================================"
echo "IAM URL:        $IAM_URL"
echo "Client ID:      $CLIENT_ID"
echo "Bits to set:    18 (category:manage) + 19 (attribute:manage)"
echo "Bitmask:        $NEW_BITS_MASK"
echo "Mode:           $([ "$DRY_RUN" = true ] && echo 'DRY RUN (no changes)' || echo 'LIVE')"
echo "============================================"
echo ""

# Step 1: Get client token
echo "[1/4] Getting IAM client token..."
TOKEN_RESP=$(curl -sf -X POST "$IAM_URL/api/v1/auth/token" \
  -H 'Content-Type: application/json' \
  -d "{\"grant_type\":\"client_credentials\",\"client_id\":\"$CLIENT_ID\",\"client_secret\":\"$CLIENT_SECRET\"}" 2>&1) || {
  echo "ERROR: Failed to get client token from IAM"
  echo "  Response: $TOKEN_RESP"
  exit 1
}

TOKEN=$(echo "$TOKEN_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('access_token',''))" 2>/dev/null)
if [[ -z "$TOKEN" ]]; then
  echo "ERROR: No access_token in response"
  echo "  Response: $TOKEN_RESP"
  exit 1
fi
echo "  Token acquired."

# Step 2: Find the namespace organization
echo ""
echo "[2/4] Finding namespace organization..."
NS_RESP=$(curl -sf "$IAM_URL/api/v1/namespaces/$NAMESPACE" \
  -H "Authorization: Bearer $TOKEN" 2>&1) || {
  echo "ERROR: Failed to get namespace info"
  echo "  Response: $NS_RESP"
  exit 1
}

NS_ORG_ID=$(echo "$NS_RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('id','') or d.get('data',{}).get('id',''))" 2>/dev/null)
if [[ -z "$NS_ORG_ID" ]]; then
  echo "ERROR: Could not extract namespace org ID"
  echo "  Response: $NS_RESP"
  exit 1
fi
echo "  Namespace org ID: $NS_ORG_ID"

# Step 3: List users in the namespace org and find admins
echo ""
echo "[3/4] Listing users in namespace org..."
USERS_RESP=$(curl -sf "$IAM_URL/api/v1/organizations/$NS_ORG_ID/users" \
  -H "Authorization: Bearer $TOKEN" 2>&1) || {
  echo "ERROR: Failed to list org users"
  echo "  Response: $USERS_RESP"
  exit 1
}

# Parse users with role OWNER or namespace_admin
ADMIN_USERS=$(echo "$USERS_RESP" | python3 -c "
import sys, json
data = json.load(sys.stdin)
users = data.get('users', data.get('data', []))
if isinstance(users, dict):
    users = users.get('users', users.get('data', []))
for u in users:
    role = u.get('role', '').upper()
    if role in ('OWNER', 'NAMESPACE_ADMIN') or 'namespace_admin' in str(u.get('roles', [])):
        uid = u.get('id', u.get('user_id', ''))
        email = u.get('email', '')
        cus_perm = u.get('cus_perm', 0)
        print(f'{uid}\t{email}\t{cus_perm}')
" 2>/dev/null)

if [[ -z "$ADMIN_USERS" ]]; then
  echo "  No namespace_admin users found. Nothing to migrate."
  exit 0
fi

echo "  Found namespace_admin users:"
echo "  USER_ID                               EMAIL                      CUS_PERM"
echo "$ADMIN_USERS" | while IFS=$'\t' read -r uid email cus_perm; do
  printf "  %-38s %-27s %s\n" "$uid" "$email" "$cus_perm"
done

# Step 4: Update each admin's cus_perm
echo ""
echo "[4/4] Updating cus_perm..."

UPDATED=0
SKIPPED=0

while IFS=$'\t' read -r uid email cus_perm; do
  CURRENT_PERM=$((cus_perm))
  NEW_PERM=$(( CURRENT_PERM | NEW_BITS_MASK ))

  if [[ $CURRENT_PERM -eq $NEW_PERM ]]; then
    echo "  SKIP $email — bits 18+19 already set (cus_perm=$CURRENT_PERM)"
    SKIPPED=$((SKIPPED + 1))
    continue
  fi

  echo "  UPDATE $email: cus_perm $CURRENT_PERM -> $NEW_PERM (adding bits 18+19)"

  if [[ "$DRY_RUN" = true ]]; then
    echo "    [DRY RUN] Would set cus_perm=$NEW_PERM for user $uid in org $NS_ORG_ID"
    UPDATED=$((UPDATED + 1))
    continue
  fi

  RESP=$(curl -sf -X PUT "$IAM_URL/api/v1/organizations/$NS_ORG_ID/users/$uid/customer-permissions" \
    -H "Authorization: Bearer $TOKEN" \
    -H 'Content-Type: application/json' \
    -d "{\"raw_bits\":true,\"cus_perm\":$NEW_PERM}" 2>&1) || {
    echo "    ERROR: Failed to update $email"
    echo "    Response: $RESP"
    continue
  }

  echo "    OK — $email updated"
  UPDATED=$((UPDATED + 1))
done <<< "$ADMIN_USERS"

echo ""
echo "============================================"
echo "  Migration complete"
echo "  Updated: $UPDATED  Skipped: $SKIPPED"
if [[ "$DRY_RUN" = true ]]; then
  echo "  (DRY RUN — no changes were made)"
  echo "  Run with --yes to apply changes."
fi
echo "============================================"
