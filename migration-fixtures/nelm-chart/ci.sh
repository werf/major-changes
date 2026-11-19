#!/usr/bin/env bash
# CI script deploying a chart with Nelm v1. Uses $NELM and $ARTIFACTS from run.sh.
set -euo pipefail

NS=nelm-chart
export NELM_FEAT_CLEAN_NULL_FIELDS=true

kubectl create namespace "$NS" --dry-run=client -o yaml | kubectl apply -f -
kubectl -n "$NS" create configmap external-config --from-literal=ready=true --dry-run=client -o yaml | kubectl apply -f -

$NELM chart lint ./chart --secret-values override-secret-values.yaml
$NELM chart render ./chart --secret-values override-secret-values.yaml --set-runtime-json 'build={"id":"7"}' > "$ARTIFACTS/render.yaml"

set +e
$NELM release plan install -n "$NS" -r nelm-chart ./chart --secret-values override-secret-values.yaml --set greeting=from-set --set-runtime-json 'build={"id":"7"}' --show-verbose-diffs --exit-code
code=$?
set -e
case $code in
  0|2|3) echo "plan exit code: $code" ;;
  *) echo "plan failed with $code"; exit 1 ;;
esac

$NELM release install -n "$NS" -r nelm-chart ./chart --secret-values override-secret-values.yaml --set greeting=from-set --set-runtime-json 'build={"id":"7"}' --save-rollback-graph-to "$ARTIFACTS/rollback-graph.dot"

$NELM release list -n "$NS" -o json > "$ARTIFACTS/list.json"
jq -e '.[] | select(.name == "nelm-chart") | .status == "deployed"' "$ARTIFACTS/list.json"
$NELM release history nelm-chart -n "$NS" --max 5 -o json > "$ARTIFACTS/history.json"
jq -e 'length >= 1' "$ARTIFACTS/history.json"

greeting=$(kubectl -n "$NS" get deploy app -o jsonpath='{.spec.template.spec.containers[0].env[0].value}')
echo "greeting: $greeting"
