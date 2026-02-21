class Nself < Formula
  desc "Self-hosted backend platform with Hasura, Auth, Storage, and more"
  homepage "https://nself.org"
  url "https://github.com/nself-org/cli/archive/v0.3.9.tar.gz"
  sha256 "9c20d0613c6dbc08a54a252ca4a92148135099b91409734a59414dd9725d2222"
  license "MIT"
  version "0.3.9"

  depends_on "bash" => :build
  depends_on "docker"
  depends_on "docker-compose"
  depends_on "mkcert" => :recommended
  depends_on "jq" => :recommended

  def install
    # Install all source files
    libexec.install Dir["*"]
    
    # Create wrapper script
    (bin/"nself").write <<~EOS
      #!/bin/bash
      export NSELF_HOME="#{libexec}"
      exec "#{libexec}/bin/nself" "$@"
    EOS
    
    # Make scripts executable
    chmod 0755, bin/"nself"
    chmod 0755, libexec/"bin/nself"
    
    # Install completions
    bash_completion.install "#{libexec}/completions/nself.bash" if File.exist?("#{libexec}/completions/nself.bash")
    zsh_completion.install "#{libexec}/completions/_nself" if File.exist?("#{libexec}/completions/_nself")
  end

  def caveats
    <<~EOS
      nself has been installed! 🚀
      
      Quick Start:
        mkdir myproject && cd myproject
        nself init
        nself build
        nself start
      
      Your services will be available at:
        GraphQL API: http://api.localhost
        Auth: http://auth.localhost
        Storage: http://storage.localhost
        Admin UI: http://localhost:3100 (run 'nself admin enable')
      
      Documentation: https://github.com/nself-org/cli/docs/
      Report issues: https://github.com/nself-org/cli/issues
    EOS
  end

  test do
    system "#{bin}/nself", "version"
  end
end