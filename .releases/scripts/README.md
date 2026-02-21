# nself Release Scripts

This directory contains automation scripts for releasing nself across various distribution channels.

## Scripts

### 🚀 release.sh
Main release automation script that handles GitHub releases.

**Usage:**
```bash
./scripts/release.sh v0.3.7
```

**What it does:**
- Validates repository state
- Updates VERSION file
- Runs tests
- Creates git tag
- Pushes to GitHub
- Creates GitHub release (requires `gh` CLI)

### 🍺 update-homebrew.sh
Updates the Homebrew formula for macOS/Linux distribution.

**Usage:**
```bash
./scripts/update-homebrew.sh v0.3.7
```

**Prerequisites:**
- Homebrew tap repository (e.g., `nself-org/homebrew-nself`)
- Homebrew installed locally for testing

### 🐳 update-docker.sh
Builds and pushes multi-architecture Docker images.

**Usage:**
```bash
./scripts/update-docker.sh v0.3.7
```

**Prerequisites:**
- Docker installed and running
- Docker Hub account
- `docker login` completed

### 🪟 create-windows-installer.ps1
Generates Windows installation scripts and package manifests.

**Usage:**
```powershell
powershell -ExecutionPolicy Bypass -File scripts/create-windows-installer.ps1 v0.3.7
```

**Outputs:**
- `install-windows.ps1` - PowerShell installer
- `nself.json` - Scoop manifest
- `chocolatey/` - Chocolatey package files

## Release Process

For a complete release, run scripts in this order:

1. **Create GitHub Release:**
   ```bash
   ./scripts/release.sh v0.3.7
   ```

2. **Update Package Managers (optional):**
   ```bash
   ./scripts/update-homebrew.sh v0.3.7
   ./scripts/update-docker.sh v0.3.7
   ```

3. **Create Windows Installers (optional):**
   ```powershell
   powershell -File scripts/create-windows-installer.ps1 v0.3.7
   ```

## Prerequisites

### Required Tools
- Git
- Bash 4+ (for scripts)
- GitHub CLI (`gh`) for automated releases

### Optional Tools
- Docker (for Docker images)
- Homebrew (for formula testing)
- PowerShell (for Windows scripts)

### Environment Setup
```bash
# Install GitHub CLI (macOS)
brew install gh

# Install GitHub CLI (Linux)
curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | sudo dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | sudo tee /etc/apt/sources.list.d/github-cli.list > /dev/null
sudo apt update
sudo apt install gh

# Authenticate GitHub CLI
gh auth login
```

## Adding New Distribution Channels

To add a new package manager or distribution channel:

1. Create a new script: `update-<channel>.sh`
2. Follow the pattern of existing scripts
3. Add documentation here
4. Update `docs/RELEASE_PROCESS.MD`

## Troubleshooting

### release.sh fails
- Ensure you're on main branch
- Check for uncommitted changes
- Verify `gh` CLI is authenticated: `gh auth status`

### Docker build fails
- Ensure Docker daemon is running
- Check Docker Hub login: `docker login`
- Verify multi-arch builder: `docker buildx ls`

### Homebrew formula fails
- Check SHA256 calculation
- Verify tap repository access
- Test locally: `brew install --build-from-source Formula/nself.rb`

## Security Notes

⚠️ **Never commit:**
- API keys or tokens
- Docker Hub credentials  
- Personal access tokens
- Signing certificates

Use environment variables or secure credential storage for sensitive data.

## Support

For issues with release scripts:
- Check `docs/RELEASE_PROCESS.MD` for detailed documentation
- Open an issue on GitHub
- Contact maintainers

## License

These scripts are part of nself and follow the same MIT license.