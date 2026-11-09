# Stage 1: Build the Go API Server
FROM golang:alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/bin/api ./cmd/api

# Stage 2: Minimal Production Runtime (Zero External Package Dependencies)
FROM alpine:3.20

WORKDIR /app

# Copy root CA certificates from builder (zero apk downloads needed)
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# Copy compiled backend binary
COPY --from=builder /app/bin/api /app/api

# Copy seed datasets
COPY seed/ /app/seed/
COPY seed/ /app/backend/seed/

# Default environment configuration
ENV HTTP_ADDR=":8080" \
    SEED_DIR="/app/seed" \
    LLM_MODE="fake"

EXPOSE 8080

HEALTHCHECK --interval=5s --timeout=3s --retries=5 --start-period=5s \
  CMD wget -qO- http://localhost:8080/healthz || exit 1

ENTRYPOINT ["/app/api"]

