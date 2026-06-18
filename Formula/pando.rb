class Pando < Formula
  desc "Fast multi-worktree dev environments"
  homepage "https://github.com/strausslabs/pando"
  version "0.1.11"

  on_macos do
    on_arm do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.11/pando-darwin-arm64.tar.gz"
      sha256 "7d370e36f6b9a6d871f438583a8233bffa9200c8ad52bbd51eed66e286a6113e"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.11/pando-linux-amd64.tar.gz"
      sha256 "0c17c7e0e5e18a854fc6dfa98a24aa644887304664fe76b1c7bdc49e7447d607"
    end
    on_arm do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.11/pando-linux-arm64.tar.gz"
      sha256 "12bbdb30f7d11295f4263a45cbb6ea6e1844c32bf74c7bb2cead6a68aaff367f"
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
