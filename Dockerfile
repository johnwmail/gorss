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

# Create non-root user
RUN adduser -D -u 1000 gorss

# Copy binary from builder
COPY --from=builder /app/gorss .

# Copy templates and static files
COPY --from=builder /app/srv/templates ./srv/templates
COPY --from=builder /app/srv/static ./srv/static

# Create data directory for SQLite and set permissions
RUN mkdir -p /data && chown -R gorss:gorss /data /app

# Switch to non-root user
USER gorss

# Environment variables
ENV GORSS_DB_PATH=/data/gorss.db
ENV GORSS_LISTEN=:8000

# Expose port
EXPOSE 8000

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8000/health || exit 1

# Run
CMD ["./gorss"]
