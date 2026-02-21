# nself v0.3.9 Publishing Guide

## Overview
This guide provides step-by-step instructions for publishing nself v0.3.9 to all distribution platforms.

## Prerequisites
- GitHub CLI (`gh`) installed and authenticated
- Docker with buildx support
- GitHub Personal Access Token with `write:packages` scope
- Access to nself GitHub repository
- Access to web server for install.nself.org (optional)

## Publishing Steps

### 1. GitHub Release

```bash
# Run the automated script
chmod +x ./publish-release.sh
./publish-release.sh

# Or manually:
# Create release
gh release create v0.3.9 \
  --title "nself v0.3.9" \
  --notes-file RELEASE_NOTES.md \
  --verify-tag

# Upload assets
gh release upload v0.3.9 \
  nself-v0.3.9.tar.gz \
  install.sh \
  install-debian.sh \
  install-rhel.sh \
  nself.rb \
  Dockerfile \
  docker-compose.release.yml \
  RELEASE_MANIFEST.md
```

### 2. Homebrew Tap

```bash
# Run the automated script
chmod +x ./update-homebrew-tap.sh
./update-homebrew-tap.sh

# Or manually:
# 1. Create/update tap repository
git clone git@github.com:nself-org/homebrew-nself.git ~/Sites/homebrew-nself
cd ~/Sites/homebrew-nself

# 2. Copy and update formula
cp /path/to/releases/v0.3.9/nself.rb Formula/
# Update SHA256 in formula

# 3. Commit and push
git add Formula/nself.rb
git commit -m "Update nself to v0.3.9"
git push origin main
```

### 3. Docker Registry (GitHub Container Registry)

```bash
# Run the automated script
chmod +x ./push-docker-images.sh
./push-docker-images.sh

# Or manually:
# 1. Build image
docker build -t ghcr.io/nself-org/cli:0.3.9 .

# 2. Login to GHCR
echo $GITHUB_TOKEN | docker login ghcr.io -u acamarata --password-stdin

# 3. Push images
docker push ghcr.io/nself-org/cli:0.3.9
docker tag ghcr.io/nself-org/cli:0.3.9 ghcr.io/nself-org/cli:latest
docker push ghcr.io/nself-org/cli:latest
```

### 4. Web Installer (install.nself.org)

Upload `install.nself.org.sh` to your web server:

```bash
# Via SSH
scp install.nself.org.sh user@server:/var/www/install.nself.org/index.html

# Or via hosting panel
# Upload install.nself.org.sh as index file
```

Configure web server to serve as plain text:
```nginx
location / {
    default_type text/plain;
    add_header Content-Type "text/plain; charset=utf-8";
}
```

### 5. Package Repositories (Future)

#### APT Repository (Debian/Ubuntu)
```bash
# Create .deb package (future)
dpkg-deb --build nself_0.3.9_amd64

# Sign and upload to PPA
dput ppa:acamarata/nself nself_0.3.9_source.changes
```

#### YUM Repository (RHEL/CentOS)
```bash
# Create .rpm package (future)
rpmbuild -ba nself.spec

# Sign and upload to repository
rpm --addsign nself-0.3.9-1.x86_64.rpm
```

## Verification Checklist

After publishing, verify each distribution method:

### ✅ GitHub Release
- [ ] Release visible at https://github.com/nself-org/cli/releases/tag/v0.3.9
- [ ] All assets uploaded and downloadable
- [ ] Release notes formatted correctly

### ✅ Homebrew
```bash
brew tap nself-org/nself
brew install nself
nself version  # Should show 0.3.9
```

### ✅ Docker
```bash
docker pull ghcr.io/nself-org/cli:0.3.9
docker run --rm ghcr.io/nself-org/cli:0.3.9 version
```

### ✅ Quick Install
```bash
curl -sSL https://install.nself.org | bash
nself version  # Should show 0.3.9
```

### ✅ Direct Downloads
```bash
# Debian/Ubuntu
wget https://raw.githubusercontent.com/nself-org/cli/v0.3.9/releases/v0.3.9/install-debian.sh
chmod +x install-debian.sh
./install-debian.sh

# RHEL/CentOS
wget https://raw.githubusercontent.com/nself-org/cli/v0.3.9/releases/v0.3.9/install-rhel.sh
chmod +x install-rhel.sh
./install-rhel.sh
```

## Post-Release Tasks

1. **Update Documentation**
   - Update README.md with new version
   - Update docs/INSTALLATION.md
   - Update website if applicable

2. **Announcements**
   - Post to Telegram channel
   - Post to Discord
   - Tweet from @nself_org
   - Update GitHub discussions

3. **Monitor**
   - Check GitHub Issues for installation problems
   - Monitor download statistics
   - Respond to user feedback

## Rollback Procedure

If issues are discovered:

1. **GitHub Release**
   ```bash
   # Mark as pre-release
   gh release edit v0.3.9 --prerelease
   
   # Or delete if critical
   gh release delete v0.3.9 --yes
   ```

2. **Homebrew**
   ```bash
   cd ~/Sites/homebrew-nself
   git revert HEAD
   git push origin main
   ```

3. **Docker**
   ```bash
   # Can't delete, but can update :latest tag
   docker tag ghcr.io/nself-org/cli:0.3.8 ghcr.io/nself-org/cli:latest
   docker push ghcr.io/nself-org/cli:latest
   ```

## Support Resources

- GitHub Issues: https://github.com/nself-org/cli/issues
- Release Notes: https://github.com/nself-org/cli/releases/tag/v0.3.9
- Documentation: https://github.com/nself-org/cli/tree/v0.3.9/docs

## Notes

- Always test installation methods before announcing
- Keep SHA256 hashes consistent across all platforms
- Maintain backward compatibility for upgrade paths
- Document any breaking changes prominently