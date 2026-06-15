class Pando < Formula
  desc "Fast multi-worktree dev environments"
  homepage "https://github.com/strausslabs/pando"
  version "0.1.8"

  on_macos do
    on_arm do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.8/pando-darwin-arm64.tar.gz"
      sha256 "3d3be080e987476865753f78113bf1dbc17b179fde8910764b6aca9031a8d399"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.8/pando-linux-amd64.tar.gz"
      sha256 "880ade741a030be4823aef70f902ca1a343f643ffc49df3bb656455b1f53e074"
    end
    on_arm do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.8/pando-linux-arm64.tar.gz"
      sha256 "15fc124ecf58edb1ab1891d4c911878d62d13a626e9e503622226257ca4e90a7"
    end
  end

  def install
    bin.install "pando"
  end

  def caveats
    <<~CAVEATS
      Driving Pando with an AI agent? Run:
        pando setup
      to install the pando.star skill into ~/.claude/skills and register the
      MCP server (claude mcp add pando -- pando mcp).
    CAVEATS
  end

  test do
    assert_match "pando version", shell_output("#{bin}/pando --version")
  end
end
