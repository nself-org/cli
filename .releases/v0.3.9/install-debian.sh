#!/usr/bin/env bash
#
# nself Debian/Ubuntu Installation Script
# Version: 0.3.9
#

set -e

NSELF_VERSION="0.3.9"
NSELF_REPO="nself-org/cli"
GITHUB_URL="https://github.com/${NSELF_REPO}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
RESET='\033[0m'

echo -e "${BLUE}Installing nself v${NSELF_VERSION} for Debian/Ubuntu...${RESET}"

# Update package list
sudo apt-get update

# Install dependencies
echo -e "${BLUE}Installing dependencies...${RESET}"
sudo apt-get install -y \
  curl \
  wget \
  git \
  jq \
  ca-certificates \
  gnupg \
  lsb-release

# Install Docker if not present
if ! command -v docker &> /dev/null; then
  echo -e "${BLUE}Installing Docker...${RESET}"
  curl -fsSL https://get.docker.com | sh
  sudo usermod -aG docker $USER
fi

# Install Docker Compose if not present
if ! command -v docker-compose &> /dev/null; then
  if ! docker compose version &> /dev/null; then
    echo -e "${BLUE}Installing Docker Compose...${RESET}"
    sudo apt-get install -y docker-compose-plugin
  fi
fi

# Install mkcert for SSL certificates
if ! command -v mkcert &> /dev/null; then
  echo -e "${BLUE}Installing mkcert...${RESET}"
  wget -O mkcert https://github.com/FiloSottile/mkcert/releases/download/v1.4.4/mkcert-v1.4.4-linux-amd64
  chmod +x mkcert
  sudo mv mkcert /usr/local/bin/
fi

# Download and install nself
echo -e "${BLUE}Downloading nself...${RESET}"
cd /tmp
wget -q "${GITHUB_URL}/archive/v${NSELF_VERSION}.tar.gz"
tar -xzf "v${NSELF_VERSION}.tar.gz"

echo -e "${BLUE}Installing nself...${RESET}"
sudo rm -rf /usr/local/nself
sudo mv "nself-${NSELF_VERSION}" /usr/local/nself

# Create symlink
sudo ln -sf /usr/local/nself/bin/nself /usr/local/bin/nself

# Set permissions
sudo chmod +x /usr/local/nself/bin/nself
sudo chmod -R 755 /usr/local/nself

# Install bash completions
if [ -d /etc/bash_completion.d ]; then
  echo -e "${BLUE}Installing bash completions...${RESET}"
  sudo tee /etc/bash_completion.d/nself > /dev/null << 'EOF'
_nself() {
  local cur="${COMP_WORDS[COMP_CWORD]}"
  local commands="init build start stop restart status logs doctor db admin reset help version update"
  COMPREPLY=($(compgen -W "${commands}" -- ${cur}))
}
complete -F _nself nself
EOF
fi

# Clean up
rm -f "/tmp/v${NSELF_VERSION}.tar.gz"

echo -e "${GREEN}✓ nself v${NSELF_VERSION} installed successfully!${RESET}"
echo ""
echo "Quick Start:"
echo "  mkdir myproject && cd myproject"
echo "  nself init"
echo "  nself build"
echo "  nself start"
echo ""
echo "Note: You may need to log out and back in for Docker group changes to take effect."