#!/usr/bin/env bash

set -euo pipefail

if [[ $# -lt 2 ]]; then
  echo "Usage: $0 <function-dir> <function-name>"
  echo "  function-dir:  path to the function directory (e.g. units/unit-numbers/function-numbers)"
  echo "  function-name: image name to use in the registry (e.g. function-numbers)"
  exit 1
fi

FUNCTION_DIR="$1"   # e.g. units/unit-numbers/function-numbers
FUNCTION_NAME="$2"  # e.g. function-numbers

BUILD_REGISTRY="${BUILD_REGISTRY:?BUILD_REGISTRY env var is required (e.g. ghcr.io/erost)}"
BUILD_VERSION="${BUILD_VERSION:?BUILD_VERSION env var is required}"

RUNTIME_IMAGE="${FUNCTION_NAME}-runtime:local"
RUNTIME_TARBALL="/tmp/${FUNCTION_NAME}-runtime.tar"
IMAGE="${BUILD_REGISTRY}/${FUNCTION_NAME}:${BUILD_VERSION}"
XPKG_FILE="${FUNCTION_NAME}.xpkg"
CHART_VERSION="${BUILD_VERSION}"
CHART_FILE="${FUNCTION_NAME}-${CHART_VERSION}.tgz"

echo "==> Building runtime image: ${RUNTIME_IMAGE}"
docker buildx build --load -t "${RUNTIME_IMAGE}" "${FUNCTION_DIR}"

echo "==> Saving runtime image to tarball"
docker save "${RUNTIME_IMAGE}" -o "${RUNTIME_TARBALL}"

echo "==> Building xpkg: ${IMAGE}"
crossplane xpkg build \
  --package-root="${FUNCTION_DIR}/package" \
  --ignore="function.yaml,xrd.yaml,composition.yaml" \
  --embed-runtime-image-tarball="${RUNTIME_TARBALL}" \
  --package-file="${XPKG_FILE}"

echo "==> Pushing xpkg: ${IMAGE}"
crossplane xpkg push \
  --package-files="${XPKG_FILE}" \
  "${IMAGE}"

echo "==> Building Helm chart: ${FUNCTION_NAME} ${CHART_VERSION}"
VALUES_FILE="${FUNCTION_DIR}/chart/values.yaml"
VALUES_BACKUP=$(mktemp /tmp/values-backup-XXXXXX.yaml)
cp "${VALUES_FILE}" "${VALUES_BACKUP}"
trap 'cp "${VALUES_BACKUP}" "${VALUES_FILE}"; rm -f "${VALUES_BACKUP}"' EXIT
yq -i ".registry = \"${BUILD_REGISTRY}\"" "${VALUES_FILE}"
yq -i ".tag = \"${BUILD_VERSION}\"" "${VALUES_FILE}"
helm dep update "${FUNCTION_DIR}/chart"
helm package "${FUNCTION_DIR}/chart" --version "${CHART_VERSION}" --destination .
cp "${VALUES_BACKUP}" "${VALUES_FILE}"
rm -f "${VALUES_BACKUP}"
trap - EXIT

echo "==> Pushing Helm chart: oci://${BUILD_REGISTRY}/charts/${FUNCTION_NAME}:${CHART_VERSION}"
helm push "${CHART_FILE}" "oci://${BUILD_REGISTRY}/charts"

rm -f "${XPKG_FILE}" "${RUNTIME_TARBALL}" "${CHART_FILE}"

echo "==> ${IMAGE} built and pushed (image + chart)"
