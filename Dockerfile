### Stage 1: build frontend
FROM oven/bun:1.3.14 AS frontend
WORKDIR /app/frontend
ENV NODE_OPTIONS=--no-deprecation
COPY frontend/package.json frontend/bun.lock ./
RUN bun install --frozen-lockfile
COPY frontend/ ./
RUN bun run build

### Stage 2: build Go binary
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
ARG VERSION=dev
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=frontend /app/frontend/dist ./frontend/dist
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -ldflags="-s -w -X main.Version=${VERSION}" -o camspeak .

### Stage 3: runtime
FROM alpine:3.20
# ffmpeg: PCM→G.711ulaw transcoding
# shairport-sync: AirPlay/RAOP receiver (handles FairPlay+ALAC correctly)
# avahi + dbus: mDNS advertisement for shairport-sync (requires --net=host in Docker)
RUN apk add --no-cache \
      ffmpeg \
      ca-certificates \
      dbus \
      avahi \
      shairport-sync
WORKDIR /app
COPY --from=builder /app/camspeak .
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

ENV CAMSPEAK_DATA_DIR=/config

EXPOSE 8585
VOLUME ["/config"]

# NOTE: AirPlay advertisement requires host networking so avahi multicast
# reaches the LAN: docker run --net=host ...
# This container runs as root so it can write to the /config volume.

ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["/app/camspeak", "serve"]
