# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o keybench ./cmd/main.go

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates

# Create non-root user
RUN addgroup -g 1000 keybench && \
    adduser -D -u 1000 -G keybench keybench

# Copy binary from builder
COPY --from=builder /app/keybench /usr/local/bin/keybench

# Set ownership
RUN chown keybench:keybench /usr/local/bin/keybench

# Switch to non-root user
USER keybench

# Set entrypoint
ENTRYPOINT ["keybench"]

# Default command (show help)
CMD ["--help"]