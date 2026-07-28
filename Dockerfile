# syntax=docker/dockerfile:1

# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && go mod verify

# Copy source code
COPY . .

# Build the application with optimizations. BuildKit cache mounts keep the
# module and compiler caches warm across builds; -trimpath drops local paths
# for reproducible binaries.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-w -s" -o server ./cmd/server

# Runtime stage
FROM alpine:3.21

# Install runtime dependencies:
#   ca-certificates — TLS for OAuth/API calls
#   tzdata          — IANA timezone database (event scheduling relies on it)
RUN apk add --no-cache ca-certificates tzdata

# Create the non-root user up front so the COPY --chown below needs no extra
# chown -R layer (which would duplicate the whole app tree).
RUN adduser -D -H -u 10001 appuser

WORKDIR /app

# Copy binary and runtime assets, already owned by the runtime user.
COPY --from=builder --chown=appuser:appuser /app/server ./server
COPY --from=builder --chown=appuser:appuser /app/templates ./templates
COPY --from=builder --chown=appuser:appuser /app/static ./static
COPY --from=builder --chown=appuser:appuser /app/migrations ./migrations

USER appuser

# Port the server listens on. Keep in sync with SERVER_ADDRESS; the healthcheck
# and EXPOSE both derive from it so there is a single source of truth.
ARG APP_PORT=8080
ENV APP_PORT=${APP_PORT}
EXPOSE ${APP_PORT}

# Health check (start-period covers migrations on first boot).
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider "http://localhost:${APP_PORT}/health" || exit 1

# Run the application
CMD ["./server"]
