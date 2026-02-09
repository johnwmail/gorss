# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache gcc musl-dev

# Copy go mod files first for caching
COPY go.mod go.sum ./
ENV GOTOOLCHAIN=auto
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=1 GOOS=linux go build -o gorss -ldflags='-s -w' ./cmd/srv

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata


# Copy binary from builder
COPY --from=builder /app/gorss .

# Copy templates and static files
COPY --from=builder /app/srv/templates ./srv/templates
COPY --from=builder /app/srv/static ./srv/static

# Create user with specific UID/GID 8080
RUN deluser gorss 2>/dev/null || true && \
    addgroup -g 8080 gorss && \
    adduser -u 8080 -G gorss -h /app -D gorss

# Create data directory for SQLite and set permissions
RUN mkdir -p /data && chown -R 8080:8080 /data /app

# Switch to non-root user
USER 8080:8080

# Environment variables
ENV GORSS_DB_PATH=/data/gorss.db
ENV GORSS_PORT=8080
ENV GORSS_PURGE_DAYS=30
ENV GORSS_AUTH_MODE=none
# ENV GORSS_PASSWORD=your-secret-password

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run
CMD ["./gorss"]
