#!/bin/bash
# Download pre-release zip from Seafile and deploy
# Usage: ./download.sh [local_zip_name]
set -euo pipefail

URL="https://seafile.safeuem.com/f/be6f4cffdeae42cbbd93/?dl=1"
ZIP="${1:-tuneloop-pre_latest.zip}"

cd /opt/flow
echo "Downloading $ZIP..."
wget -q --show-progress -O "$ZIP" "$URL" || { echo "ERROR: download failed"; exit 1; }
echo "Done."
"$(dirname "$0")/deploy.sh" "$ZIP"
