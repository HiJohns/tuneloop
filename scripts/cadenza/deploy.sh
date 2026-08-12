#!/bin/bash
set -euo pipefail

# Deploy a release zip (pre-prod or production)
# Usage: ./deploy.sh <path/to/pkg.zip>
#   OR set SEAFILE_DEPLOY=<download-url>

APPS_BASE="${TUNELOOP_APPS_BASE:-/opt/tuneloop/apps}"
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

# Auto-detect environment:
# - tuneloop-pre_*.zip carries its environment in the package name
# - beaconiam_*.zip is dual-environment; TUNELOOP_APPS_BASE decides the apps
#   dir, and the systemd unit MUST follow it: /opt/tuneloop-pre/apps means the
#   unit is beaconiam-pre (fix: never stop/start the production unit)
if echo "$SERVICE" | grep -q "pre$"; then
    APPS_BASE="/opt/tuneloop-pre/apps"
else
    APPS_BASE="${TUNELOOP_APPS_BASE:-$APPS_BASE}"
fi
case "$APPS_BASE" in
    *"-pre"*)
        if [[ "$SERVICE" != *-pre ]]; then
            SYSTEMD_UNIT="${SERVICE}-pre"
        else
            SYSTEMD_UNIT="$SERVICE"
        fi
        ;;
    *)
        SYSTEMD_UNIT="$SERVICE"
        ;;
esac
TARGET_DIR="$APPS_BASE/$SERVICE"

if [ ! -d "$TARGET_DIR" ]; then
    echo "ERROR: target directory $TARGET_DIR does not exist"
    exit 1
fi

EXTRACT_DIR="$FLOW_BASE/$TITLE"

echo "=========================================="
echo "Deploying: $SERVICE ($PKG_NAME)"
echo "  apps dir : $TARGET_DIR"
echo "  systemd  : $SYSTEMD_UNIT"
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

# Verify the service binary was updated (binary name differs per service:
# tuneloop for tuneloop/tuneloop-pre, beaconiam for beaconiam/beaconiam-pre)
if [ -L "$TARGET_DIR/service" ]; then
    BIN_TARGET=$(find -L "$TARGET_DIR/service" -maxdepth 1 -type f -perm -u+x ! -name "*.sh" | head -1)
    echo "  Binary: ${BIN_TARGET:-NOT FOUND}"
    if [ -z "$BIN_TARGET" ] || [ ! -f "$BIN_TARGET" ]; then
        echo "ERROR: binary not found under $TARGET_DIR/service"
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