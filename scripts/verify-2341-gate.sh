#!/usr/bin/env bash
# R7: build + services/database tests.
set -euo pipefail
source scripts/lib-2341-env.sh
require_pg
echo "PROVENANCE $PROVENANCE"
( cd backend && go build . && go test -v ./services/ ./database/ -count=1 -timeout 10m )
echo "R7 GATE PASS"
