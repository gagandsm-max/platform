# ==============================================================================
# Platform Agent Dockerfile (Linux Daemon / Test Harness)
# ==============================================================================

FROM golang:1.23-alpine AS builder
WORKDIR /build

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /build/bin/platform-agent-linux ./linux/cmd

# Verify Windows cross-compilation during build
RUN CGO_ENABLED=0 GOOS=windows go build -ldflags="-s -w" -o /build/bin/platform-agent-windows.exe ./windows/cmd

# ==============================================================================
# Runtime Stage (Linux Daemon)
# ==============================================================================
FROM alpine:3.20
WORKDIR /app

RUN apk add --no-cache curl tzdata \
    && addgroup -S platform \
    && adduser -S platform -G platform \
    && mkdir -p /var/lib/platform/agent \
    && chown -R platform:platform /var/lib/platform/agent /app

COPY --from=builder /build/bin/platform-agent-linux /usr/local/bin/platform-agent-linux

USER platform

ENTRYPOINT ["/usr/local/bin/platform-agent-linux"]
