#!/bin/bash
set -e

VERSION="0.9.8"
IMAGE_NAME="nself-org/cli"

echo "Building Docker image for nself v${VERSION}"

# Create buildx builder if it doesn't exist
docker buildx create --use --name nself-builder 2>/dev/null || docker buildx use nself-builder

# Build multi-arch image
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t "${IMAGE_NAME}:${VERSION}" \
  -t "${IMAGE_NAME}:latest" \
  --push \
  -f .releases/packaging/docker/Dockerfile \
  .

echo "✅ Docker image ${IMAGE_NAME}:${VERSION} built and pushed successfully!"
echo "✅ Docker image ${IMAGE_NAME}:latest updated!"

# Test the image
echo "Testing Docker image..."
docker run --rm "${IMAGE_NAME}:${VERSION}" version

echo "🚀 Docker release complete!"