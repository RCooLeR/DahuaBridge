# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.26.2
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build

WORKDIR /src

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download


RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    set -eux; \
    MODULE="$(go list -m)"; \
    CGO_ENABLED=0 GOOS="${TARGETOS:-linux}" GOARCH="${TARGETARCH:-amd64}" \
    go build \
      -trimpath \
      -ldflags="-s -w \
        -X ${MODULE}/internal/buildinfo.Version=${VERSION} \
        -X ${MODULE}/internal/buildinfo.Commit=${COMMIT} \
        -X ${MODULE}/internal/buildinfo.BuildDate=${BUILD_DATE}" \
      -o /out/dahuabridge \
      ./cmd/dahuabridge

FROM --platform=$TARGETPLATFORM debian:bookworm-slim

ARG TARGETARCH

RUN set -eux; \
    sed -i 's/Components: main/Components: main contrib non-free non-free-firmware/g' /etc/apt/sources.list.d/debian.sources; \
    apt-get update; \
    apt-get install -y --no-install-recommends \
        ca-certificates \
        tzdata \
        wget \
        ffmpeg \
        gosu; \
    if [ "${TARGETARCH:-amd64}" = "amd64" ]; then \
        apt-get install -y --no-install-recommends \
            vainfo \
            intel-media-va-driver-non-free \
            libvpl2 \
            libmfx-gen1.2; \
    fi; \
    rm -rf /var/lib/apt/lists/*; \
    groupadd --system dahuabridge; \
    useradd --system --no-create-home --home-dir /app --shell /usr/sbin/nologin --gid dahuabridge dahuabridge; \
    mkdir -p /app /config /data; \
    chown -R dahuabridge:dahuabridge /app /config /data

WORKDIR /app

COPY --from=build /out/dahuabridge /app/dahuabridge
COPY --chmod=0755 docker-entrypoint.sh /app/docker-entrypoint.sh

EXPOSE 9205
VOLUME ["/config"]

ENV DAHUABRIDGE_CONFIG=/config/config.yaml

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD wget -qO- http://127.0.0.1:9205/readyz >/dev/null || exit 1

ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["--config", "/config/config.yaml"]