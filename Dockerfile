# Build stage
FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS builder

# Build arguments for cross-compilation (needed early for pkl download)
ARG TARGETOS
ARG TARGETARCH

# Install git and pkl
RUN apk add --no-cache git curl && \
    # Download pkl binary based on target architecture
    if [ "$TARGETARCH" = "amd64" ]; then \
        PKL_ARCH="amd64"; \
    elif [ "$TARGETARCH" = "arm64" ]; then \
        PKL_ARCH="aarch64"; \
    else \
        echo "Unsupported architecture: $TARGETARCH" && exit 1; \
    fi && \
    curl -L "https://github.com/apple/pkl/releases/download/0.28.2/pkl-linux-${PKL_ARCH}" -o /usr/local/bin/pkl && \
    chmod +x /usr/local/bin/pkl

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build arguments for cross-compilation
ARG TARGETOS
ARG TARGETARCH

# Build the application (pkl should be available in PATH)
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-w -s" -o codegen ./cmd/codegen/main.go

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates git gcompat

# Copy the binary from builder stage
COPY --from=builder /app/codegen /usr/local/bin/codegen

# Copy pkl binary from builder stage
COPY --from=builder /usr/local/bin/pkl /usr/local/bin/pkl

# Run the binary
ENTRYPOINT ["codegen"]
