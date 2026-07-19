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

### Stage 3: build shairport-sync from source with --with-stdout
# The Alpine package (4.3.2) omits stdout support; we build it ourselves.
FROM alpine:3.20 AS shairport-builder
RUN apk add --no-cache \
      git autoconf automake libtool \
      pkgconfig gcc make \
      popt-dev libconfig-dev openssl-dev \
      avahi-dev soxr-dev alsa-lib-dev
RUN git clone --depth 1 --branch 4.3.2 \
      https://github.com/mikebrady/shairport-sync.git /src
WORKDIR /src
RUN autoreconf -fi && \
    ./configure \
      --with-avahi \
      --with-ssl=openssl \
      --with-stdout \
      --with-pipe \
      --with-soxr \
      --with-metadata \
      --sysconfdir=/etc && \
    make -j"$(nproc)" && \
    make install DESTDIR=/build

### Stage 4: runtime
FROM alpine:3.20
# ffmpeg: PCM→G.711ulaw transcoding
# avahi: mDNS advertisement for shairport-sync (run with --no-dbus, no dbus needed)
# runtime libs for our custom shairport-sync build
RUN apk add --no-cache \
      ffmpeg \
      ca-certificates \
      avahi \
      popt \
      libconfig \
      openssl \
      soxr
COPY --from=shairport-builder /build/usr/local/bin/shairport-sync /usr/local/bin/shairport-sync
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
