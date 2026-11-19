#!/usr/bin/env bash
# CI script of a werf v2 Stapel project. Uses $WERF, $REGISTRY and $ARTIFACTS from run.sh.
set -euo pipefail

export WERF_REPO="$REGISTRY/werf-stapel"

$WERF config list > "$ARTIFACTS/config-list.txt"
$WERF build --save-build-report --build-report-path "$ARTIFACTS/build-report.json"
jq -e '.Images.app.DockerImageName' "$ARTIFACTS/build-report.json"
$WERF run app -- sh -c 'cat /app/setup.txt /app/config/app.conf /app/meta/marker /app/bin/tools/marker.txt && /app/bin/app.sh && echo "APP_ENV=$APP_ENV" && pwd' | tee "$ARTIFACTS/run.txt"
