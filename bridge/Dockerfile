# syntax=docker/dockerfile:1.7

# --- Stage 1: Build the Go binary ---
FROM golang:1.26.2-alpine AS build

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    set -eux; \
    MODULE="$(go list -m)"; \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
      -trimpath \
      -ldflags="-s -w \
        -X ${MODULE}/internal/buildinfo.Version=${VERSION} \
        -X ${MODULE}/internal/buildinfo.Commit=${COMMIT} \
        -X ${MODULE}/internal/buildinfo.BuildDate=${BUILD_DATE}" \
      -o /app/dahuabridge \
      ./cmd/dahuabridge; \
    chmod +x /app/dahuabridge /app/docker-entrypoint.sh


# --- Stage 2: Runtime image ---
FROM ubuntu:26.04

ENV DEBIAN_FRONTEND=noninteractive
ENV LIBVA_DRIVER_NAME=iHD

RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends \
      ca-certificates \
      wget \
      ffmpeg \
      tzdata \
      gosu \
      passwd \
      vainfo \
      libva2 \
      libva-drm2 \
      libvpl2 \
      libvpl-tools \
      libmfx-gen1.2 \
      va-driver-all \
      intel-media-va-driver-non-free; \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=build /app/dahuabridge /app/dahuabridge
COPY --from=build /app/docker-entrypoint.sh /app/docker-entrypoint.sh

RUN set -eux; \
    useradd --system --create-home --home-dir /var/lib/dahuabridge --shell /usr/sbin/nologin dahuabridge; \
    chmod 0755 /app/dahuabridge /app/docker-entrypoint.sh; \
    mkdir -p /config /data /data/tmp; \
    chown -R dahuabridge:dahuabridge /data /data/tmp /var/lib/dahuabridge

EXPOSE 9205

VOLUME ["/config", "/data"]

ENV DAHUABRIDGE_CONFIG=/config/config.yaml

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD wget -qO- http://127.0.0.1:9205/readyz >/dev/null || exit 1

ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["--config", "/config/config.yaml"]