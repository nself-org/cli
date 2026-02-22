class Nself < Formula
  desc "Production-ready self-hosted backend infrastructure"
  homepage "https://nself.org"
  url "https://github.com/nself-org/cli/archive/refs/tags/v0.9.9.tar.gz"
  sha256 "0000000000000000000000000000000000000000000000000000000000000000" # TODO: update when v0.9.9 release tarball is published
  license "MIT"
  version "0.9.9"

  depends_on "docker"
  depends_on "docker-compose"

  def install
    # Install all source files to libexec
    libexec.install "src"

    # Install templates
    libexec.install "templates" if File.exist?("templates")

    # Create the main executable wrapper
    (bin/"nself").write <<~EOS
      #!/usr/bin/env bash
      exec "#{libexec}/src/cli/nself.sh" "$@"
    EOS

    # Make it executable
    (bin/"nself").chmod 0755

    # Install documentation
    doc.install "README.md", "LICENSE" if File.exist?("README.md")
    doc.install "docs" if File.exist?("docs")
  end

  def post_install
    # Create .nself directory structure
    nself_dir = File.expand_path("~/.nself")
    FileUtils.mkdir_p(nself_dir)

    # Copy source and templates to ~/.nself
    FileUtils.cp_r("#{libexec}/src", nself_dir)
    FileUtils.cp_r("#{libexec}/templates", nself_dir) if File.exist?("#{libexec}/templates")

    ohai "nself has been installed successfully!"
    ohai "Run 'nself init' to get started with your first project"
  end

  test do
    system "#{bin}/nself", "version"
    system "#{bin}/nself", "help"
  end
end
