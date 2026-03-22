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

IMAGE="${BUILD_REGISTRY}/${FUNCTION_NAME}:${BUILD_VERSION}"
CHART_VERSION="${BUILD_VERSION}"
CHART_FILE="${FUNCTION_NAME}-${CHART_VERSION}.tgz"

PLATFORMS=("linux/amd64" "linux/arm64")
XPKG_FILES=()
RUNTIME_TARBALLS=()

for platform in "${PLATFORMS[@]}"; do
  arch="${platform#linux/}"
  runtime_image="${FUNCTION_NAME}-runtime-${arch}:local"
  runtime_tarball="/tmp/${FUNCTION_NAME}-runtime-${arch}.tar"
  xpkg_file="${FUNCTION_NAME}-${arch}.xpkg"

  echo "==> Building runtime image for ${platform}: ${runtime_image}"
  docker buildx build --platform "${platform}" --load -t "${runtime_image}" "${FUNCTION_DIR}"

  echo "==> Saving runtime image to tarball"
  docker save "${runtime_image}" -o "${runtime_tarball}"

  echo "==> Building xpkg for ${platform}: ${IMAGE}"
  crossplane xpkg build \
    --package-root="${FUNCTION_DIR}/package" \
    --ignore="function.yaml,xrd.yaml,composition.yaml" \
    --embed-runtime-image-tarball="${runtime_tarball}" \
    --package-file="${xpkg_file}"

  XPKG_FILES+=("${xpkg_file}")
  RUNTIME_TARBALLS+=("${runtime_tarball}")
done

XPKG_LIST=$(IFS=,; echo "${XPKG_FILES[*]}")

echo "==> Pushing xpkg (multi-arch): ${IMAGE}"
crossplane xpkg push \
  --package-files="${XPKG_LIST}" \
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

rm -f "${XPKG_FILES[@]}" "${RUNTIME_TARBALLS[@]}" "${CHART_FILE}"

echo "==> ${IMAGE} built and pushed (image + chart)"
