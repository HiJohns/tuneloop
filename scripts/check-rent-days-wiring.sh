#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

files=(
  frontend-mobile/src/pages/Checkout.jsx
  frontend-mobile/src/pages-weapp/Checkout.jsx
  frontend-mobile/src/pages/Renewal.jsx
  frontend-mobile/src/pages-weapp/renewal/Renewal.jsx
)

fail=0
for f in "${files[@]}"; do
  if ! grep -q "utils/rentDays.mjs" "$f"; then
    echo "FAIL: $f missing rentDays helper import"
    fail=1
  fi
  if ! grep -q "rentDaysInputState" "$f"; then
    echo "FAIL: $f missing rentDaysInputState usage"
    fail=1
  fi
  if ! grep -q "rentDaysCommitState" "$f"; then
    echo "FAIL: $f missing rentDaysCommitState usage"
    fail=1
  fi
  if ! grep -q "onBlur=" "$f"; then
    echo "FAIL: $f missing onBlur commit handler"
    fail=1
  fi
done

if grep -qE "setDays\(Math\.max\(1, Math\.min\(730" \
  frontend-mobile/src/pages/Checkout.jsx \
  frontend-mobile/src/pages-weapp/Checkout.jsx; then
  echo "FAIL: checkout still contains immediate clamps"
  fail=1
fi

for f in frontend-mobile/src/pages/Checkout.jsx frontend-mobile/src/pages/Renewal.jsx; do
  if grep -q "value={days}" "$f"; then
    echo "FAIL: $f still binds input text directly to numeric days"
    fail=1
  fi
done

if grep -q "parseInt(e.detail.value)" frontend-mobile/src/pages-weapp/renewal/Renewal.jsx; then
  echo "FAIL: weapp renewal still clamps raw input immediately"
  fail=1
fi

if [ "$fail" -ne 0 ]; then
  exit 1
fi

echo "WIRING: PASS"
