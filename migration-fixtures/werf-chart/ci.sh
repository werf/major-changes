#!/usr/bin/env bash
# CI script of a werf v2 project. Uses $WERF, $REGISTRY and $ARTIFACTS from run.sh.
set -euo pipefail

export WERF_REPO="$REGISTRY/werf-chart"
export WERF_ENV=test
export HELM_DRIVER=secret
NS=werf-chart-test

kubectl create namespace "$NS" --dry-run=client -o yaml | kubectl apply -f -
kubectl -n "$NS" create configmap external-config --from-literal=ready=true --dry-run=client -o yaml | kubectl apply -f -

$WERF helm template werf-chart .helm --set werf.image.app=example --set global.env=test --set api.token=example > "$ARTIFACTS/helm-template.yaml"
$WERF render --secret-values extra-secret-values.yaml --set-runtime-json 'ci={"job":"42"}' > "$ARTIFACTS/render.yaml"

set +e
$WERF plan --secret-values extra-secret-values.yaml --set-runtime-json 'ci={"job":"42"}' --show-verbose-diffs --exit-code > "$ARTIFACTS/plan.log"
code=$?
set -e
case $code in
  0|2) echo "plan exit code: $code" ;;
  *) echo "plan failed with $code"; cat "$ARTIFACTS/plan.log"; exit 1 ;;
esac

$WERF converge --virtual-merge=false --secret-values extra-secret-values.yaml --set-runtime-json 'ci={"job":"42"}' --set backend.config.greeting=from-ci

$WERF helm list -n "$NS" > "$ARTIFACTS/helm-list.txt"
$WERF helm get values werf-chart-test -n "$NS" > "$ARTIFACTS/helm-get-values.yaml" || true
kubectl -n "$NS" get configmap backend-config -o jsonpath='{.data.greeting}' > "$ARTIFACTS/backend-greeting.txt"
echo "backend greeting: $(cat "$ARTIFACTS/backend-greeting.txt")"
