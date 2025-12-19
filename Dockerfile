# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /dynamo ./cmd/dynamo

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 dynamo && \
    adduser -u 1000 -G dynamo -s /bin/sh -D dynamo

# Create data directory
RUN mkdir -p /data && chown dynamo:dynamo /data

# Copy binary from builder
COPY --from=builder /dynamo /usr/local/bin/dynamo

# Switch to non-root user
USER dynamo

# Set default environment variables
ENV DYNAMO_ADDRESS=0.0.0.0
ENV DYNAMO_PORT=8080
ENV DYNAMO_GOSSIP_PORT=7946
ENV DYNAMO_DATA_DIR=/data

# Expose ports
EXPOSE 8080 7946/udp

# Health check
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

# Default command
ENTRYPOINT ["/usr/local/bin/dynamo"]
CMD ["--address=0.0.0.0", "--port=8080", "--data-dir=/data"]
