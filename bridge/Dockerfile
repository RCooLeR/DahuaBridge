# syntax=docker/dockerfile:1.29.0@sha256:572cebbc2b15e6f84c077742952674889cb63cd5e9b80abdffcbe587b6f19183
# check=error=true

# --- Stage 1: Build the Go binary ---
FROM golang:1.27.4-alpine3.24@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS build

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
# Keep dependencies in the layer: remote layer caches do not restore cache mounts,
# and the compilation step must also work offline on a fresh builder.
RUN go mod download && go mod verify

COPY --link cmd ./cmd
COPY --link internal ./internal

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
ARG TARGETOS=linux
ARG TARGETARCH=amd64

RUN --network=none \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    set -eux; \
    MODULE="$(go list -m)"; \
    CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" \
    go build \
      -mod=readonly \
      -buildvcs=false \
      -tags=urfave_cli_no_template \
      -trimpath \
      -ldflags="-s -w \
        -X ${MODULE}/internal/buildinfo.Version=${VERSION} \
        -X ${MODULE}/internal/buildinfo.Commit=${COMMIT} \
        -X ${MODULE}/internal/buildinfo.BuildDate=${BUILD_DATE}" \
      -o /out/dahuabridge \
      ./cmd/dahuabridge


# --- Stage 2: Runtime image ---
FROM ubuntu:26.04@sha256:d66b366dac61b575bee2aef07e9c26e8070096fbe84a3d4d3eb0da069b5d76db

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

LABEL org.opencontainers.image.title="DahuaBridge" \
      org.opencontainers.image.description="Dahua camera bridge for Home Assistant" \
      org.opencontainers.image.source="https://github.com/RCooLeR/DahuaBridge" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}"

ENV DAHUABRIDGE_CONFIG=/config/config.yaml \
    HOME=/var/lib/dahuabridge \
    LIBVA_DRIVER_NAME=iHD

RUN set -eux; \
    apt-get update; \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      ca-certificates \
      ffmpeg \
      libva-drm2 \
      libva2 \
      libmfx-gen1.2 \
      libvpl-tools \
      libvpl2 \
      intel-media-va-driver-non-free \
      passwd \
      tzdata \
      util-linux \
      va-driver-all \
      vainfo \
      wget; \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --link --from=build --chmod=0755 /out/dahuabridge /app/dahuabridge
COPY --link --chmod=0644 docker-entrypoint.sh /app/docker-entrypoint.sh

RUN set -eux; \
    sed -i 's/\r$//' /app/docker-entrypoint.sh; \
    chmod 0755 /app/docker-entrypoint.sh; \
    groupadd --system dahuabridge; \
    useradd --system --gid dahuabridge --create-home --home-dir /var/lib/dahuabridge --shell /usr/sbin/nologin dahuabridge; \
    mkdir -p /config /data /data/tmp; \
    chown -R dahuabridge:dahuabridge /data /data/tmp /var/lib/dahuabridge

EXPOSE 9020 8554

VOLUME ["/config", "/data"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --start-interval=2s --retries=3 \
  CMD ["wget", "--quiet", "--output-document=/dev/null", "--timeout=4", "--tries=1", "http://127.0.0.1:9020/readyz"]

ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["--config", "/config/config.yaml"]
