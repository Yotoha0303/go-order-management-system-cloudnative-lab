# syntax=docker/dockerfile:1.7
#
# Legacy single-binary image (./cmd). Prefer deploy/docker/Dockerfile.service
# for the microservice layout used by Compose / GHCR.

ARG GO_VERSION=1.25.12
ARG ALPINE_VERSION=3.22

FROM golang:${GO_VERSION}-alpine AS builder

WORKDIR /src

# Windows + Docker Desktop + slow proxy.golang.org 常卡死
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=off
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
      -trimpath \
      -ldflags="-s -w" \
      -o /out/app \
      ./cmd

FROM golang:${GO_VERSION}-alpine AS goose-builder

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go install github.com/pressly/goose/v3/cmd/goose@v3.27.1

FROM alpine:${ALPINE_VERSION}

RUN apk add --no-cache ca-certificates \
    && addgroup -S app \
    && adduser -S -G app -H -D app \
    && rm -rf /var/cache/apk/*

WORKDIR /app

COPY --from=builder /out/app ./go-order-management-system-cloudnative-lab
COPY --from=goose-builder /go/bin/goose ./goose
COPY --chown=app:app config.yml ./config.yml
COPY --chown=app:app migrations ./migrations

USER app

EXPOSE 8082

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8082/ping || exit 1

STOPSIGNAL SIGTERM

CMD ["./go-order-management-system-cloudnative-lab"]
