# syntax=docker/dockerfile:1

FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w -X eks-upgrader/internal/cli.Version=${VERSION:-dev} -X eks-upgrader/internal/cli.Commit=${COMMIT:-none}" \
    -o /out/eks-upgrader ./cmd/eks-upgrader

# Runtime image needs eksctl + helm + kubectl on PATH since this application shells
# out to them; it never calls the AWS EKS/CloudFormation mutation APIs directly.
FROM debian:bookworm-slim AS runtime
ARG EKSCTL_VERSION=0.212.0
ARG HELM_VERSION=3.17.0
ARG KUBECTL_VERSION=1.31.0

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates curl tar git \
    && rm -rf /var/lib/apt/lists/*

RUN ARCH=$(dpkg --print-architecture) && \
    case "$ARCH" in amd64) EKSCTL_ARCH=amd64; HELM_ARCH=amd64; KCTL_ARCH=amd64 ;; \
                    arm64) EKSCTL_ARCH=arm64; HELM_ARCH=arm64; KCTL_ARCH=arm64 ;; \
                    *) echo "unsupported architecture: $ARCH" && exit 1 ;; esac && \
    curl -sSL "https://github.com/eksctl-io/eksctl/releases/download/v${EKSCTL_VERSION}/eksctl_Linux_${EKSCTL_ARCH}.tar.gz" \
        | tar xz -C /usr/local/bin eksctl && \
    curl -sSL "https://get.helm.sh/helm-v${HELM_VERSION}-linux-${HELM_ARCH}.tar.gz" \
        | tar xz -C /tmp && mv /tmp/linux-${HELM_ARCH}/helm /usr/local/bin/helm && \
    curl -sSL "https://dl.k8s.io/release/v${KUBECTL_VERSION}/bin/linux/${KCTL_ARCH}/kubectl" \
        -o /usr/local/bin/kubectl && chmod +x /usr/local/bin/kubectl

RUN useradd --uid 10001 --create-home --shell /usr/sbin/nologin eks-upgrader
COPY --from=build /out/eks-upgrader /usr/local/bin/eks-upgrader

USER eks-upgrader
WORKDIR /home/eks-upgrader
ENTRYPOINT ["/usr/local/bin/eks-upgrader"]
CMD ["--help"]
