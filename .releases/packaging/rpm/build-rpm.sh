#!/bin/bash
set -e

VERSION="0.9.8"
PACKAGE_NAME="nself"

echo "Building RPM package for nself v${VERSION}"

# Create RPM build directories
mkdir -p ~/rpmbuild/{BUILD,RPMS,SOURCES,SPECS,SRPMS}

# Copy spec file
cp .releases/packaging/rpm/nself.spec ~/rpmbuild/SPECS/

# Download source tarball
cd ~/rpmbuild/SOURCES
curl -L -o "v${VERSION}.tar.gz" "https://github.com/nself-org/cli/archive/v${VERSION}.tar.gz"

# Build the RPM
cd ~/rpmbuild
rpmbuild -ba SPECS/nself.spec

# Copy the built RPM back to packaging directory
ARCH=$(uname -m)
if [ "$ARCH" = "x86_64" ]; then
    ARCH="x86_64"
elif [ "$ARCH" = "aarch64" ]; then
    ARCH="aarch64"
else
    ARCH="noarch"
fi

RPM_FILE="RPMS/noarch/${PACKAGE_NAME}-${VERSION}-1.*.noarch.rpm"
cp $RPM_FILE "$(pwd)/packaging/rpm/"

echo "✅ RPM package built: packaging/rpm/${PACKAGE_NAME}-${VERSION}-1.*.noarch.rpm"