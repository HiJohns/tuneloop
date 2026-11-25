#!/bin/bash
set -euo pipefail

# migrate.sh — Migrate namespace_admin cus_perm for category:manage + attribute:manage
#
# Adds bit 18 (category:manage) and bit 19 (attribute:manage) to cus_perm
# for all namespace_admin users in IAM.
#
# >>> usage
# Usage:
#   ./migrate.sh                          # dry-run plan (no changes)
#   ./migrate.sh --yes --confirm-rows N   # apply, after confirming the row count
#   ./migrate.sh --fixture FILE|-         # offline dry-run against a fixture
#                                         # (feature: uid|email|cus_perm per line)
#
# Safety gates (#2490):
#   * records are parsed with a NON-whitespace separator (`|`) so an empty email
#     keeps its column (previously Tab-IFS collapsed it and shifted cus_perm);
#   * field count != 3, empty uid, or a non-numeric cus_perm → fail-fast (exit 3),
#     no write, no silent default to 0;
#   * `--yes` requires `--confirm-rows N` equal to the planned update count;
#   * `--fixture` is dry-run only and mutually exclusive with `--yes`.
#
# Machine-readable keys: PLAN_UPDATES, SKIPPED, BAD_ROW, ROW_FIELDS, BAD_PERM,
#   MIGRATE_INVALID, APPLY_READY, APPLY_CONFIRM_MISMATCH, DB_WRITES,
#   RESULT: PASS|FAIL.
#
# Env vars (or reads from .env in same directory):
#   BEACONIAM_INTERNAL_URL  — IAM internal URL (default: http://localhost:5561)
#   IAM_CLIENT_ID           — IAM client ID (default: tuneloop)
#   IAM_CLIENT_SECRET       — IAM client secret (also reads IAM_SECRET alias)
#   IAM_NAMESPACE           — IAM namespace (default: tuneloop)
# <<< usage

DRY_RUN=true
FIXTURE=""
CONFIRM_ROWS=""
APPLY_REQUESTED=false

usage() {
  # Marker-based so the help text stays correct when the header grows.
  awk '/^# >>> usage$/{f=1;next} /^# <<< usage$/{f=0} f{sub(/^# ?/,"");print}' "$0"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --yes) DRY_RUN=false; APPLY_REQUESTED=true; shift ;;
    --fixture) FIXTURE="${2:-}"; shift 2 || { echo "ERROR: --fixture requires a path or -"; exit 2; } ;;
    --confirm-rows) CONFIRM_ROWS="${2:-}"; shift 2 || { echo "ERROR: --confirm-rows requires a number"; echo "RESULT: FAIL"; exit 2; } ;;
    -h|--help) usage; exit 0 ;;
    *) echo "ERROR: unknown argument: $1"; usage; echo "RESULT: FAIL"; exit 2 ;;
  esac
done

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ENV_FILE="$SCRIPT_DIR/.env"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# ---------------------------------------------------------------- arg gates
if [[ -n "$FIXTURE" && "$APPLY_REQUESTED" = true ]]; then
  # D3: fixture data must never be written to a real IAM.
  echo "ERROR: --fixture is dry-run only and cannot be combined with --yes"
  echo "RESULT: FAIL"
  exit 2
fi

if [[ "$APPLY_REQUESTED" = true ]]; then
  if [[ -z "$CONFIRM_ROWS" ]]; then
    echo "APPLY_CONFIRM_MISMATCH=1"
    echo "ERROR: --yes requires --confirm-rows <n> (the number printed as PLAN_UPDATES)"
    echo "RESULT: FAIL"
    exit 2
  fi
  if ! [[ "$CONFIRM_ROWS" =~ ^[0-9]+$ ]]; then
    echo "APPLY_CONFIRM_MISMATCH=1"
    echo "ERROR: --confirm-rows must be a non-negative integer"
    echo "RESULT: FAIL"
    exit 2
  fi
fi
if [[ -n "$FIXTURE" ]]; then
  # Fixture mode never touches the network.
  DRY_RUN=true
fi

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

if [[ -z "$FIXTURE" && -z "$CLIENT_SECRET" ]]; then
  echo "ERROR: IAM_CLIENT_SECRET (or IAM_SECRET) is required (set env var or add to .env)"
  echo "RESULT: FAIL"
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

if [[ -n "$FIXTURE" ]]; then
  echo "[1/4] Reading fixture (offline mode)..."
  if [[ "$FIXTURE" = "-" ]]; then
    ADMIN_USERS="$(cat)"
  else
    if [[ ! -f "$FIXTURE" ]]; then
      echo "ERROR: fixture not found: $FIXTURE"
      echo "RESULT: FAIL"
      exit 2
    fi
    ADMIN_USERS="$(cat "$FIXTURE")"
  fi
  NS_ORG_ID="fixture"
  echo "  Fixture rows: $(printf '%s\n' "$ADMIN_USERS" | grep -c . || true)"
else
  # Step 1: Get client token
  echo "[1/4] Getting IAM client token..."
  TOKEN_RESP=$(curl -sf -X POST "$IAM_URL/api/v1/auth/token" \
    -H 'Content-Type: application/json' \
    -d "{\"grant_type\":\"client_credentials\",\"client_id\":\"$CLIENT_ID\",\"client_secret\":\"$CLIENT_SECRET\"}" 2>&1) || {
    echo "ERROR: Failed to get client token from IAM"
    echo "  Response: $TOKEN_RESP"
    echo "RESULT: FAIL"
    exit 1
  }

  TOKEN=$(echo "$TOKEN_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('access_token',''))" 2>/dev/null)
  if [[ -z "$TOKEN" ]]; then
    echo "ERROR: No access_token in response"
    echo "  Response: $TOKEN_RESP"
    echo "RESULT: FAIL"
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
    echo "RESULT: FAIL"
    exit 1
  }

  NS_ORG_ID=$(echo "$NS_RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('id','') or d.get('data',{}).get('id',''))" 2>/dev/null)
  if [[ -z "$NS_ORG_ID" ]]; then
    echo "ERROR: Could not extract namespace org ID"
    echo "  Response: $NS_RESP"
    echo "RESULT: FAIL"
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
    echo "RESULT: FAIL"
    exit 1
  }

  # Parse users with role OWNER or namespace_admin.
  # #2490: `|` separator (email may legitimately be empty for phone-only admins;
  # a Tab-IFS reader would collapse the empty column and shift cus_perm left).
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
        print(f'{uid}|{email}|{cus_perm}')
" 2>/dev/null)

  if [[ -z "$ADMIN_USERS" ]]; then
    echo "  No namespace_admin users found. Nothing to migrate."
    echo "RESULT: PASS"
    exit 0
  fi

  echo "  Found namespace_admin users:"
  echo "  USER_ID                               EMAIL                      CUS_PERM"
  while IFS='|' read -r uid email cus_perm; do
    [[ -z "$uid" ]] && continue
    printf "  %-38s %-27s %s\n" "$uid" "$email" "$cus_perm"
  done <<< "$ADMIN_USERS"
fi

# ------------------------------------------------------- parse + validate
# Emits one normalized record per valid row; any malformed row aborts the run
# (fail-fast, exit 3) so nothing is written from a mis-parsed record.
# Scratch file lives under the project tmp/ (never a system temp dir).
mkdir -p "$PROJECT_ROOT/tmp"
PLAN_FILE="$(mktemp "$PROJECT_ROOT/tmp/migrate-plan.XXXXXX")"
trap 'rm -f "$PLAN_FILE"' EXIT

BAD_ROW=0
MIGRATE_INVALID=0
while IFS= read -r line; do
  [[ -z "$line" ]] && continue
  ROW_FIELDS=$(printf '%s' "$line" | awk -F'|' '{print NF}')
  if [[ "$ROW_FIELDS" != "3" ]]; then
    echo "BAD_ROW=1"
    echo "ROW_FIELDS=$ROW_FIELDS"
    echo "ERROR: expected 3 fields (uid|email|cus_perm), got $ROW_FIELDS: $line"
    BAD_ROW=1
    MIGRATE_INVALID=1
    continue
  fi
  IFS='|' read -r uid email cus_perm <<< "$line"
  if [[ -z "$uid" ]]; then
    echo "BAD_ROW=1"
    echo "ERROR: empty uid in row: $line"
    BAD_ROW=1
    MIGRATE_INVALID=1
    continue
  fi
  if ! [[ "${cus_perm:-}" =~ ^[0-9]+$ ]]; then
    echo "BAD_ROW=1"
    echo "BAD_PERM=${cus_perm:-}"
    echo "MIGRATE_INVALID=1"
    echo "ERROR: cus_perm must be a non-negative integer (got '${cus_perm:-}'): $line"
    BAD_ROW=1
    MIGRATE_INVALID=1
    continue
  fi
  printf '%s|%s|%s\n' "$uid" "$email" "$cus_perm" >> "$PLAN_FILE"
done <<< "$ADMIN_USERS"

if [[ "$BAD_ROW" = "1" ]]; then
  echo "BAD_ROW=1"
  echo "MIGRATE_INVALID=$MIGRATE_INVALID"
  echo "DB_WRITES=0"
  echo "ERROR: aborting migration because of malformed input (no changes were made)"
  echo "RESULT: FAIL"
  exit 3
fi
echo "MIGRATE_INVALID=0"

# Step 4: plan + apply
echo ""
echo "[4/4] Updating cus_perm..."

UPDATED=0
SKIPPED=0

while IFS='|' read -r uid email cus_perm; do
  [[ -z "$uid" ]] && continue
  CURRENT_PERM=$((cus_perm))
  NEW_PERM=$(( CURRENT_PERM | NEW_BITS_MASK ))

  if [[ $(( CURRENT_PERM & NEW_BITS_MASK )) -eq $NEW_BITS_MASK ]]; then
    echo "ACTION=SKIP uid=$uid email=$email old_perm=$CURRENT_PERM"
    SKIPPED=$((SKIPPED + 1))
    continue
  fi

  echo "ACTION=UPDATE uid=$uid email=$email old_perm=$CURRENT_PERM new_perm=$NEW_PERM"
  UPDATED=$((UPDATED + 1))
done < "$PLAN_FILE"

echo "PLAN_UPDATES=$UPDATED"
echo "SKIPPED=$SKIPPED"

# ------------------------------------------------------ write confirmation
# D4: --confirm-rows must equal the planned update count exactly (including the
# zero-update case: a stale count must never be treated as "close enough").
if [[ -n "$CONFIRM_ROWS" && "$CONFIRM_ROWS" != "$UPDATED" ]]; then
  echo "APPLY_CONFIRM_MISMATCH=1"
  echo "ERROR: --confirm-rows=$CONFIRM_ROWS does not match PLAN_UPDATES=$UPDATED"
  echo "DB_WRITES=0"
  echo "RESULT: FAIL"
  exit 2
fi
if [[ -n "$CONFIRM_ROWS" ]]; then
  echo "APPLY_READY=1"
fi

DB_WRITES=0
if [[ "$DRY_RUN" = true ]]; then
  echo "DB_WRITES=0"
  echo ""
  echo "============================================"
  echo "  Migration plan only (dry run)"
  echo "  To update: $UPDATED  Skipped: $SKIPPED"
  echo "  (DRY RUN — no changes were made)"
  if [[ -z "$FIXTURE" ]]; then
    echo "  Re-run with: ./migrate.sh --yes --confirm-rows $UPDATED"
  fi
  echo "============================================"
  echo "RESULT: PASS"
  exit 0
fi

DB_FAILURES=0
while IFS='|' read -r uid email cus_perm; do
  [[ -z "$uid" ]] && continue
  CURRENT_PERM=$((cus_perm))
  NEW_PERM=$(( CURRENT_PERM | NEW_BITS_MASK ))
  [[ $(( CURRENT_PERM & NEW_BITS_MASK )) -eq $NEW_BITS_MASK ]] && continue

  RESP=$(curl -sf -X PUT "$IAM_URL/api/v1/organizations/$NS_ORG_ID/users/$uid/customer-permissions" \
    -H "Authorization: Bearer $TOKEN" \
    -H 'Content-Type: application/json' \
    -d "{\"raw_bits\":true,\"cus_perm\":$NEW_PERM}" 2>&1) || {
    echo "    ERROR: Failed to update $email"
    echo "    Response: $RESP"
    DB_FAILURES=$((DB_FAILURES + 1))
    continue
  }
  echo "    OK — $email updated"
  DB_WRITES=$((DB_WRITES + 1))
done < "$PLAN_FILE"

echo "DB_WRITES=$DB_WRITES"
echo ""
echo "============================================"
echo "  Migration complete"
echo "  Updated: $DB_WRITES  Skipped: $SKIPPED"
if [[ "$DB_FAILURES" -gt 0 ]]; then
  echo "  Failures: $DB_FAILURES"
  echo "============================================"
  echo "RESULT: FAIL"
  exit 1
fi
echo "============================================"
echo "RESULT: PASS"
