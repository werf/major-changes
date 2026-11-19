#!/usr/bin/env bash
# CI script of a werf v2 Dockerfile project. Uses $WERF, $REGISTRY and $ARTIFACTS from run.sh.
set -euo pipefail

export WERF_REPO="$REGISTRY/werf-dockerfile"

$WERF build --save-build-report --build-report-path "$ARTIFACTS/build-report.json"
jq -e '.Images.app.Stages | length > 0' "$ARTIFACTS/build-report.json"
$WERF run app | tee "$ARTIFACTS/app.txt"
$WERF run worker | tee "$ARTIFACTS/worker.txt"
