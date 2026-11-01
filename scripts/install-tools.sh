#!/usr/bin/env bash
# Installs the external tools eks-upgrader shells out to: eksctl, helm, kubectl, and
# optionally the Pluto/kubent deprecated-API scanners. All EKS/CloudFormation
# mutations are performed exclusively through eksctl; this script never configures AWS
# credentials, which must come from the environment, IAM roles, or CI OIDC.
set -euo pipefail

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

BIN_DIR="${BIN_DIR:-/usr/local/bin}"
EKSCTL_VERSION="${EKSCTL_VERSION:-0.212.0}"
HELM_VERSION="${HELM_VERSION:-3.17.0}"
KUBECTL_VERSION="${KUBECTL_VERSION:-1.31.0}"

echo "Installing eksctl v${EKSCTL_VERSION}..."
curl -sSL "https://github.com/eksctl-io/eksctl/releases/download/v${EKSCTL_VERSION}/eksctl_$(uname -s)_${ARCH}.tar.gz" \
  | tar xz -C /tmp eksctl
sudo install -m 0755 /tmp/eksctl "${BIN_DIR}/eksctl"

echo "Installing helm v${HELM_VERSION}..."
curl -sSL "https://get.helm.sh/helm-v${HELM_VERSION}-${OS}-${ARCH}.tar.gz" | tar xz -C /tmp
sudo install -m 0755 "/tmp/${OS}-${ARCH}/helm" "${BIN_DIR}/helm"

echo "Installing kubectl v${KUBECTL_VERSION}..."
curl -sSL "https://dl.k8s.io/release/v${KUBECTL_VERSION}/bin/${OS}/${ARCH}/kubectl" -o /tmp/kubectl
sudo install -m 0755 /tmp/kubectl "${BIN_DIR}/kubectl"

echo "eksctl, helm, and kubectl installed to ${BIN_DIR}."
echo "Pluto and kubent are optional; install separately if deprecated-API scanning via those tools is desired."
