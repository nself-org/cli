#!/usr/bin/env bash
#
# nself Quick Installer
# This file should be served at https://install.nself.org
#
# Usage:
#   curl -sSL https://install.nself.org | bash
#   wget -qO- https://install.nself.org | bash
#

set -e

# Latest stable version
NSELF_VERSION="0.3.9"

# Detect OS and architecture
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$OS" in
  linux*)
    # Detect Linux distribution
    if [ -f /etc/os-release ]; then
      . /etc/os-release
      DISTRO="$ID"
    elif [ -f /etc/debian_version ]; then
      DISTRO="debian"
    elif [ -f /etc/redhat-release ]; then
      DISTRO="rhel"
    else
      DISTRO="unknown"
    fi
    ;;
  darwin*)
    DISTRO="macos"
    ;;
  *)
    echo "Unsupported operating system: $OS"
    exit 1
    ;;
esac

# Download and run appropriate installer
case "$DISTRO" in
  ubuntu|debian|raspbian)
    echo "Detected Debian-based system. Installing nself v${NSELF_VERSION}..."
    curl -sSL "https://raw.githubusercontent.com/nself-org/cli/v${NSELF_VERSION}/releases/v${NSELF_VERSION}/install-debian.sh" | bash
    ;;
  rhel|centos|fedora|rocky|almalinux)
    echo "Detected RHEL-based system. Installing nself v${NSELF_VERSION}..."
    curl -sSL "https://raw.githubusercontent.com/nself-org/cli/v${NSELF_VERSION}/releases/v${NSELF_VERSION}/install-rhel.sh" | bash
    ;;
  macos)
    echo "Detected macOS. Installing nself v${NSELF_VERSION}..."
    if command -v brew >/dev/null 2>&1; then
      echo "Using Homebrew..."
      brew tap nself-org/nself
      brew install nself
    else
      echo "Homebrew not found. Using universal installer..."
      curl -sSL "https://raw.githubusercontent.com/nself-org/cli/v${NSELF_VERSION}/releases/v${NSELF_VERSION}/install.sh" | bash
    fi
    ;;
  *)
    echo "Using universal installer for nself v${NSELF_VERSION}..."
    curl -sSL "https://raw.githubusercontent.com/nself-org/cli/v${NSELF_VERSION}/releases/v${NSELF_VERSION}/install.sh" | bash
    ;;
esac