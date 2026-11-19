#!/usr/bin/env bash
# Deploys the migration fixtures with werf v2 / Nelm v1 and re-runs them with werf v3 / Nelm v2.
set -euo pipefail

FIXTURES_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MIGRATION_DIR="${MIGRATION_DIR:-/home/user1/.bb/thread-storage/thr_x4cy6wre6t/migration}"
BIN_DIR="${BIN_DIR:-$MIGRATION_DIR/bin}"
WORK_DIR="${WORK_DIR:-$MIGRATION_DIR/work}"
KUBECONFIG_PATH="${KUBECONFIG_PATH:-$MIGRATION_DIR/kubeconfig}"

OLD_WERF="${OLD_WERF:-$BIN_DIR/werf-2.85.1}"
NEW_WERF="${NEW_WERF:-$BIN_DIR/werf-3.13.1}"
OLD_NELM="${OLD_NELM:-$BIN_DIR/nelm-1.31.3}"
NEW_NELM="${NEW_NELM:-$BIN_DIR/nelm-2-main}"

CLUSTER_NAME="werf-migration"
KIND_NODE_IMAGE="${KIND_NODE_IMAGE:-kindest/node:v1.36.1}"
REGISTRY_NAME="werf-migration-registry"
REGISTRY_PORT="${REGISTRY_PORT:-5055}"
REGISTRY="localhost:$REGISTRY_PORT"

SAMPLES=(werf-stapel werf-dockerfile werf-chart werf-bundle nelm-chart)

log() { printf '\n=== %s\n' "$*" >&2; }
die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

usage() {
  cat >&2 <<USAGE
Usage: $0 <command>
  cluster-up               create the kind cluster "$CLUSTER_NAME" and the registry "$REGISTRY_NAME" ($REGISTRY)
  baseline [SAMPLE...]     deploy/publish samples with the old binaries (default: all: ${SAMPLES[*]})
  upgrade SAMPLE DIR       run DIR (an edited copy of SAMPLE) with the new binaries against the baseline release/bundle
  cluster-down             delete the cluster and the registry
USAGE
  exit 1
}

is_werf_sample() { [[ -f "$1/werf.yaml" ]]; }

cluster_up() {
  if [[ "$(docker inspect -f '{{.State.Running}}' "$REGISTRY_NAME" 2>/dev/null || true)" != "true" ]]; then
    log "starting registry $REGISTRY_NAME on 127.0.0.1:$REGISTRY_PORT"
    docker rm -f "$REGISTRY_NAME" >/dev/null 2>&1 || true
    docker run -d --restart=always -p "127.0.0.1:$REGISTRY_PORT:5000" --network bridge --name "$REGISTRY_NAME" registry:3 >/dev/null
  fi

  if ! kind get clusters 2>/dev/null | grep -qx "$CLUSTER_NAME"; then
    log "creating kind cluster $CLUSTER_NAME"
    kind create cluster --name "$CLUSTER_NAME" --image "$KIND_NODE_IMAGE" --kubeconfig "$KUBECONFIG_PATH" --config=- <<KIND
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
  - |-
    [plugins."io.containerd.grpc.v1.cri".registry]
      config_path = "/etc/containerd/certs.d"
KIND
  fi

  local node registry_dir="/etc/containerd/certs.d/$REGISTRY"
  for node in $(kind get nodes --name "$CLUSTER_NAME"); do
    docker exec "$node" mkdir -p "$registry_dir"
    printf '[host."http://%s:5000"]\n' "$REGISTRY_NAME" | docker exec -i "$node" cp /dev/stdin "$registry_dir/hosts.toml"
  done

  if [[ "$(docker inspect -f '{{json .NetworkSettings.Networks.kind}}' "$REGISTRY_NAME")" == "null" ]]; then
    docker network connect kind "$REGISTRY_NAME"
  fi

  kubectl --kubeconfig "$KUBECONFIG_PATH" apply -f - >/dev/null <<CM
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "$REGISTRY"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
CM
  kubectl --kubeconfig "$KUBECONFIG_PATH" wait --for=condition=Ready nodes --all --timeout=180s >/dev/null
  log "cluster $CLUSTER_NAME is up (kubeconfig: $KUBECONFIG_PATH), registry: $REGISTRY"
}

cluster_down() {
  kind delete cluster --name "$CLUSTER_NAME" --kubeconfig "$KUBECONFIG_PATH" || true
  docker rm -f "$REGISTRY_NAME" >/dev/null 2>&1 || true
}

# The secret key of a sample is created once with the old binary and reused by baseline and upgrade.
secret_key() {
  local sample="$1" key_file="$WORK_DIR/keys/$1.key"
  if [[ ! -s "$key_file" ]]; then
    mkdir -p "$WORK_DIR/keys"
    chmod 700 "$WORK_DIR/keys"
    if [[ "$sample" == nelm-* ]]; then
      "$OLD_NELM" chart secret key create | tr -d '\n' > "$key_file"
    else
      "$OLD_WERF" helm secret generate-secret-key | tr -d '\n' > "$key_file"
    fi
    chmod 600 "$key_file"
  fi
  cat "$key_file"
}

# Copies SRC into DEST, encrypts every FILE.plain into FILE with the old binary (unless FILE already exists,
# e.g. re-encrypted while migrating), and commits the result into a fresh Git repository.
prepare() {
  local sample="$1" src="$2" dest="$3" key plain target
  key="$(secret_key "$sample")"

  rm -rf "$dest"
  mkdir -p "$dest"
  cp -a "$src/." "$dest/"
  rm -rf "$dest/.git"

  while IFS= read -r -d '' plain; do
    target="${plain%.plain}"
    if [[ ! -e "$target" ]]; then
      if [[ "$sample" == nelm-* ]]; then
        "$OLD_NELM" chart secret values-file encrypt "$plain" --secret-key "$key" --save-output-to "$target"
      else
        WERF_SECRET_KEY="$key" "$OLD_WERF" helm secret values encrypt "$plain" -o "$target" --dir "$dest" --loose-giterminism
      fi
    fi
    rm -f "$plain"
  done < <(find "$dest" -type f -name '*.plain' -print0)

  git -C "$dest" init -q -b main
  git -C "$dest" add -A
  git -C "$dest" -c user.name=werf-migration -c user.email=werf-migration@example.com commit -q -m "$sample"
}

# Runs ci.sh of the prepared sample with the given binaries.
run_ci() {
  local sample="$1" dir="$2" werf_bin="$3" nelm_bin="$4" phase="$5" key
  key="$(secret_key "$sample")"
  local artifacts="$WORK_DIR/artifacts/$phase/$sample"
  rm -rf "$artifacts"
  mkdir -p "$artifacts"

  log "$phase: $sample ($dir) with $(basename "$werf_bin") / $(basename "$nelm_bin")"
  (
    cd "$dir"
    export KUBECONFIG="$KUBECONFIG_PATH"
    export WERF="$werf_bin" NELM="$nelm_bin"
    export REGISTRY ARTIFACTS="$artifacts"
    export WERF_SECRET_KEY="$key" NELM_SECRET_KEY="$key"
    export WERF_HOME="$WORK_DIR/werf-home/$phase"
    export WERF_INSECURE_REGISTRY=1 WERF_SKIP_TLS_VERIFY_REGISTRY=1
    export WERF_LOG_COLOR_MODE="${WERF_LOG_COLOR_MODE:-off}" NELM_COLOR_MODE="${NELM_COLOR_MODE:-off}"
    export HOME_FOR_HELM="$WORK_DIR/helm-home"
    timeout "${CI_TIMEOUT:-600}" bash ./ci.sh
  ) 2>&1 | tee "$artifacts/ci.log"
}

baseline() {
  local samples=("$@") sample failed=()
  [[ ${#samples[@]} -gt 0 ]] || samples=("${SAMPLES[@]}")
  for sample in "${samples[@]}"; do
    [[ -d "$FIXTURES_DIR/$sample" ]] || die "unknown sample $sample"
    prepare "$sample" "$FIXTURES_DIR/$sample" "$WORK_DIR/baseline/$sample"
    if run_ci "$sample" "$WORK_DIR/baseline/$sample" "$OLD_WERF" "$OLD_NELM" baseline; then
      log "baseline $sample: OK"
    else
      log "baseline $sample: FAILED"
      failed+=("$sample")
    fi
  done
  [[ ${#failed[@]} -eq 0 ]] || die "baseline failed for: ${failed[*]}"
}

upgrade() {
  local sample="$1" src="$2"
  [[ -d "$src" ]] || die "no such directory: $src"
  [[ -d "$WORK_DIR/baseline/$sample" ]] || die "run baseline for $sample first"
  prepare "$sample" "$(cd "$src" && pwd)" "$WORK_DIR/upgrade/$sample"
  if run_ci "$sample" "$WORK_DIR/upgrade/$sample" "$NEW_WERF" "$NEW_NELM" upgrade; then
    log "upgrade $sample: OK"
  else
    log "upgrade $sample: FAILED"
    return 1
  fi
}

[[ $# -ge 1 ]] || usage
cmd="$1"; shift
case "$cmd" in
  cluster-up) cluster_up ;;
  cluster-down) cluster_down ;;
  baseline) baseline "$@" ;;
  upgrade) [[ $# -eq 2 ]] || usage; upgrade "$1" "$2" ;;
  *) usage ;;
esac
