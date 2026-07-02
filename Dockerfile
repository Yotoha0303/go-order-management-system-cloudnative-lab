# syntax=docker/dockerfile:1

FROM golang:1.25.7-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o go-order-management-system ./cmd

FROM golang:1.25.7-alpine AS goose-builder

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go install github.com/pressly/goose/v3/cmd/goose@v3.27.1

FROM alpine:3.22

WORKDIR /app

RUN addgroup -S app && adduser -S app -G app

COPY --from=builder /app/go-order-management-system ./go-order-management-system
COPY --from=goose-builder /go/bin/goose ./goose
COPY config.yml ./config.yml
COPY migrations ./migrations

USER app

EXPOSE 8082

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
	CMD wget -qO- http://127.0.0.1:8082/ping || exit 1

STOPSIGNAL SIGTERM

CMD ["./go-order-management-system"]
