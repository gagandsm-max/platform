# ==============================================================================
# Platform Sensor Dockerfile (Clean-room Multi-stage Build)
# ==============================================================================

FROM golang:1.27.1-alpine3.24 AS builder
WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /build/bin/platform-sensor ./cmd/sensor

# ==============================================================================
# Runtime Stage
# ==============================================================================
FROM alpine:3.20
WORKDIR /app

RUN apk add --no-cache curl tzdata \
    && addgroup -S platform \
    && adduser -S platform -G platform \
    && mkdir -p /var/lib/platform/sensor/spool \
    && chown -R platform:platform /var/lib/platform/sensor /app

COPY --from=builder /build/bin/platform-sensor /usr/local/bin/platform-sensor

USER platform
EXPOSE 8088

HEALTHCHECK --interval=10s --timeout=5s --start-period=5s --retries=3 \
    CMD curl -f http://127.0.0.1:8088/health || exit 1

ENTRYPOINT ["/usr/local/bin/platform-sensor"]
