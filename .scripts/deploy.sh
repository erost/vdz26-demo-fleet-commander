#!/usr/bin/env bash

set -euo pipefail

DEPLOYMENT_FILE="${1:-deployment.yaml}"

# ──────────────────────────────────────────
# Helpers
# ──────────────────────────────────────────

deploy_compositions() {
  local context="$1"
  local selector="$2"
  local count
  count=$(yq "${selector} | length" "$DEPLOYMENT_FILE")
  for i in $(seq 0 $((count - 1))); do
    local name enabled chart version
    name=$(yq -r "${selector}[$i].name" "$DEPLOYMENT_FILE")
    enabled=$(yq -r "${selector}[$i].enabled" "$DEPLOYMENT_FILE")
    if [ "${enabled}" = "false" ]; then
      echo "==> Skipping disabled composition ${name}"
      continue
    fi
    chart=$(yq -r "${selector}[$i].chart" "$DEPLOYMENT_FILE")
    version=$(yq -r "${selector}[$i].version" "$DEPLOYMENT_FILE")

    local values_args=()
    if yq -e "${selector}[$i].values" "$DEPLOYMENT_FILE" &>/dev/null; then
      while IFS= read -r kv; do
        values_args+=("--set" "$kv")
      done < <(yq -r "${selector}[$i].values | to_entries[] | .key + \"=\" + .value" "$DEPLOYMENT_FILE")
    fi

    echo "==> Deploying ${name} to ${context}"
    helm upgrade --install "${name}" "${chart}" \
      --kube-context="${context}" \
      --namespace crossplane-system \
      --create-namespace \
      --version "${version}" \
      "${values_args[@]+"${values_args[@]}"}" \
      --wait
  done
}

# ──────────────────────────────────────────
# Commander compositions
# ──────────────────────────────────────────

echo "==> Deploying compositions to commander"
deploy_compositions "kind-commander" ".commander.compositions"

# ──────────────────────────────────────────
# Unit compositions
# ──────────────────────────────────────────

UNIT_COUNT=$(yq '.units | length' "$DEPLOYMENT_FILE")
for i in $(seq 0 $((UNIT_COUNT - 1))); do
  UNIT_NAME=$(yq -r ".units[$i].name" "$DEPLOYMENT_FILE")
  UNIT_ENABLED=$(yq -r ".units[$i].enabled" "$DEPLOYMENT_FILE")
  if [ "${UNIT_ENABLED}" = "false" ]; then
    echo "==> Skipping disabled unit ${UNIT_NAME}"
    continue
  fi
  echo "==> Deploying compositions to ${UNIT_NAME}"
  deploy_compositions "kind-${UNIT_NAME}" ".units[$i].compositions"
done

echo "==> All compositions deployed"
