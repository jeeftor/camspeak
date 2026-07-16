### Stage 1: build frontend
FROM oven/bun:1 AS frontend
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
RUN apk add --no-cache ffmpeg ca-certificates
WORKDIR /app
COPY --from=builder /app/camspeak .

ENV CAMSPEAK_DATA_DIR=/config

EXPOSE 8585
VOLUME ["/config"]

ENTRYPOINT ["/app/camspeak"]
CMD ["serve"]
