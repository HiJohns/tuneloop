#!/bin/bash
# Deploy a release zip (pre-prod or production)
# Usage: ./deploy.sh <path/to/pkg.zip>
#   OR set SEAFILE_DEPLOY=<download-url>
set -euo pipefail

FLOW_BASE="/opt/flow"
UPLOADS_DIR="/opt/uploads"

if [ -n "${1:-}" ]; then
    ZIP_FILE="$1"
elif [ -n "${SEAFILE_DEPLOY:-}" ]; then
    echo "=== Downloading from Seafile ==="
    WRAPPER_ZIP="${FLOW_BASE}/test.zip"
    wget -O "$WRAPPER_ZIP" "$SEAFILE_DEPLOY"
    TMP_DIR=$(mktemp -d -p "$FLOW_BASE" unwrap.XXXXXX)
    unzip -o "$WRAPPER_ZIP" -d "$TMP_DIR"
    ZIP_FILE=$(find "$TMP_DIR" -maxdepth 1 -name "*.zip" ! -name "test.zip" | head -1)
    if [ -z "$ZIP_FILE" ]; then
        echo "ERROR: No deploy package inside test.zip"
        rm -rf "$TMP_DIR"
        exit 1
    fi
    echo "  Unwrapped: $ZIP_FILE"
else
    echo "Usage: $0 <path/to/pkg.zip>"
    echo "  OR set SEAFILE_DEPLOY=<download-url>"
    exit 1
fi

[ ! -f "$ZIP_FILE" ] && { echo "ERROR: $ZIP_FILE not found"; exit 1; }

PKG_NAME=$(basename "$ZIP_FILE")
TITLE="${PKG_NAME%.zip}"
SERVICE=$(echo "$TITLE" | cut -d_ -f1)

# Auto-detect environment: pre-prod vs production
if echo "$SERVICE" | grep -q "pre$"; then
    APPS_BASE="/opt/tuneloop-pre/apps"
    SERVICE="tuneloop-pre"
    SYSTEMD_UNIT="tuneloop-pre"
else
    APPS_BASE="${TUNELOOP_APPS_BASE:-/opt/tuneloop/apps}"
    SYSTEMD_UNIT="$SERVICE"
fi

TARGET_DIR="$APPS_BASE/$SERVICE"

if [ ! -d "$TARGET_DIR" ]; then
    echo "ERROR: target directory $TARGET_DIR does not exist"
    exit 1
fi

EXTRACT_DIR="$FLOW_BASE/$TITLE"

echo "=========================================="
echo "Deploying: $SERVICE ($PKG_NAME)"
echo "=========================================="

rm -rf "$EXTRACT_DIR"
mkdir -p "$EXTRACT_DIR"
unzip -o "$ZIP_FILE" -d "$EXTRACT_DIR"

CONTENT_DIR="$EXTRACT_DIR/$SERVICE"
if [ ! -d "$CONTENT_DIR" ]; then
    CONTENT_DIR=$(find "$EXTRACT_DIR" -maxdepth 1 -type d ! -name . | head -1)
    echo "  Auto-detected content dir: $(basename "$CONTENT_DIR")"
fi
echo "  Content: $CONTENT_DIR"

echo "Stopping $SYSTEMD_UNIT..."
sudo systemctl stop "$SYSTEMD_UNIT" 2>/dev/null || true
sleep 1

echo "Updating symlinks..."
for comp in www mobile service database; do
    if [ -d "$CONTENT_DIR/$comp" ]; then
        rm -f "$TARGET_DIR/$comp"
        ln -sf "$CONTENT_DIR/$comp" "$TARGET_DIR/$comp"
        echo "  $comp -> $(readlink "$TARGET_DIR/$comp")"
    fi
done

case "$SERVICE" in
    tuneloop)
        # Production: uploads -> /opt/uploads
        [ -L "$TARGET_DIR/uploads" ] || ln -sf "$UPLOADS_DIR" "$TARGET_DIR/uploads"
        ;;
    tuneloop-pre)
        # Prerelease: dedicated uploads dir, never link to production
        mkdir -p /opt/tuneloop-pre/uploads/media
        chown -R deploy:deploy /opt/tuneloop-pre/uploads
        [ -L "$TARGET_DIR/uploads" ] && rm -f "$TARGET_DIR/uploads"
        ln -sf /opt/tuneloop-pre/uploads "$TARGET_DIR/uploads"
        ;;
    *)
        echo "  WARN: unknown service $SERVICE, skipping uploads link"
        ;;
esac

echo "Starting $SYSTEMD_UNIT..."
sudo systemctl start "$SYSTEMD_UNIT"
sleep 2

# Verify the service binary was updated
if [ -L "$TARGET_DIR/service" ]; then
    BIN_TARGET=$(readlink -f "$TARGET_DIR/service/tuneloop")
    echo "  Binary: $BIN_TARGET"
    if [ ! -f "$BIN_TARGET" ]; then
        echo "ERROR: binary not found at $BIN_TARGET"
        exit 1
    fi
fi

if docker ps --format "{{.Names}}" | grep -q "tuneloop-nginx"; then
    echo "Restarting tuneloop-nginx..."
    docker restart tuneloop-nginx
fi

echo ""
echo "=========================================="
echo "Deploy complete: $SERVICE"
echo "=========================================="
