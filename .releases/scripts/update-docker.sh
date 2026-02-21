#!/bin/bash

# update-docker.sh - Build and push Docker images for nself
# Usage: ./scripts/update-docker.sh <version>

set -e

VERSION="${1:-}"
DOCKER_REPO="nself-org/cli"
PLATFORMS="linux/amd64,linux/arm64"

if [[ -z "$VERSION" ]]; then
    echo "Usage: $0 <version>"
    exit 1
fi

echo "🐳 Building Docker images for nself ${VERSION}"

# Check if docker is available
if ! command -v docker &> /dev/null; then
    echo "❌ Docker not found. Please install Docker."
    exit 1
fi

# Check if we're in the repository root
if [[ ! -f "Dockerfile" ]]; then
    echo "📝 Creating Dockerfile..."
    cat > Dockerfile << 'EOF'
# Multi-stage build for smaller image
FROM alpine:3.19 AS builder

# Install build dependencies
RUN apk add --no-cache \
    bash \
    curl \
    git \
    make

# Copy source
WORKDIR /build
COPY . .

# No compilation needed for bash scripts
# Just organize files
RUN mkdir -p /app && \
    cp -r bin src LICENSE README.md /app/ && \
    chmod +x /app/bin/nself

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache \
    bash \
    curl \
    git \
    docker-cli \
    docker-cli-compose \
    ca-certificates \
    tzdata

# Copy app from builder
COPY --from=builder /app /opt/nself

# Create symlink
RUN ln -s /opt/nself/bin/nself /usr/local/bin/nself

# Create workspace directory
RUN mkdir -p /workspace
WORKDIR /workspace

# Add version label
LABEL org.opencontainers.image.title="nself" \
      org.opencontainers.image.description="Self-hosted infrastructure manager" \
      org.opencontainers.image.url="https://nself.org" \
      org.opencontainers.image.source="https://github.com/nself-org/cli" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.vendor="nself" \
      org.opencontainers.image.licenses="MIT"

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD nself version || exit 1

# Default command
ENTRYPOINT ["nself"]
CMD ["help"]
EOF
fi

# Login to Docker Hub
echo "🔐 Logging in to Docker Hub..."
echo "Please enter your Docker Hub credentials:"
docker login

# Setup buildx for multi-platform builds
echo "🔧 Setting up Docker Buildx..."
docker buildx create --name nself-builder --use 2>/dev/null || docker buildx use nself-builder

# Build and push multi-platform image
echo "🏗️  Building multi-platform images..."
docker buildx build \
    --platform "${PLATFORMS}" \
    --tag "${DOCKER_REPO}:${VERSION}" \
    --tag "${DOCKER_REPO}:latest" \
    --push \
    .

echo "✅ Docker images pushed successfully!"
echo ""
echo "Images available:"
echo "  ${DOCKER_REPO}:${VERSION}"
echo "  ${DOCKER_REPO}:latest"
echo ""
echo "Users can now run:"
echo "  docker run -v /var/run/docker.sock:/var/run/docker.sock ${DOCKER_REPO}:latest"