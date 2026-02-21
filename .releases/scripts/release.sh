#!/bin/bash

# release.sh - Automated release script for nself
# Usage: ./scripts/release.sh <version>

set -e

# Configuration
REPO="nself-org/cli"
VERSION="${1:-}"
BRANCH="main"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Functions
log_info() { echo -e "${BLUE}ℹ${NC} $1"; }
log_success() { echo -e "${GREEN}✓${NC} $1"; }
log_error() { echo -e "${RED}✗${NC} $1"; }
log_warning() { echo -e "${YELLOW}⚠${NC} $1"; }

# Validate version
if [[ -z "$VERSION" ]]; then
    log_error "Version required. Usage: $0 <version>"
    echo "Example: $0 v0.3.7"
    exit 1
fi

# Remove 'v' prefix if present for version comparisons
VERSION_NUM="${VERSION#v}"

# Check if we're in the repository root
if [[ ! -f "install.sh" ]] || [[ ! -d "src" ]]; then
    log_error "Must be run from nself repository root"
    exit 1
fi

# Check for clean git state
if [[ -n $(git status -s) ]]; then
    log_error "Working directory not clean. Commit or stash changes first."
    exit 1
fi

# Check we're on main branch
CURRENT_BRANCH=$(git branch --show-current)
if [[ "$CURRENT_BRANCH" != "$BRANCH" ]]; then
    log_warning "Not on $BRANCH branch (currently on $CURRENT_BRANCH)"
    read -p "Switch to $BRANCH? (y/n) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        git checkout "$BRANCH"
        git pull origin "$BRANCH"
    else
        exit 1
    fi
fi

log_info "Preparing release for nself ${VERSION}"
echo

# Step 1: Update version file
log_info "Updating VERSION file..."
echo "$VERSION" > src/VERSION
git add src/VERSION
log_success "VERSION file updated"

# Step 2: Update CHANGELOG if it exists
if [[ -f "CHANGELOG.md" ]]; then
    log_info "Please update CHANGELOG.md with release notes"
    log_info "Opening in default editor..."
    ${EDITOR:-nano} CHANGELOG.md
    git add CHANGELOG.md
    log_success "CHANGELOG.md updated"
fi

# Step 3: Run tests
log_info "Running tests..."
if [[ -f "src/tests/run_tests.sh" ]]; then
    if bash src/tests/run_tests.sh; then
        log_success "Tests passed"
    else
        log_error "Tests failed. Fix issues before releasing."
        exit 1
    fi
else
    log_warning "No tests found, skipping"
fi

# Step 4: Commit version bump
log_info "Committing version bump..."
git commit -m "Release ${VERSION}" || true

# Step 5: Create and push tag
log_info "Creating git tag ${VERSION}..."
git tag -a "$VERSION" -m "Release ${VERSION}"

# Step 6: Push to origin
log_info "Pushing to origin..."
git push origin "$BRANCH"
git push origin "$VERSION"
log_success "Pushed to GitHub"

# Step 7: Create GitHub release
if command -v gh &> /dev/null; then
    log_info "Creating GitHub release..."
    
    # Create release notes file
    cat > /tmp/release_notes.md << EOF
## 🚀 nself ${VERSION} Release

### ✨ What's New
- See [CHANGELOG.md](https://github.com/${REPO}/blob/main/CHANGELOG.md) for full details

### 📦 Installation

**New Installation:**
\`\`\`bash
curl -fsSL https://raw.githubusercontent.com/${REPO}/main/install.sh | bash
\`\`\`

**Upgrade Existing:**
\`\`\`bash
nself update
\`\`\`

### 🐛 Bug Reports
Report issues at: https://github.com/${REPO}/issues
EOF

    gh release create "$VERSION" \
        --repo "$REPO" \
        --title "nself ${VERSION}" \
        --notes-file /tmp/release_notes.md \
        --latest
    
    rm /tmp/release_notes.md
    log_success "GitHub release created"
else
    log_warning "GitHub CLI not installed. Create release manually at:"
    echo "https://github.com/${REPO}/releases/new"
fi

# Step 8: Create distribution packages (optional)
echo
log_info "Next steps for distribution:"
echo "  1. Update Homebrew formula (if exists)"
echo "  2. Update AUR package (if exists)"  
echo "  3. Build and push Docker image (if exists)"
echo "  4. Update package managers"
echo
echo "Run these commands as needed:"
echo "  ./scripts/update-homebrew.sh ${VERSION}"
echo "  ./scripts/update-docker.sh ${VERSION}"
echo "  ./scripts/update-aur.sh ${VERSION}"

log_success "Release ${VERSION} completed successfully!"
echo
echo "Release URL: https://github.com/${REPO}/releases/tag/${VERSION}"