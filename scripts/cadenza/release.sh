#!/bin/bash
# Promote a pre-prod release to production
# Usage: /opt/flow/release.sh tuneloop-pre_20260717-184337_55758503.zip
#   → renames tuneloop-pre → tuneloop in extracted dir
#   → calls deploy.sh with renamed content

set -euo pipefail

if [ $# -lt 1 ]; then
    echo "Usage: $0 <tuneloop-pre_YYYYMMDD-HHMMSS_COMMITID.zip>"
    exit 1
fi

PRE_ZIP="$1"
PRE_NAME=$(basename "$PRE_ZIP")

if [ ! -f "/opt/flow/$PRE_NAME" ]; then
    echo "ERROR: /opt/flow/$PRE_NAME not found"
    exit 1
fi

TIMESTAMP=$(date +%Y%m%d-%H%M%S)
COMMIT=$(echo "$PRE_NAME" | sed "s/tuneloop-pre_[0-9]*-[0-9]*_//" | sed s/.zip//)
PROD_NAME="tuneloop_${TIMESTAMP}_${COMMIT}.zip"
PROD_TITLE="${PROD_NAME%.zip}"
EXTRACT_DIR="/opt/flow/$PROD_TITLE"

echo "=========================================="
echo "Promoting to PRODUCTION (v$(date +%Y.%m.%d.%H%M))"
echo "  Source:   $PRE_NAME"
echo "  Target:   $PROD_NAME"
echo "=========================================="

# Extract and rename tuneloop-pre → tuneloop for production
rm -rf "$EXTRACT_DIR"
mkdir -p "$EXTRACT_DIR"
unzip -o "/opt/flow/$PRE_NAME" -d "$EXTRACT_DIR"

if [ -d "$EXTRACT_DIR/tuneloop-pre" ]; then
    mv "$EXTRACT_DIR/tuneloop-pre" "$EXTRACT_DIR/tuneloop"
    echo "  Renamed: tuneloop-pre → tuneloop"
fi

# Re-pack as production zip
cd "$EXTRACT_DIR"
zip -r "/opt/flow/$PROD_NAME" .
cd /opt/flow

echo ""
echo "Deploying to production..."
/opt/flow/deploy.sh "/opt/flow/$PROD_NAME"

# Clean up extracted directory
rm -rf "$EXTRACT_DIR"

echo ""
echo "=========================================="
echo "PRODUCTION DEPLOYED: $PROD_NAME"
echo "Verify: https://web.cadenzayueqi.com"
echo "=========================================="
