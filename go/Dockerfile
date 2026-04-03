## ── Stage 1: build ──────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Download dependencies first (better layer cache)
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
RUN VERSION=$(cat VERSION) && \
    CGO_ENABLED=0 GOOS=linux go build \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o drohnenwetter ./cmd/drohnenwetter

## ── Stage 2: minimal runtime ────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder /app/drohnenwetter  /app/drohnenwetter
COPY --from=builder /app/templates   /app/templates
COPY --from=builder /app/static      /app/static

EXPOSE 8080

ENV TEMPLATE_DIR=/app/templates
ENV STATIC_DIR=/app/static

ENTRYPOINT ["/app/drohnenwetter"]
