# Homebrew Installation

## For Users

To install nself via Homebrew:

```bash
# Add the tap
brew tap nself-org/nself

# Install nself
brew install nself
```

## For Maintainers

To update the formula for a new release:

1. Update the version number in `nself.rb`
2. Update the URL to point to the new release tag
3. Calculate the SHA256 of the release tarball:
   ```bash
   curl -L https://github.com/nself-org/cli/archive/refs/tags/vX.X.X.tar.gz | shasum -a 256
   ```
4. Update the sha256 field in the formula
5. Test locally:
   ```bash
   brew install --build-from-source ./nself.rb
   ```
6. Push to the tap repository