# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.26
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build

WORKDIR /src

ARG TARGETOS
ARG TARGETARCH

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY cmd ./cmd
COPY internal ./internal

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build \
      -trimpath \
      -ldflags="-s -w \
        -X RCooLeR/DahuaBridge/internal/buildinfo.Version=${VERSION} \
        -X RCooLeR/DahuaBridge/internal/buildinfo.Commit=${COMMIT} \
        -X RCooLeR/DahuaBridge/internal/buildinfo.BuildDate=${BUILD_DATE}" \
      -o /out/dahuabridge \
      ./cmd/dahuabridge

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata wget ffmpeg \
    && addgroup -S dahuabridge \
    && adduser -S -D -H -h /app -s /sbin/nologin -G dahuabridge dahuabridge \
    && mkdir -p /app /config /data \
    && chown -R dahuabridge:dahuabridge /app /config /data

WORKDIR /app

COPY --from=build /out/dahuabridge /app/dahuabridge

USER dahuabridge

EXPOSE 8080
VOLUME ["/config", "/data"]

ENV DAHUABRIDGE_CONFIG=/config/config.yaml

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/readyz >/dev/null || exit 1

ENTRYPOINT ["/app/dahuabridge"]
CMD ["--config", "/config/config.yaml"]
