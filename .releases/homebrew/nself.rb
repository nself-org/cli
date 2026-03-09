class Nself < Formula
  desc "ɳSelf v0.9.9: Production Ready - Complete testing, documentation, and production features for v1.0 LTS"
  homepage "https://nself.org"
  url "https://github.com/nself-org/cli/archive/refs/tags/v0.9.9.tar.gz"
  sha256 "PLACEHOLDER_SHA256"
  license "Source-Available"
  version "0.9.9"

  depends_on "docker"
  depends_on "docker-compose"

  def install
    # Install ONLY essential source directories to libexec (exclude tests, examples)
    libexec.mkpath

    # Copy VERSION file
    (libexec/"src").install "src/VERSION" if File.exist?("src/VERSION")

    # Install only essential directories (exclude tests, examples, development files)
    %w[cli lib templates services].each do |dir|
      (libexec/"src").install "src/#{dir}" if File.directory?("src/#{dir}")
    end

    # Create the main executable wrapper
    (bin/"nself").write <<~EOS
      #!/usr/bin/env bash
      exec "#{libexec}/src/cli/nself.sh" "$@"
    EOS

    # Make it executable
    (bin/"nself").chmod 0755

    # Install minimal documentation (LICENSE only, docs available online)
    doc.install "LICENSE" if File.exist?("LICENSE")
  end

  def post_install
    # Create .nself directory structure
    nself_dir = File.expand_path("~/.nself")
    FileUtils.mkdir_p(nself_dir)
    
    # Copy source and templates to ~/.nself
    FileUtils.cp_r("#{libexec}/src", nself_dir)
    FileUtils.cp_r("#{libexec}/templates", nself_dir) if File.exist?("#{libexec}/templates")
    
    ohai "ɳSelf has been installed successfully!"
    ohai "Run 'nself init' to get started with your first ɳSelf project"
  end

  test do
    system "#{bin}/nself", "version"
    system "#{bin}/nself", "help"
  end
end