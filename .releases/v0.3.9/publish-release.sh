#!/usr/bin/env bash
#
# nself v0.3.9 Release Publishing Script
# Uploads release assets to GitHub and updates package repositories
#

set -e

VERSION="0.3.9"
REPO="nself-org/cli"
RELEASE_DIR="$(dirname "$0")"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[0;33m'
RED='\033[0;31m'
RESET='\033[0m'

echo -e "${BLUE}Publishing nself v${VERSION} to all platforms...${RESET}"

# 1. Create GitHub Release
echo -e "${BLUE}Creating GitHub release...${RESET}"
gh release create "v${VERSION}" \
  --title "nself v${VERSION}" \
  --notes-file "${RELEASE_DIR}/RELEASE_NOTES.md" \
  --verify-tag

# 2. Upload Release Assets
echo -e "${BLUE}Uploading release assets...${RESET}"

# Create tarball
echo "Creating source tarball..."
cd /Users/admin/Sites/nself
git archive --format=tar.gz --prefix="nself-${VERSION}/" "v${VERSION}" > "${RELEASE_DIR}/nself-v${VERSION}.tar.gz"

# Calculate SHA256
SHA256=$(shasum -a 256 "${RELEASE_DIR}/nself-v${VERSION}.tar.gz" | cut -d' ' -f1)
echo "SHA256: ${SHA256}"

# Upload all assets
gh release upload "v${VERSION}" \
  "${RELEASE_DIR}/nself-v${VERSION}.tar.gz" \
  "${RELEASE_DIR}/install.sh" \
  "${RELEASE_DIR}/install-debian.sh" \
  "${RELEASE_DIR}/install-rhel.sh" \
  "${RELEASE_DIR}/nself.rb" \
  "${RELEASE_DIR}/Dockerfile" \
  "${RELEASE_DIR}/docker-compose.release.yml" \
  "${RELEASE_DIR}/RELEASE_MANIFEST.md"

echo -e "${GREEN}✓ GitHub release created and assets uploaded${RESET}"

# 3. Update Homebrew Tap
echo -e "${BLUE}Updating Homebrew tap...${RESET}"
if [[ -d ~/Sites/homebrew-nself ]]; then
  cd ~/Sites/homebrew-nself
  
  # Update formula with actual SHA256
  cp "${RELEASE_DIR}/nself.rb" Formula/nself.rb
  sed -i '' "s/sha256 \".*\"/sha256 \"${SHA256}\"/" Formula/nself.rb
  
  # Commit and push
  git add Formula/nself.rb
  git commit -m "Update nself to v${VERSION}"
  git push origin main
  
  echo -e "${GREEN}✓ Homebrew tap updated${RESET}"
else
  echo -e "${YELLOW}⚠ Homebrew tap repository not found at ~/Sites/homebrew-nself${RESET}"
  echo "To create the tap:"
  echo "  1. Create repository: https://github.com/${REPO%-*}/homebrew-nself"
  echo "  2. Clone: git clone git@github.com:${REPO%-*}/homebrew-nself.git ~/Sites/homebrew-nself"
  echo "  3. Copy formula: cp ${RELEASE_DIR}/nself.rb ~/Sites/homebrew-nself/Formula/"
  echo "  4. Update SHA256 in formula to: ${SHA256}"
  echo "  5. Commit and push"
fi

# 4. Build and Push Docker Image
echo -e "${BLUE}Building and pushing Docker image...${RESET}"
cd "${RELEASE_DIR}"

# Build image
docker build -t "ghcr.io/${REPO}:${VERSION}" -t "ghcr.io/${REPO}:latest" .

# Login to GitHub Container Registry
echo -e "${YELLOW}Logging into GitHub Container Registry...${RESET}"
echo "Enter your GitHub Personal Access Token with 'write:packages' scope:"
docker login ghcr.io -u "${REPO%/*}"

# Push images
docker push "ghcr.io/${REPO}:${VERSION}"
docker push "ghcr.io/${REPO}:latest"

echo -e "${GREEN}✓ Docker images pushed to ghcr.io${RESET}"

# 5. Update install.nself.org
echo -e "${BLUE}Updating install.nself.org...${RESET}"
echo "Upload ${RELEASE_DIR}/install.nself.org.sh to your web server at install.nself.org"
echo "This allows users to install with: curl -sSL https://install.nself.org | bash"

# 6. Create announcement
echo -e "${GREEN}═══════════════════════════════════════════════════════${RESET}"
echo -e "${GREEN}✓ nself v${VERSION} published successfully!${RESET}"
echo -e "${GREEN}═══════════════════════════════════════════════════════${RESET}"
echo ""
echo "Installation methods now available:"
echo "  • Quick: curl -sSL https://install.nself.org | bash"
echo "  • Homebrew: brew tap ${REPO%-*}/nself && brew install nself"
echo "  • Docker: docker pull ghcr.io/${REPO}:${VERSION}"
echo "  • Manual: Download from https://github.com/${REPO}/releases/tag/v${VERSION}"
echo ""
echo "Next steps:"
echo "  1. Verify GitHub release: https://github.com/${REPO}/releases/tag/v${VERSION}"
echo "  2. Test installation methods"
echo "  3. Post announcement to Telegram/Discord"
echo "  4. Update website documentation"