#!/usr/bin/env bash
#
# Push Docker Images for nself v0.3.9
#

set -e

VERSION="0.3.9"
REGISTRY="ghcr.io"
NAMESPACE="nself-org"
IMAGE="nself"

echo "Building and pushing Docker images for nself v${VERSION}..."

# Build multi-arch image
echo "Building Docker image..."
cd /Users/admin/Sites/nself/releases/v${VERSION}

# Build for multiple architectures
docker buildx create --name nself-builder --use 2>/dev/null || docker buildx use nself-builder

docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --tag "${REGISTRY}/${NAMESPACE}/${IMAGE}:${VERSION}" \
  --tag "${REGISTRY}/${NAMESPACE}/${IMAGE}:latest" \
  --tag "${REGISTRY}/${NAMESPACE}/${IMAGE}:0.3" \
  --push \
  .

echo "✓ Docker images pushed to ${REGISTRY}"
echo ""
echo "Available tags:"
echo "  • ${REGISTRY}/${NAMESPACE}/${IMAGE}:${VERSION}"
echo "  • ${REGISTRY}/${NAMESPACE}/${IMAGE}:latest"
echo "  • ${REGISTRY}/${NAMESPACE}/${IMAGE}:0.3"
echo ""
echo "Users can now run:"
echo "  docker pull ${REGISTRY}/${NAMESPACE}/${IMAGE}:${VERSION}"
echo "  docker run --rm -it ${REGISTRY}/${NAMESPACE}/${IMAGE}:${VERSION} help"