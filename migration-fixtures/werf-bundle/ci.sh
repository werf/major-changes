#!/usr/bin/env bash
# CI script publishing and deploying a werf v2 bundle. Uses $WERF, $REGISTRY and $ARTIFACTS from run.sh.
set -euo pipefail

export WERF_REPO="$REGISTRY/werf-bundle"
BUNDLE_REPO="$REGISTRY/werf-bundle-chart"
NS=werf-bundle

$WERF bundle publish --repo "$BUNDLE_REPO" --tag 1.0 \
  --set app.greeting=from-publish \
  --values publish-values.yaml \
  --secret-values publish-secret-values.yaml

$WERF bundle apply --repo "$BUNDLE_REPO" --tag 1.0 --release werf-bundle --namespace "$NS"

$WERF bundle copy --from "$BUNDLE_REPO:1.0" --to "archive:$ARTIFACTS/bundle.tar.gz"
ls -l "$ARTIFACTS/bundle.tar.gz"

kubectl -n "$NS" get configmap werf-bundle-config -o jsonpath='{.data}' | tee "$ARTIFACTS/config.json"
echo
jq -e '.greeting == "from-publish" and .replicas == "2" and ."token-set" == "true"' "$ARTIFACTS/config.json"
