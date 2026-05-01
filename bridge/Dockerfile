# --- Stage 1: Build the Go binary ---
FROM golang:1.26.2-alpine AS builder

# Install git and CA certs (if needed for Go modules)
RUN apk upgrade --no-cache && apk add --no-cache git ca-certificates

# Set working directory inside the container
WORKDIR /app

# Copy Go source code into the container
COPY . .

# Enable Go Modules (optional if using go.mod)
ENV GO111MODULE=on

# Download deps
RUN go mod download

# Build the Go app for Linux with CGO disabled so the binary runs in Alpine.
RUN CGO_ENABLED=0 go build -o /app/dahuabridge ./cmd/dahuabridge

# --- Stage 2: Create a lightweight image with the binary only ---
FROM alpine:3.23

# Add CA certs for HTTPS support
RUN apk upgrade --no-cache && apk add --no-cache ca-certificates

# Copy binary from builder stage
COPY --from=builder /app/dahuabridge /app/dahuabridge
COPY --from=builder /app/docker-entrypoint.sh /app/docker-entrypoint.sh

# Set executable permissions (just in case)
RUN chmod +x /app/dahuabridge
RUN chmod +x /app/docker-entrypoint.sh


EXPOSE 9205
VOLUME ["/config"]

ENV DAHUABRIDGE_CONFIG=/config/config.yaml

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD wget -qO- http://127.0.0.1:9205/readyz >/dev/null || exit 1

ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["--config", "/config/config.yaml"]