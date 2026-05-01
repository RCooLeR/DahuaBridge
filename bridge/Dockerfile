# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.26.2

FROM golang:${GO_VERSION}-alpine AS build

WORKDIR /src

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    set -eux; \
    if [ -f /src/go.mod ]; then \
        MODULE_ROOT=/src; \
    elif [ -f /src/bridge/go.mod ]; then \
        MODULE_ROOT=/src/bridge; \
    else \
        echo "unable to locate go.mod in /src or /src/bridge" >&2; \
        exit 1; \
    fi; \
    cd "${MODULE_ROOT}"; \
    go mod download

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    set -eux; \
    if [ -f /src/go.mod ]; then \
        MODULE_ROOT=/src; \
    elif [ -f /src/bridge/go.mod ]; then \
        MODULE_ROOT=/src/bridge; \
    else \
        echo "unable to locate go.mod in /src or /src/bridge" >&2; \
        exit 1; \
    fi; \
    cd "${MODULE_ROOT}"; \
    MODULE="$(go list -m)"; \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
      -trimpath \
      -ldflags="-s -w \
        -X ${MODULE}/internal/buildinfo.Version=${VERSION} \
        -X ${MODULE}/internal/buildinfo.Commit=${COMMIT} \
        -X ${MODULE}/internal/buildinfo.BuildDate=${BUILD_DATE}" \
      -o /out/dahuabridge \
      ./cmd/dahuabridge; \
    if [ -f "${MODULE_ROOT}/docker-entrypoint.sh" ]; then \
        ENTRYPOINT_SRC="${MODULE_ROOT}/docker-entrypoint.sh"; \
    elif [ -f /src/docker-entrypoint.sh ]; then \
        ENTRYPOINT_SRC=/src/docker-entrypoint.sh; \
    else \
        echo "unable to locate docker-entrypoint.sh" >&2; \
        exit 1; \
    fi; \
    install -Dm755 "${ENTRYPOINT_SRC}" /out/docker-entrypoint.sh

FROM --platform=linux/amd64 debian:bookworm-slim

RUN set -eux; \
    sed -i 's/Components: main/Components: main contrib non-free non-free-firmware/g' /etc/apt/sources.list.d/debian.sources; \
    apt-get update; \
    apt-get install -y --no-install-recommends \
        ca-certificates \
        tzdata \
        wget \
        ffmpeg \
        gosu \
        vainfo \
        libvpl2 \
        intel-media-va-driver-non-free; \
    if apt-cache show libmfx-gen1.2 >/dev/null 2>&1; then \
        apt-get install -y --no-install-recommends libmfx-gen1.2; \
    elif apt-cache show libmfx1 >/dev/null 2>&1; then \
        apt-get install -y --no-install-recommends libmfx1; \
    fi; \
    rm -rf /var/lib/apt/lists/*; \
    groupadd --system dahuabridge; \
    useradd --system --no-create-home --home-dir /app --shell /usr/sbin/nologin --gid dahuabridge dahuabridge; \
    mkdir -p /app /config /data; \
    chown -R dahuabridge:dahuabridge /app /config /data

WORKDIR /app

COPY --from=build /out/dahuabridge /app/dahuabridge
COPY --from=build /out/docker-entrypoint.sh /app/docker-entrypoint.sh

EXPOSE 9205
VOLUME ["/config"]

ENV DAHUABRIDGE_CONFIG=/config/config.yaml

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD wget -qO- http://127.0.0.1:9205/readyz >/dev/null || exit 1

ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["--config", "/config/config.yaml"]