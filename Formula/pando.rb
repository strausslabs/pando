class Pando < Formula
  desc "Fast multi-worktree dev environments"
  homepage "https://github.com/strausslabs/pando"
  version "0.1.9"

  on_macos do
    on_arm do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.9/pando-darwin-arm64.tar.gz"
      sha256 "55e7d8a9a48b9c998924241f64c6e984d3623c5487193a46132f620597dbd6ca"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.9/pando-linux-amd64.tar.gz"
      sha256 "e513fd7ef4774dc866b80706cc09bfdcc8afb762eb2e6fbfbd215354f2079dfd"
    end
    on_arm do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.9/pando-linux-arm64.tar.gz"
      sha256 "29292396b67eaa3499163b5e15b775b9ba75fbd880f711dda7eac2c20ef6f852"
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
