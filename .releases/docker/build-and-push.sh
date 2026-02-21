#!/bin/bash
set -e

VERSION="${1:-0.3.7}"
DOCKER_USER="nself"
IMAGE_NAME="nself"

echo "Building Docker image for nself v${VERSION}..."

# Build the image
docker build -t ${DOCKER_USER}/${IMAGE_NAME}:${VERSION} -t ${DOCKER_USER}/${IMAGE_NAME}:latest -f .releases/docker/Dockerfile .

echo ""
echo "To push to Docker Hub:"
echo "  1. Login: docker login -u ${DOCKER_USER}"
echo "  2. Push: docker push ${DOCKER_USER}/${IMAGE_NAME}:${VERSION}"
echo "  3. Push latest: docker push ${DOCKER_USER}/${IMAGE_NAME}:latest"
echo ""
echo "Users can then run:"
echo "  docker pull ${DOCKER_USER}/${IMAGE_NAME}:latest"
echo "  docker run -it ${DOCKER_USER}/${IMAGE_NAME}:latest"