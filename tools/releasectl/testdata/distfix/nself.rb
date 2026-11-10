# FIXTURE: verbatim copy of nself-org/homebrew-nself Formula/nself.rb at origin/main 6cb01eac50ce37bb3cdf9383c4d8e53522390f06 (test.sh only).
# Formula auto-updated on each CLI release.
# Dual-arch sha256 blocks use the pre-built darwin binaries published to GitHub
# Releases (nself-<ver>-darwin-{arm64,amd64}.tar.gz). The sha256 values are
# taken from the release's checksums.txt and must be kept in sync with the tag.
class Nself < Formula
  desc "Self-hosted backend CLI: Postgres, GraphQL, Auth, Nginx in minutes"
  homepage "https://nself.org"
  version "1.4.13"
  license "MIT"

  depends_on "docker"
  depends_on "docker-compose"

  on_macos do
    on_arm do
      url "https://github.com/nself-org/cli/releases/download/v#{version}/nself-#{version}-darwin-arm64.tar.gz"
      sha256 "73432f349295c10e6f28621d92fdc7736284ecb8287b5df697f98d5289acd138"
    end

    on_intel do
      url "https://github.com/nself-org/cli/releases/download/v#{version}/nself-#{version}-darwin-amd64.tar.gz"
      sha256 "eda10ee8bd97ae6707c92bef4a77624fa26883e5bea39efb2c413c18cf2aa809"
    end
  end

  # Install the nself binary into the bin directory.
  # Purpose: Extract and place the pre-built CLI binary into the standard Homebrew installation path.
  # Inputs: bin (Homebrew-provided bin installation directory)
  # Outputs: nself binary available in PATH
  # Constraints: The tarball must contain a top-level 'nself' executable; if not, installation fails.
  def install
    bin.install "nself"
  end

  # Run post-install verification tests for the nself binary.
  # Purpose: Verify that the installed binary is executable and responds to basic commands.
  # Inputs: bin/nself from install phase
  # Outputs: Exit 0 if all tests pass; non-zero if any test fails
  # Constraints: docker and docker-compose dependencies must be available on the system.
  #              Tests do not verify docker functionality, only nself CLI responsiveness.
  test do
    system "#{bin}/nself", "version"
    system "#{bin}/nself", "help"
    system "#{bin}/nself", "doctor", "--help"
  end
end
