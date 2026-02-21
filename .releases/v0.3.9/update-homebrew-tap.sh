#!/usr/bin/env bash
#
# Update Homebrew Tap for nself v0.3.9
#

set -e

VERSION="0.3.9"
GITHUB_USER="nself-org"
TAP_REPO="homebrew-nself"

echo "Setting up Homebrew tap for nself v${VERSION}..."

# Create tap repository if it doesn't exist
if [[ ! -d ~/Sites/${TAP_REPO} ]]; then
  echo "Creating Homebrew tap repository..."
  mkdir -p ~/Sites/${TAP_REPO}/Formula
  cd ~/Sites/${TAP_REPO}
  
  git init
  
  # Create README
  cat > README.md << 'EOF'
# Homebrew Tap for nself

This tap provides the nself formula for Homebrew installation.

## Installation

```bash
brew tap nself-org/nself
brew install nself
```

## Update

```bash
brew update
brew upgrade nself
```
EOF
  
  # Copy formula
  cp /Users/admin/Sites/nself/releases/v${VERSION}/nself.rb Formula/
  
  # Update SHA256 with actual value
  echo "Calculating SHA256..."
  cd /Users/admin/Sites/nself
  git archive --format=tar.gz --prefix="nself-${VERSION}/" "v${VERSION}" > /tmp/nself-v${VERSION}.tar.gz
  SHA256=$(shasum -a 256 /tmp/nself-v${VERSION}.tar.gz | cut -d' ' -f1)
  
  cd ~/Sites/${TAP_REPO}
  sed -i '' "s/sha256 \".*\"/sha256 \"${SHA256}\"/" Formula/nself.rb
  
  # Initial commit
  git add .
  git commit -m "Initial tap with nself v${VERSION}"
  
  echo ""
  echo "Next steps:"
  echo "1. Create repository at: https://github.com/${GITHUB_USER}/${TAP_REPO}"
  echo "2. Add remote: git remote add origin git@github.com:${GITHUB_USER}/${TAP_REPO}.git"
  echo "3. Push: git push -u origin main"
else
  echo "Updating existing Homebrew tap..."
  cd ~/Sites/${TAP_REPO}
  
  # Update formula
  cp /Users/admin/Sites/nself/releases/v${VERSION}/nself.rb Formula/
  
  # Update SHA256
  echo "Calculating SHA256..."
  cd /Users/admin/Sites/nself
  git archive --format=tar.gz --prefix="nself-${VERSION}/" "v${VERSION}" > /tmp/nself-v${VERSION}.tar.gz
  SHA256=$(shasum -a 256 /tmp/nself-v${VERSION}.tar.gz | cut -d' ' -f1)
  
  cd ~/Sites/${TAP_REPO}
  sed -i '' "s/sha256 \".*\"/sha256 \"${SHA256}\"/" Formula/nself.rb
  
  # Commit and push
  git add Formula/nself.rb
  git commit -m "Update nself to v${VERSION}"
  git push origin main
fi

echo "✓ Homebrew tap ready for nself v${VERSION}"
echo ""
echo "Users can now install with:"
echo "  brew tap ${GITHUB_USER}/nself"
echo "  brew install nself"