#!/usr/bin/env bash

set -euo pipefail

DEPLOYMENT_FILE="${1:-deployment.yaml}"

CROSSPLANE_VERSION=$(yq -r '.crossplaneVersion' "$DEPLOYMENT_FILE")

# ──────────────────────────────────────────
# Helpers
# ──────────────────────────────────────────

create_cluster() {
  local name="$1"
  if kind get clusters 2>/dev/null | grep -q "^${name}$"; then
    echo "==> Cluster ${name} already exists, skipping"
  else
    echo "==> Creating KinD cluster: ${name}"
    kind create cluster --name "${name}" --config ".scripts/kind/${name}.yaml"
  fi
}

install_crossplane() {
  local context="$1"
  echo "==> Installing Crossplane v${CROSSPLANE_VERSION} on ${context}"
  helm repo add crossplane-stable https://charts.crossplane.io/stable --force-update 2>/dev/null
  helm upgrade --install crossplane \
    --kube-context="${context}" \
    --namespace crossplane-system \
    --create-namespace \
    crossplane-stable/crossplane \
    --version "${CROSSPLANE_VERSION}" \
    --set metrics.enabled=true \
    --wait
}

apply_providers() {
  local context="$1"
  local selector="$2"
  local count
  count=$(yq "${selector} | length" "$DEPLOYMENT_FILE")
  for i in $(seq 0 $((count - 1))); do
    local name manifest
    name=$(yq -r "${selector}[$i].name" "$DEPLOYMENT_FILE")
    manifest=$(yq -r "${selector}[$i].manifestFile" "$DEPLOYMENT_FILE")
    echo "==> Applying provider ${name} to ${context}"
    kubectl --context="${context}" apply -n crossplane-system -f "${manifest}"
    echo "==> Waiting for provider ${name} to be healthy"
    kubectl --context="${context}" -n crossplane-system wait \
      --for=condition=Healthy "provider/${name}" \
      --timeout=120s
  done
}

# ──────────────────────────────────────────
# Commander
# ──────────────────────────────────────────

echo "==> Setting up commander"
create_cluster "commander"
install_crossplane "kind-commander"
apply_providers "kind-commander" ".commander.providers"

# ──────────────────────────────────────────
# Units
# ──────────────────────────────────────────

UNIT_COUNT=$(yq '.units | length' "$DEPLOYMENT_FILE")

for i in $(seq 0 $((UNIT_COUNT - 1))); do
  UNIT_NAME=$(yq -r ".units[$i].name" "$DEPLOYMENT_FILE")
  CONTEXT="kind-${UNIT_NAME}"
  echo "==> Setting up ${UNIT_NAME}"
  create_cluster "${UNIT_NAME}"
  install_crossplane "${CONTEXT}"
  apply_providers "${CONTEXT}" ".units[$i].providers"
done

# ──────────────────────────────────────────
# provider-kubernetes: kubeconfig secrets, ClusterProviderConfigs, CoreDNS
# ──────────────────────────────────────────

echo "==> Configuring provider-kubernetes on commander"

COMMANDER_CONTEXT="kind-commander"
UNIT_HOSTS=()

for i in $(seq 0 $((UNIT_COUNT - 1))); do
  UNIT_NAME=$(yq -r ".units[$i].name" "$DEPLOYMENT_FILE")
  SECRET_NAME="kind-${UNIT_NAME}-kubeconfig-plain"
  CONTROL_PLANE_HOST="${UNIT_NAME}-control-plane"
  CONTROL_PLANE_IP=$(docker inspect "${CONTROL_PLANE_HOST}" \
    --format '{{.NetworkSettings.Networks.kind.IPAddress}}')

  echo "==> Fetching kubeconfig for ${UNIT_NAME}"
  KUBECONFIG_CONTENT=$(kind get kubeconfig --name "${UNIT_NAME}" \
    | sed -E "s|server:.*|server: https://${CONTROL_PLANE_HOST}:6443|")

  echo "==> Recreating secret ${SECRET_NAME} on commander"
  kubectl --context="${COMMANDER_CONTEXT}" -n crossplane-system \
    delete secret "${SECRET_NAME}" --ignore-not-found=true
  kubectl --context="${COMMANDER_CONTEXT}" -n crossplane-system \
    create secret generic "${SECRET_NAME}" \
    --from-literal=kubeconfig="${KUBECONFIG_CONTENT}"

  echo "==> Applying ClusterProviderConfig for ${UNIT_NAME}"
  kubectl --context="${COMMANDER_CONTEXT}" apply -f - <<EOF
apiVersion: kubernetes.m.crossplane.io/v1alpha1
kind: ClusterProviderConfig
metadata:
  name: ${UNIT_NAME}
spec:
  credentials:
    source: Secret
    secretRef:
      namespace: crossplane-system
      name: ${SECRET_NAME}
      key: kubeconfig
EOF

  UNIT_HOSTS+=("${CONTROL_PLANE_IP} ${CONTROL_PLANE_HOST}")
done

echo "==> Patching CoreDNS on commander with unit host entries"

COREFILE_TMP=$(mktemp /tmp/coredns-corefile-XXXXXX)
HOSTS_BLOCK_TMP=$(mktemp /tmp/coredns-hosts-XXXXXX)
MODIFIED_TMP=$(mktemp /tmp/coredns-modified-XXXXXX)
trap 'rm -f "$COREFILE_TMP" "$HOSTS_BLOCK_TMP" "$MODIFIED_TMP"' EXIT

kubectl --context="${COMMANDER_CONTEXT}" -n kube-system \
  get configmap coredns -o jsonpath='{.data.Corefile}' > "$COREFILE_TMP"

INDENT=$(grep -m1 '[[:space:]]*forward ' "$COREFILE_TMP" | sed 's/forward.*//')

{
  printf '%s# BEGIN unit-hosts\n' "$INDENT"
  printf '%shosts {\n' "$INDENT"
  for ENTRY in "${UNIT_HOSTS[@]}"; do
    printf '%s    %s\n' "$INDENT" "$ENTRY"
  done
  printf '%s    fallthrough\n' "$INDENT"
  printf '%s}\n' "$INDENT"
  printf '%s# END unit-hosts\n' "$INDENT"
} > "$HOSTS_BLOCK_TMP"

awk -v hosts_block="$HOSTS_BLOCK_TMP" '
  /# BEGIN unit-hosts/ { skip=1; next }
  /# END unit-hosts/   { skip=0; next }
  skip                 { next }
  /[[:space:]]+forward / && !inserted {
    while ((getline line < hosts_block) > 0) print line
    inserted=1
  }
  { print }
' "$COREFILE_TMP" > "$MODIFIED_TMP"

kubectl --context="${COMMANDER_CONTEXT}" create configmap coredns \
  --from-file=Corefile="$MODIFIED_TMP" \
  -n kube-system \
  --dry-run=client -o yaml \
  | kubectl --context="${COMMANDER_CONTEXT}" replace -f -

echo "==> Restarting CoreDNS"
kubectl --context="${COMMANDER_CONTEXT}" -n kube-system rollout restart deployment coredns
kubectl --context="${COMMANDER_CONTEXT}" -n kube-system rollout status deployment coredns --timeout=60s

echo "==> Setup complete"
