#!/usr/bin/env bash
# Configure AWS credentials for unit-aws.
#
# Creates (or updates) a namespace-scoped ProviderConfig named "aws" and its
# backing Secret on the unit-aws KinD cluster.  Nothing is written to disk;
# credentials flow only through shell variables piped directly into kubectl.

set -euo pipefail

CONTEXT="kind-unit-aws"

# ── Inputs ────────────────────────────────────────────────────────────────────

read -r -p "Target namespace on unit-aws [default]: " NAMESPACE
NAMESPACE="${NAMESPACE:-default}"

read -r -p "AWS Access Key ID: " AWS_ACCESS_KEY_ID

read -r -s -p "AWS Secret Access Key: " AWS_SECRET_ACCESS_KEY
echo  # newline after hidden input

read -r -p "AWS Default Region [eu-west-1]: " AWS_REGION
AWS_REGION="${AWS_REGION:-eu-west-1}"

# ── Namespace ─────────────────────────────────────────────────────────────────

echo "==> Ensuring namespace '${NAMESPACE}' exists on ${CONTEXT}"
kubectl --context="${CONTEXT}" create namespace "${NAMESPACE}" \
  --dry-run=client -o yaml \
  | kubectl --context="${CONTEXT}" apply -f -

# ── Secret ────────────────────────────────────────────────────────────────────

echo "==> Creating/updating Secret 'aws-credentials' in namespace '${NAMESPACE}'"

# Credentials are assembled in a variable and piped directly — never touch disk.
CREDENTIALS="[default]
aws_access_key_id = ${AWS_ACCESS_KEY_ID}
aws_secret_access_key = ${AWS_SECRET_ACCESS_KEY}
region = ${AWS_REGION}"

kubectl --context="${CONTEXT}" create secret generic aws-credentials \
  --namespace="${NAMESPACE}" \
  --from-literal=credentials="${CREDENTIALS}" \
  --dry-run=client -o yaml \
  | kubectl --context="${CONTEXT}" apply -f -

# Clear the credentials variable immediately after use.
unset CREDENTIALS AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY

# ── ProviderConfig ────────────────────────────────────────────────────────────

echo "==> Applying namespace-scoped ProviderConfig 'aws' in namespace '${NAMESPACE}'"

kubectl --context="${CONTEXT}" apply -f - <<EOF
apiVersion: aws.m.upbound.io/v1beta1
kind: ProviderConfig
metadata:
  name: aws
  namespace: ${NAMESPACE}
spec:
  credentials:
    source: Secret
    secretRef:
      namespace: ${NAMESPACE}
      name: aws-credentials
      key: credentials
EOF

echo "==> Done. AWS credentials configured for namespace '${NAMESPACE}' on ${CONTEXT}."
echo "    Buckets created in that namespace will use ProviderConfig 'aws' automatically."
