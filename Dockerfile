# Stage 1: Build the React 18 Workstation
FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --silent
COPY frontend/ ./
RUN npm run build

# Stage 2: Build the Go API Server
FROM golang:1.23-alpine AS backend-builder
RUN apk add --no-cache git ca-certificates
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/backend/bin/api ./cmd/api

# Stage 3: Minimal Production Runtime
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata curl

WORKDIR /app

# Copy compiled backend binary
COPY --from=backend-builder /app/backend/bin/api /app/api

# Copy seed datasets
COPY backend/seed/ /app/backend/seed/
COPY backend/seed/ /app/seed/

# Copy built frontend assets
COPY --from=frontend-builder /app/frontend/dist/ /app/frontend/dist/
COPY --from=frontend-builder /app/frontend/dist/ /app/dist/

# Default environment configuration
ENV HTTP_ADDR=":8080" \
    SEED_DIR="/app/backend/seed" \
    STATIC_DIR="/app/frontend/dist" \
    LLM_MODE="fake"

EXPOSE 8080

HEALTHCHECK --interval=5s --timeout=3s --retries=5 --start-period=5s \
  CMD curl -f http://localhost:8080/healthz || exit 1

ENTRYPOINT ["/app/api"]

