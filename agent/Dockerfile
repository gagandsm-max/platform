# Platform Linux Agent runtime / CI verification image.
FROM golang:1.27.1-alpine3.24 AS verify
WORKDIR /build
RUN apk add --no-cache gcc musl-dev
COPY go.mod ./
COPY common ./common
COPY linux ./linux
COPY windows ./windows
RUN go vet ./...
RUN CGO_ENABLED=1 go test -race ./...
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/platform-agent-linux ./linux/cmd
# Compile-only boundary check. Windows Service/SCM certification is explicitly separate.
RUN CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/platform-agent-windows.exe ./windows/cmd

FROM alpine:3.24
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S platform \
    && adduser -S -D -H -G platform platform \
    && install -d -m 0700 -o platform -g platform /var/lib/platform/agent
COPY --from=verify /out/platform-agent-linux /usr/local/bin/platform-agent-linux
USER platform
ENTRYPOINT ["/usr/local/bin/platform-agent-linux"]
