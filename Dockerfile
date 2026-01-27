# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install git for go mod download
RUN apk add --no-cache git

# Copy source files
COPY . .

# Download dependencies and build
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o llm-bridge ./cmd/llm-bridge

# Runtime stage
FROM alpine:latest

# Install Claude CLI dependencies
RUN apk add --no-cache \
    ca-certificates \
    nodejs \
    npm \
    bash \
    git

# Install Claude CLI (adjust version as needed)
# Note: Replace with actual Claude CLI installation method
RUN npm install -g @anthropic-ai/claude-code || true

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/llm-bridge /usr/local/bin/llm-bridge

# Create config directory
RUN mkdir -p /etc/llm-bridge

# Default config path
ENV LLM_BRIDGE_CONFIG=/etc/llm-bridge/llm-bridge.yaml

ENTRYPOINT ["llm-bridge"]
CMD ["serve", "--config", "/etc/llm-bridge/llm-bridge.yaml"]
