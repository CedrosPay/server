# Cedros Pay Server - Production Dockerfile
# Multi-stage build for minimal final image size

# ============================================================================
# Stage 1: Builder - Build the Go binary
# ============================================================================
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary with optimizations
# - CGO_ENABLED=0: Static binary (no C dependencies)
# - -trimpath: Remove file system paths from binary
# - -ldflags: Strip debug info and set version
ARG VERSION=dev
ARG BUILD_TIME
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-w -s -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}" \
    -o server \
    ./cmd/server

# ============================================================================
# Stage 2: Runtime - Minimal runtime image
# ============================================================================
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    && addgroup -g 1000 cedros \
    && adduser -D -u 1000 -G cedros cedros

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy CA certificates for HTTPS calls
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/server /app/server

# Copy configuration templates (not secrets)
COPY --from=builder /build/configs/*.yaml ./configs/

# Copy migrations if they exist
COPY --from=builder /build/migrations ./migrations/

# Change ownership to non-root user
RUN chown -R cedros:cedros /app

# Switch to non-root user
USER cedros

# Expose port (default 8080)
EXPOSE 8080

# Health check (adjust path if needed)
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/cedros-health || exit 1

# Set default environment variables
ENV GO_ENV=production \
    PORT=8080 \
    LOG_LEVEL=info

# Run the binary
# Server will auto-detect configs/config.yaml if present, or fall back to configs/dev.yaml
# Override with: docker run ... /app/server -config /path/to/config.yaml
ENTRYPOINT ["/app/server"]
