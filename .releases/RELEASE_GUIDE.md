# nself v0.4.0 Release Guide

All package files have been prepared for the v0.4.0 release across multiple platforms.

## Release Workflow

### Step 1: Create GitHub Release (Required)

1. Go to: https://github.com/nself-org/cli/releases/new
2. Tag: `v0.4.0` (already created and pushed)
3. Title: `nself v0.4.0 - Production-Ready Release`
4. Description: Use the release notes provided
5. Mark as "Set as latest release"
6. Publish release

After publishing, GitHub will automatically create the source tarball at:
`https://github.com/nself-org/cli/archive/refs/tags/v0.4.0.tar.gz`

### Step 2: Calculate SHA256 for Homebrew

Once the GitHub release is published, calculate the SHA256:

```bash
curl -sL "https://github.com/nself-org/cli/archive/refs/tags/v0.4.0.tar.gz" | shasum -a 256
```

Then update `.releases/homebrew/nself.rb` with the actual SHA256 (currently shows PLACEHOLDER).

### Step 3: Update Homebrew (macOS/Linux)

**Repository**: https://github.com/nself-org/homebrew-nself

#### Option A: Automated Script
```bash
cd /Users/admin/Sites/nself
./.releases/scripts/update-homebrew.sh v0.4.0
```

This will:
- Download the release tarball
- Calculate SHA256
- Update the formula
- Push to homebrew-nself repository

#### Option B: Manual Update
```bash
# Clone or update tap repository
cd ~/homebrew-nself || git clone https://github.com/nself-org/homebrew-nself.git ~/homebrew-nself
cd ~/homebrew-nself

# Update Formula/nself.rb
git add Formula/nself.rb
git commit -m "Update nself to v0.4.0"
git push
```

Users install with:
```bash
brew tap nself-org/nself
brew install nself
```

## Version Information

- **Version**: 0.4.0
- **Tag**: v0.4.0
- **Status**: Production-Ready
- **Source**: https://github.com/nself-org/cli/archive/refs/tags/v0.4.0.tar.gz
- **Release Notes**: See `/docs/releases/v0.4.0.md`

## Post-Release Checklist

- [ ] GitHub Release published with release notes
- [ ] Homebrew formula updated with correct SHA256
- [ ] Test installation on macOS
- [ ] Test installation on Linux
- [ ] Verify `nself version` returns `0.4.0`

## Priority Release Channels

For v0.4.0, focus on:

1. ✅ **GitHub Release** (Required)
2. ✅ **install.nself.org** (Already deployed)
3. 🔄 **Homebrew** (Update after GitHub release)

## Support

- **GitHub**: https://github.com/nself-org/cli
- **Issues**: https://github.com/nself-org/cli/issues

---

*Last Updated: October 13, 2025 for v0.4.0 release*
